from __future__ import annotations

import json
import os
import re
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path
from typing import Callable

from .logstore import LogStore
from .models import (
    DIAG_STAGE_DEFS,
    INSTALL_STAGE_DEFS,
    UNINSTALL_STAGE_DEFS,
    DiagnoseSpec,
    InstallSpec,
    OperationResult,
    RunnerError,
    StageDef,
    UninstallSpec,
)
from .system_ops import CancelToken, AppPaths, command_output, stream_command

EVENT_PREFIX = "::despatch-event::"
LOG_RE = re.compile(r"^\[(INFO|WARN|ERR\s*)\]\s*(.*)$")

EventCb = Callable[[dict], None]


class OperationRunner:
    def __init__(self, paths: AppPaths) -> None:
        self.paths = paths

    def run_install(
        self,
        spec: InstallSpec,
        logstore: LogStore,
        cancel: CancelToken,
        on_event: EventCb,
    ) -> OperationResult:
        cmd = ["bash", str(self.paths.install_script)]
        env = os.environ.copy()
        env.update(spec.to_env())
        return self._run_script(
            operation="install",
            stage_defs=INSTALL_STAGE_DEFS,
            cmd=cmd,
            env=env,
            logstore=logstore,
            cancel=cancel,
            on_event=on_event,
            install_spec=spec,
        )

    def run_uninstall(
        self,
        spec: UninstallSpec,
        logstore: LogStore,
        cancel: CancelToken,
        on_event: EventCb,
    ) -> OperationResult:
        cmd = ["bash", str(self.paths.uninstall_script)]
        env = os.environ.copy()
        env.update(spec.to_env())
        return self._run_script(
            operation="uninstall",
            stage_defs=UNINSTALL_STAGE_DEFS,
            cmd=cmd,
            env=env,
            logstore=logstore,
            cancel=cancel,
            on_event=on_event,
        )

    def run_diagnose(
        self,
        _spec: DiagnoseSpec,
        logstore: LogStore,
        cancel: CancelToken,
        on_event: EventCb,
    ) -> OperationResult:
        cmd = ["bash", str(self.paths.diagnose_script)]
        env = os.environ.copy()
        env["DESPATCH_NONINTERACTIVE"] = "1"
        return self._run_script(
            operation="diagnose",
            stage_defs=DIAG_STAGE_DEFS,
            cmd=cmd,
            env=env,
            logstore=logstore,
            cancel=cancel,
            on_event=on_event,
        )

    def _run_script(
        self,
        *,
        operation: str,
        stage_defs: tuple[StageDef, ...],
        cmd: list[str],
        env: dict[str, str],
        logstore: LogStore,
        cancel: CancelToken,
        on_event: EventCb,
        install_spec: InstallSpec | None = None,
    ) -> OperationResult:
        run_id = str(uuid.uuid4())
        env["DESPATCH_TUI_MODE"] = "1"
        env["DESPATCH_RUN_ID"] = run_id

        seen_run_result = False
        seen_stage_result = False
        had_run_result_event = False
        run_result_status = ""
        terminal_stage_id = stage_defs[-1].stage_id if stage_defs else ""
        terminal_stage_ok = False
        errors: list[RunnerError] = []
        active_stage = stage_defs[0].stage_id if stage_defs else ""

        on_event({"type": "run_start", "run_id": run_id, "operation": operation})

        for stage in stage_defs:
            # Prime UI with deterministic stage catalog.
            on_event(
                {
                    "type": "stage_start",
                    "stage_id": stage.stage_id,
                    "title": stage.title,
                    "weight": str(stage.weight),
                    "message": "pending",
                }
            )
            on_event(
                {
                    "type": "stage_progress",
                    "stage_id": stage.stage_id,
                    "current": "0",
                    "total": "1",
                    "message": "pending",
                    "rate_hint": "",
                    "eta_hint": "",
                }
            )

        # Reset first stage to running so UI doesn't show only pending states.
        if stage_defs:
            on_event(
                {
                    "type": "stage_start",
                    "stage_id": stage_defs[0].stage_id,
                    "title": stage_defs[0].title,
                    "weight": str(stage_defs[0].weight),
                    "message": "started",
                }
            )

        def on_line(line: str) -> None:
            nonlocal seen_run_result, seen_stage_result, had_run_result_event, active_stage, run_result_status, terminal_stage_ok
            if line.startswith(EVENT_PREFIX):
                payload = line[len(EVENT_PREFIX) :].strip()
                try:
                    evt = json.loads(payload)
                    evt_type = str(evt.get("type", ""))
                    if evt_type == "stage_start":
                        active_stage = str(evt.get("stage_id", active_stage))
                    if evt_type == "stage_result":
                        seen_stage_result = True
                        if str(evt.get("stage_id", "")) == terminal_stage_id:
                            terminal_stage_ok = str(evt.get("status", "")) == "ok"
                    if evt_type == "run_result":
                        seen_run_result = True
                        had_run_result_event = True
                        run_result_status = str(evt.get("status", ""))
                    on_event(evt)
                except json.JSONDecodeError:
                    logstore.append("warn", active_stage, f"Malformed event: {payload}")
                return

            level = "info"
            message = line
            match = LOG_RE.match(line)
            if match:
                raw = match.group(1).strip().lower()
                message = match.group(2)
                if raw.startswith("err"):
                    level = "error"
                elif raw == "warn":
                    level = "warn"
                else:
                    level = "info"
            if "[non-interactive]" in message:
                level = "debug"
            category = self._log_category(active_stage, message)
            entry = logstore.append(level, active_stage, message, category=category)
            on_event(
                {
                    "type": "log",
                    "level": entry.level,
                    "stage_id": active_stage,
                    "category": category,
                    "message": entry.message,
                    "ts": str(entry.ts),
                }
            )
            if level == "error":
                errors.append(
                    RunnerError(
                        code="E_COMMAND",
                        message=entry.message,
                        stage_id=active_stage,
                        suggested_fix="Inspect log and retry the operation.",
                    )
                )

        exit_code = stream_command(cmd, self.paths.root_dir, env, cancel, on_line)

        if cancel.cancelled:
            on_event({"type": "run_result", "status": "cancelled", "failed_stage": active_stage, "exit_code": "130"})
            return OperationResult(
                status="cancelled",
                errors=[RunnerError("E_CANCELLED", "Operation cancelled by operator.", active_stage)],
                artifacts={
                    "full_log": str(logstore.log_path),
                },
                next_actions=["Re-run operation when ready."],
            )

        if not seen_stage_result and active_stage:
            on_event(
                {
                    "type": "stage_progress",
                    "stage_id": active_stage,
                    "current": "1",
                    "total": "1",
                    "message": "done" if exit_code == 0 else "failed",
                    "rate_hint": "",
                    "eta_hint": "",
                }
            )
            on_event(
                {
                    "type": "stage_result",
                    "stage_id": active_stage,
                    "status": "ok" if exit_code == 0 else "failed",
                    "error_code": "" if exit_code == 0 else "E_COMMAND",
                }
            )

        if not seen_run_result:
            status = "ok" if exit_code == 0 else "failed"
            run_result_status = status
            if operation == "diagnose":
                on_event(
                    {
                        "type": "run_result",
                        "status": status,
                        "failed_stage": "" if exit_code == 0 else active_stage,
                        "exit_code": str(exit_code),
                    }
                )
                seen_run_result = True

        protocol_issue = self._detect_protocol_issue(
            operation=operation,
            exit_code=exit_code,
            seen_run_result=had_run_result_event,
            seen_stage_result=seen_stage_result,
            run_result_status=run_result_status,
            terminal_stage_id=terminal_stage_id,
            terminal_stage_ok=terminal_stage_ok,
        )
        if protocol_issue:
            on_event(
                {
                    "type": "run_result",
                    "status": "failed",
                    "failed_stage": active_stage or terminal_stage_id,
                    "exit_code": str(exit_code if exit_code != 0 else 1),
                }
            )
            return OperationResult(
                status="failed",
                errors=[
                    RunnerError(
                        code="E_PROTOCOL",
                        message=protocol_issue,
                        stage_id=active_stage or terminal_stage_id,
                        suggested_fix="Re-run installer from latest scripts and inspect event/log output.",
                    )
                ],
                artifacts={"full_log": str(logstore.log_path)},
                next_actions=[
                    "Update scripts and retry.",
                    "Run diagnostics to confirm current deployment state.",
                ],
            )

        if run_result_status and run_result_status not in {"ok", "partial"}:
            if not errors:
                errors.append(
                    RunnerError(
                        code="E_COMMAND",
                        message=f"Operation reported status={run_result_status}",
                        stage_id=active_stage,
                        suggested_fix="Inspect failure inspector and logs.",
                    )
                )
            return OperationResult(
                status="failed",
                errors=errors,
                artifacts={"full_log": str(logstore.log_path)},
                next_actions=[
                    "Review failure inspector details.",
                    "Run diagnostics from the tool catalog.",
                ],
            )

        if exit_code == 0:
            artifacts = {"full_log": str(logstore.log_path)}
            if operation == "install" and install_spec is not None:
                verify = self._verify_install_postchecks(install_spec, logstore, run_id)
                artifacts.update(verify["artifacts"])
                if not verify["ok"]:
                    on_event(
                        {
                            "type": "run_result",
                            "status": "failed",
                            "failed_stage": "post_checks",
                            "exit_code": "1",
                        }
                    )
                    return OperationResult(
                        status="failed",
                        errors=verify["errors"],
                        artifacts=artifacts,
                        next_actions=verify["next_actions"],
                    )

            next_actions = ["Open web UI and verify OOBE/mail access."]
            if operation == "install" and install_spec is not None and not install_spec.install_service:
                next_actions = ["Service install skipped intentionally (install_service=false). Start manually if needed."]
            return OperationResult(
                status="ok",
                artifacts=artifacts,
                next_actions=next_actions,
            )

        if not errors:
            errors.append(
                RunnerError(
                    code="E_COMMAND",
                    message=f"Command exited with code {exit_code}",
                    stage_id=active_stage,
                    suggested_fix="Open Failure Inspector and run suggested checks.",
                )
            )

        return OperationResult(
            status="failed",
            errors=errors,
            artifacts={
                "full_log": str(logstore.log_path),
            },
            next_actions=[
                "Review failure inspector details.",
                "Run diagnostics from the tool catalog.",
            ],
        )

    @staticmethod
    def _detect_protocol_issue(
        *,
        operation: str,
        exit_code: int,
        seen_run_result: bool,
        seen_stage_result: bool,
        run_result_status: str,
        terminal_stage_id: str,
        terminal_stage_ok: bool,
    ) -> str:
        if operation not in {"install", "uninstall"}:
            return ""

        if not seen_run_result:
            return "Missing run_result event from installer protocol."

        if exit_code == 0 and not terminal_stage_ok and terminal_stage_id:
            return f"Missing terminal stage_result success for '{terminal_stage_id}'."

        if run_result_status == "ok" and not seen_stage_result:
            return "run_result=ok received without any stage_result events."

        return ""

    @staticmethod
    def _log_category(stage_id: str, message: str) -> str:
        stage = (stage_id or "").lower()
        msg = (message or "").lower()
        if "proxy" in stage or "nginx" in msg or "apache" in msg:
            return "proxy"
        if "firewall" in stage or "ufw" in msg:
            return "network"
        if "service" in stage or "systemctl" in msg:
            return "service"
        if "auth" in stage or "dovecot" in msg or "pam" in msg or "sql" in msg:
            return "auth"
        if "dns" in msg or "port" in msg or "health" in msg:
            return "network"
        return "system"

    def _verify_install_postchecks(
        self,
        spec: InstallSpec,
        logstore: LogStore,
        run_id: str,
    ) -> dict[str, object]:
        if not spec.install_service:
            return {
                "ok": True,
                "errors": [],
                "artifacts": {},
                "next_actions": ["Service install skipped intentionally (install_service=false)."],
            }

        checks: dict[str, str] = {}
        errors: list[RunnerError] = []

        unit_listing = command_output(
            ["systemctl", "list-unit-files", "--type=service", "--no-pager", "despatch.service"],
            timeout=5.0,
        )
        checks["systemctl_list_unit_files"] = unit_listing
        if "despatch.service" not in unit_listing:
            errors.append(
                RunnerError(
                    code="UNIT_MISSING",
                    message="despatch.service unit file was not found after install.",
                    stage_id="post_checks",
                    suggested_fix="Run: systemctl list-unit-files | grep despatch",
                )
            )

        health_url = self._health_url_for_listen(spec.listen_addr)
        service_ready, is_active, sub_state, health_ok, health_detail = self._wait_for_despatch_ready(health_url)
        checks["systemctl_is_active"] = is_active
        checks["systemctl_sub_state"] = sub_state
        checks["health_url"] = health_url
        checks["health_response"] = health_detail
        if not service_ready:
            errors.append(
                RunnerError(
                    code="SERVICE_INACTIVE",
                    message=(
                        "despatch service is not ready "
                        f"(is-active={is_active or 'unknown'}, sub-state={sub_state or 'unknown'})."
                    ),
                    stage_id="post_checks",
                    suggested_fix="Run: systemctl status despatch --no-pager",
                )
            )
        if not health_ok:
            errors.append(
                RunnerError(
                    code="HEALTH_FAIL",
                    message=f"Local health check failed for {health_url}.",
                    stage_id="post_checks",
                    suggested_fix="Run: curl -fsS " + health_url,
                )
            )

        artifact_path = logstore.log_dir / f"post-checks-{run_id}.json"
        with artifact_path.open("w", encoding="utf-8") as fh:
            json.dump(checks, fh, indent=2, sort_keys=True)
            fh.write("\n")

        artifacts = {"post_checks": str(artifact_path)}
        if not errors:
            logstore.append("info", "post_checks", "Post-install service invariants verified.")
            return {"ok": True, "errors": [], "artifacts": artifacts, "next_actions": []}

        for err in errors:
            logstore.append("error", "post_checks", f"{err.code}: {err.message}")
        return {
            "ok": False,
            "errors": [
                RunnerError(
                    code="E_SERVICE",
                    message="Post-install service invariants failed.",
                    stage_id="post_checks",
                    suggested_fix="Inspect post-check artifact and run remediation commands.",
                ),
                *errors,
            ],
            "artifacts": artifacts,
            "next_actions": [
                "Run: systemctl status despatch --no-pager",
                "Run: journalctl -u despatch -n 100 --no-pager",
                "Run diagnose from the tool catalog.",
            ],
        }

    @staticmethod
    def _health_url_for_listen(listen_addr: str) -> str:
        val = (listen_addr or "").strip()
        host = "127.0.0.1"
        port = "8080"
        if val.startswith(":"):
            port = val[1:] or port
        elif ":" in val:
            host_part, port_part = val.rsplit(":", 1)
            host_part = host_part.strip("[]")
            host = host_part or host
            port = port_part or port
        if host in {"0.0.0.0", "::", ""}:
            host = "127.0.0.1"
        if ":" in host:
            host = f"[{host}]"
        return f"http://{host}:{port}/health/live"

    def _wait_for_despatch_ready(
        self,
        health_url: str,
        timeout_sec: float = 45.0,
    ) -> tuple[bool, str, str, bool, str]:
        deadline = time.monotonic() + timeout_sec
        state = "unknown"
        sub_state = "unknown"
        health_ok = False
        health_detail = "unreachable"

        while True:
            state = command_output(["systemctl", "is-active", "despatch"], timeout=5.0).strip() or "unknown"
            sub_state = (
                command_output(["systemctl", "show", "despatch", "--property=SubState", "--value"], timeout=5.0).strip()
                or "unknown"
            )
            health_ok, health_detail = self._http_health_ok(health_url)

            if state in {"active", "activating"} and health_ok:
                return True, state, sub_state, health_ok, health_detail

            should_retry = state == "activating" or (state == "active" and not health_ok)
            if not should_retry or time.monotonic() >= deadline:
                return False, state, sub_state, health_ok, health_detail

            time.sleep(1.0)

    @staticmethod
    def _http_health_ok(url: str) -> tuple[bool, str]:
        req = urllib.request.Request(url, method="GET")
        try:
            with urllib.request.urlopen(req, timeout=5.0) as resp:
                body = resp.read(256).decode("utf-8", errors="ignore")
                return 200 <= resp.status < 300, f"status={resp.status} body={body}"
        except urllib.error.URLError as exc:
            return False, str(exc)
