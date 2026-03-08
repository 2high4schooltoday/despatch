from __future__ import annotations

import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from scripts.tui.assistant import INSTALL_FLOW, UNINSTALL_FLOW, visible_fields
from scripts.tui.assistant_app import (
    render_completion_preview,
    render_form_preview,
    render_license_preview,
    render_progress_preview,
    render_review_preview,
    render_welcome_preview,
)
from scripts.tui.glyphs import ASCII_GLYPHS, UNICODE_GLYPHS, smooth_bar
from scripts.tui.models import INSTALL_STAGE_DEFS, InstallSpec
from scripts.tui.logstore import LogStore
from scripts.tui.models import AppPaths
from scripts.tui.runner import OperationRunner
from scripts.tui.state import apply_runner_event, new_run_state
from scripts.tui.system_ops import detect_letsencrypt_cert_pair, fetch_repo_text, github_raw_url


class InstallSpecTests(unittest.TestCase):
    def _install_step(self, key: str):
        return next(step for step in INSTALL_FLOW if step.key == key)

    def test_install_spec_validation(self) -> None:
        spec = InstallSpec(base_domain="invalid", proxy_setup=True, proxy_server_name="")
        errs = spec.validate()
        self.assertTrue(errs)

    def test_sql_requires_driver_and_dsn(self) -> None:
        spec = InstallSpec(base_domain="example.com", dovecot_auth_mode="sql", proxy_setup=False)
        errs = spec.validate()
        self.assertTrue(any("driver" in e.lower() for e in errs))
        self.assertTrue(any("dsn" in e.lower() for e in errs))

    def test_visible_install_fields_hide_sql_and_tls_dependents(self) -> None:
        spec = InstallSpec(base_domain="example.com", proxy_setup=False, dovecot_auth_mode="pam")
        names = visible_fields("install", self._install_step("security"), spec)
        self.assertNotIn("dovecot_auth_db_driver", names)
        self.assertNotIn("dovecot_auth_db_dsn", names)
        self.assertNotIn("proxy_cert", names)
        self.assertNotIn("proxy_key", names)

    def test_visible_install_fields_include_sql_and_tls_dependents(self) -> None:
        spec = InstallSpec(
            base_domain="example.com",
            proxy_setup=True,
            proxy_tls=True,
            dovecot_auth_mode="sql",
            dovecot_auth_db_driver="mysql",
            dovecot_auth_db_dsn="dsn",
        )
        names = visible_fields("install", self._install_step("security"), spec)
        self.assertIn("dovecot_auth_db_driver", names)
        self.assertIn("dovecot_auth_db_dsn", names)
        self.assertIn("proxy_cert", names)
        self.assertIn("proxy_key", names)


class StateReducerTests(unittest.TestCase):
    def test_progress_aggregation(self) -> None:
        run = new_run_state("install", "run1", INSTALL_STAGE_DEFS)
        apply_runner_event(run, {"type": "stage_start", "stage_id": "preflight", "message": "started"})
        apply_runner_event(run, {"type": "stage_progress", "stage_id": "preflight", "current": "1", "total": "1", "message": "done"})
        apply_runner_event(run, {"type": "stage_result", "stage_id": "preflight", "status": "ok", "error_code": ""})
        self.assertGreater(run.overall_progress, 0.05)

    def test_run_result_failed(self) -> None:
        run = new_run_state("install", "run1", INSTALL_STAGE_DEFS)
        apply_runner_event(run, {"type": "run_result", "status": "failed", "failed_stage": "deps", "exit_code": "1"})
        self.assertEqual(run.status, "failed")
        self.assertEqual(run.failed_stage, "deps")


class GlyphTests(unittest.TestCase):
    def test_unicode_bar_uses_partial_blocks(self) -> None:
        out = smooth_bar(10, 0.35, UNICODE_GLYPHS)
        self.assertEqual(len(out), 10)
        self.assertTrue(any(ch in out for ch in "▏▎▍▌▋▊▉█"))

    def test_ascii_bar_uses_bracket_fallback(self) -> None:
        out = smooth_bar(10, 0.35, ASCII_GLYPHS)
        self.assertTrue(out.startswith("["))
        self.assertTrue(out.endswith("]"))


class PreviewRenderTests(unittest.TestCase):
    def test_welcome_preview_contains_installer_shell(self) -> None:
        lines = render_welcome_preview(120, 34)
        joined = "\n".join(lines)
        self.assertIn("Despatch Installer Assistant", joined)
        self.assertIn("Welcome to the Despatch Installer", joined)
        self.assertIn("Continue", joined)

    def test_license_preview_contains_license_gate_and_actions(self) -> None:
        lines = render_license_preview(120, 34)
        joined = "\n".join(lines)
        self.assertIn("Software License", joined)
        self.assertIn("Despatch Software License", joined)
        self.assertIn("Decline", joined)
        self.assertIn("Agree", joined)
        self.assertIn("You must choose Agree", joined)

    def test_license_preview_renders_in_ascii_mode(self) -> None:
        lines = render_license_preview(96, 24, ascii_mode=True)
        joined = "\n".join(lines)
        self.assertIn("ASCII", joined)
        self.assertIn("Decline", joined)
        self.assertIn("Agree", joined)

    def test_header_hint_does_not_share_the_divider_row(self) -> None:
        lines = render_welcome_preview(120, 34)
        self.assertIn("Installer flow is guided.", lines[4])
        self.assertNotIn("Installer flow", lines[5])

    def test_form_previews_remove_full_width_shade_artifacts(self) -> None:
        for step_key in ("scope", "network", "security"):
            lines = render_form_preview(120, 34, step_key)
            joined = "\n".join(lines)
            self.assertNotIn("░░░░░░", joined)
            self.assertIn("Back", joined)
            self.assertIn("Continue", joined)

    def test_form_previews_use_plain_language_install_copy(self) -> None:
        security = "\n".join(render_form_preview(120, 34, "security"))
        network = "\n".join(render_form_preview(120, 34, "network"))
        review = "\n".join(render_review_preview(120, 34))
        joined = "\n".join([security, network, review])
        self.assertIn("Sign-in and connection settings", security)
        self.assertIn("Sign-in source", security)
        self.assertIn("Website address", network)
        self.assertIn("Use HTTPS", review)
        self.assertIn("firewall", security.lower())
        self.assertNotIn("Dovecot", joined)
        self.assertNotIn("DSN", joined)
        self.assertNotIn("systemd", joined)

    def test_review_preview_wraps_long_values_without_overflow(self) -> None:
        lines = render_review_preview(120, 34, long_values=True)
        joined = "\n".join(lines)
        self.assertIn("really.long.example.mail.2h", joined)
        self.assertIn("4s2d.ru/with/a/deep/path", joined)
        self.assertIn("with/a/deep/path", joined)
        self.assertTrue(all(len(line) == 120 for line in lines))

    def test_progress_preview_can_show_log_drawer(self) -> None:
        lines = render_progress_preview(120, 34, True)
        joined = "\n".join(lines)
        self.assertIn("Installing Despatch", joined)
        self.assertIn("Stage timeline", joined)
        self.assertIn("Log", joined)
        self.assertIn("What is happening now", joined)
        self.assertIn("Show Log", "\n".join(render_progress_preview(120, 34, False)))
        self.assertIn("Hide Log", joined)

    def test_completion_preview_can_show_log_drawer(self) -> None:
        lines = render_completion_preview(120, 34, True)
        joined = "\n".join(lines)
        self.assertIn("Finished", joined)
        self.assertIn("Next actions", joined)
        self.assertIn("Hide Log", joined)

    def test_compact_preview_still_renders_shell(self) -> None:
        lines = render_welcome_preview(96, 24)
        self.assertEqual(len(lines), 24)
        joined = "\n".join(lines)
        self.assertIn("Install or Update Despatch", joined)
        self.assertIn("Uninstall", joined)

    def test_ascii_preview_still_renders_clean_shell(self) -> None:
        lines = render_welcome_preview(96, 24, ascii_mode=True)
        joined = "\n".join(lines)
        self.assertIn("ASCII", joined)
        self.assertIn("Continue", joined)
        self.assertTrue(all(len(line) == 96 for line in lines))


class LogStoreTests(unittest.TestCase):
    def test_logstore_falls_back_when_preferred_dir_unwritable(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            home = Path(td)
            real_mkdir = Path.mkdir

            def fake_mkdir(path_obj: Path, *args: object, **kwargs: object) -> None:
                if str(path_obj) == "/var/log/despatch":
                    raise PermissionError("denied")
                return real_mkdir(path_obj, *args, **kwargs)

            with mock.patch("scripts.tui.logstore.Path.home", return_value=home), mock.patch(
                "scripts.tui.logstore.Path.mkdir",
                new=fake_mkdir,
            ):
                store = LogStore(max_entries=32, log_dir=Path("/var/log/despatch"))
                expected_root = home / ".cache" / "despatch-tui" / "logs"
                self.assertTrue(str(store.log_dir).startswith(str(expected_root)))
                self.assertTrue(store.log_path.exists())

    def test_log_category_filtering(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            store = LogStore(max_entries=32, log_dir=Path(td))
            store.append("info", "proxy", "proxy up", category="proxy")
            store.append("info", "service", "service active", category="service")
            only_proxy = store.filtered({"info"}, "", {"proxy"})
            self.assertEqual(len(only_proxy), 1)
            self.assertEqual(only_proxy[0].category, "proxy")


class RunnerTests(unittest.TestCase):
    def _runner(self, tmp: Path) -> OperationRunner:
        paths = AppPaths(
            root_dir=tmp,
            scripts_dir=tmp,
            install_script=tmp / "auto_install.sh",
            uninstall_script=tmp / "uninstall.sh",
            diagnose_script=tmp / "diagnose_access.sh",
        )
        return OperationRunner(paths)

    def test_missing_run_result_is_protocol_failure(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp = Path(td)
            runner = self._runner(tmp)
            logstore = LogStore(max_entries=128, log_dir=tmp / "logs")
            spec = InstallSpec(base_domain="example.com", proxy_setup=False, install_service=False)

            def fake_stream(_cmd, _cwd, _env, _cancel, on_line):  # type: ignore[no-untyped-def]
                on_line('::despatch-event::{"type":"stage_result","stage_id":"preflight","status":"ok","error_code":""}')
                return 0

            with mock.patch("scripts.tui.runner.stream_command", new=fake_stream):
                result = runner.run_install(spec, logstore, mock.Mock(cancelled=False), lambda _evt: None)
            self.assertEqual(result.status, "failed")
            self.assertTrue(any(err.code == "E_PROTOCOL" for err in result.errors))

    def test_post_install_verifier_reports_service_error_when_unit_missing(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp = Path(td)
            runner = self._runner(tmp)
            logstore = LogStore(max_entries=128, log_dir=tmp / "logs")
            spec = InstallSpec(base_domain="example.com", proxy_setup=False, install_service=True)

            def fake_cmd_output(cmd: list[str], timeout: float = 2.0) -> str:
                _ = timeout
                if cmd[:2] == ["systemctl", "list-unit-files"]:
                    return ""
                if cmd[:2] == ["systemctl", "is-active"]:
                    return "inactive"
                return ""

            with mock.patch("scripts.tui.runner.command_output", side_effect=fake_cmd_output), mock.patch(
                "scripts.tui.runner.OperationRunner._http_health_ok",
                return_value=(True, "ok"),
            ):
                verify = runner._verify_install_postchecks(spec, logstore, "run1")
            self.assertFalse(bool(verify["ok"]))
            errs = verify["errors"]
            assert isinstance(errs, list)
            codes = {err.code for err in errs}
            self.assertIn("E_SERVICE", codes)
            self.assertIn("UNIT_MISSING", codes)

    def test_post_install_verifier_accepts_activating_when_health_is_ready(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp = Path(td)
            runner = self._runner(tmp)
            logstore = LogStore(max_entries=128, log_dir=tmp / "logs")
            spec = InstallSpec(base_domain="example.com", proxy_setup=False, install_service=True)

            def fake_cmd_output(cmd: list[str], timeout: float = 2.0) -> str:
                _ = timeout
                if cmd[:2] == ["systemctl", "list-unit-files"]:
                    return "despatch.service enabled"
                if cmd[:2] == ["systemctl", "is-active"]:
                    return "activating"
                if cmd[:3] == ["systemctl", "show", "despatch"]:
                    return "start-post"
                return ""

            with mock.patch("scripts.tui.runner.command_output", side_effect=fake_cmd_output), mock.patch(
                "scripts.tui.runner.OperationRunner._http_health_ok",
                return_value=(True, "status=200 body=ok"),
            ):
                verify = runner._verify_install_postchecks(spec, logstore, "run2")

            self.assertTrue(bool(verify["ok"]))

    def test_health_url_for_listen_formats_ipv6(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp = Path(td)
            runner = self._runner(tmp)
            self.assertEqual(
                runner._health_url_for_listen("[::1]:8080"),
                "http://[::1]:8080/health/live",
            )


class SystemOpsTests(unittest.TestCase):
    def test_github_raw_url_uses_default_repo(self) -> None:
        self.assertEqual(
            github_raw_url("LICENSE.md", repo_url="https://github.com/2high4schooltoday/despatch.git", repo_ref="main"),
            "https://raw.githubusercontent.com/2high4schooltoday/despatch/main/LICENSE.md",
        )

    def test_github_raw_url_accepts_git_ssh_remote(self) -> None:
        self.assertEqual(
            github_raw_url("scripts/tui/assistant_app.py", repo_url="git@github.com:2high4schooltoday/despatch.git", repo_ref="release/test"),
            "https://raw.githubusercontent.com/2high4schooltoday/despatch/release/test/scripts/tui/assistant_app.py",
        )

    def test_fetch_repo_text_uses_supplied_opener(self) -> None:
        class _FakeResponse:
            def __enter__(self) -> "_FakeResponse":
                return self

            def __exit__(self, exc_type, exc, tb) -> None:
                _ = exc_type, exc, tb

            def read(self) -> bytes:
                return b"license text"

        called: dict[str, object] = {}

        def fake_opener(request, timeout=0.0):  # type: ignore[no-untyped-def]
            called["url"] = request.full_url
            called["timeout"] = timeout
            return _FakeResponse()

        text = fetch_repo_text("LICENSE.md", opener=fake_opener, timeout=1.25)
        self.assertEqual(text, "license text")
        self.assertEqual(called["url"], "https://raw.githubusercontent.com/2high4schooltoday/despatch/main/LICENSE.md")
        self.assertEqual(called["timeout"], 1.25)

    def test_detect_letsencrypt_cert_pair_prefers_exact_domain(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            live = Path(td)
            exact = live / "mail.example.com"
            fallback = live / "example.com"
            exact.mkdir(parents=True)
            fallback.mkdir(parents=True)
            (exact / "fullchain.pem").write_text("cert", encoding="utf-8")
            (exact / "privkey.pem").write_text("key", encoding="utf-8")
            (fallback / "fullchain.pem").write_text("cert", encoding="utf-8")
            (fallback / "privkey.pem").write_text("key", encoding="utf-8")

            cert, key = detect_letsencrypt_cert_pair("mail.example.com", live)
            self.assertEqual(cert, str(exact / "fullchain.pem"))
            self.assertEqual(key, str(exact / "privkey.pem"))

    def test_detect_letsencrypt_cert_pair_falls_back_to_parent_domain(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            live = Path(td)
            parent = live / "example.com"
            parent.mkdir(parents=True)
            (parent / "fullchain.pem").write_text("cert", encoding="utf-8")
            (parent / "privkey.pem").write_text("key", encoding="utf-8")

            cert, key = detect_letsencrypt_cert_pair("mail.example.com", live)
            self.assertEqual(cert, str(parent / "fullchain.pem"))
            self.assertEqual(key, str(parent / "privkey.pem"))


class InstallerScriptTests(unittest.TestCase):
    def _script_text(self) -> str:
        return (Path(__file__).resolve().parents[2] / "scripts" / "auto_install.sh").read_text(encoding="utf-8")

    def _extract_function(self, name: str) -> str:
        lines = self._script_text().splitlines()
        start = None
        for index, line in enumerate(lines):
            if line.startswith(f"{name}() {{"):
                start = index
                break
        if start is None:
            self.fail(f"function {name} not found")
        depth = 0
        chunk: list[str] = []
        for line in lines[start:]:
            chunk.append(line)
            depth += line.count("{")
            depth -= line.count("}")
            if depth == 0:
                return "\n".join(chunk)
        self.fail(f"function {name} did not terminate")

    def _run_functions(self, names: list[str], body: str) -> str:
        script = "set -e\n" + "\n\n".join(self._extract_function(name) for name in names) + "\n\n" + body + "\n"
        proc = subprocess.run(
            ["bash", "-lc", script],
            capture_output=True,
            text=True,
            check=True,
        )
        return proc.stdout.strip()

    def test_detect_smtp_port_prefers_local_relay(self) -> None:
        out = self._run_functions(
            ["detect_smtp_port"],
            """
port_open() {
  [[ "$2" == "25" ]]
}
detect_smtp_port
""",
        )
        self.assertEqual(out, "25")

    def test_password_reset_sender_domain_strips_mail_prefix(self) -> None:
        out = self._run_functions(
            ["trim", "lower", "derive_password_reset_sender_domain"],
            """
printf '%s\n' "$(derive_password_reset_sender_domain 'mail.2h4s2d.ru')"
printf '%s\n' "$(derive_password_reset_sender_domain 'example.com')"
""",
        ).splitlines()
        self.assertEqual(out[0], "2h4s2d.ru")
        self.assertEqual(out[1], "example.com")

    def test_password_reset_base_url_prefers_public_proxy_host(self) -> None:
        out = self._run_functions(
            ["trim", "http_base_url_from_host_port", "derive_password_reset_base_url"],
            """
printf '%s\n' "$(derive_password_reset_base_url 'proxy' 'mail.2h4s2d.ru' '1' 'mail.2h4s2d.ru' '127.0.0.1:8080')"
printf '%s\n' "$(derive_password_reset_base_url 'direct' '' '0' '2h4s2d.ru' '127.0.0.1:8080')"
""",
        ).splitlines()
        self.assertEqual(out[0], "https://mail.2h4s2d.ru")
        self.assertEqual(out[1], "http://2h4s2d.ru:8080")


class FlowDefinitionTests(unittest.TestCase):
    def test_install_flow_has_license_before_scope(self) -> None:
        keys = [step.key for step in INSTALL_FLOW]
        self.assertEqual(keys, ["license", "scope", "network", "security", "review", "progress", "completion"])

    def test_uninstall_flow_has_review_progress_and_completion(self) -> None:
        keys = [step.key for step in UNINSTALL_FLOW]
        self.assertEqual(keys, ["backup", "removal", "review", "progress", "completion"])


if __name__ == "__main__":
    unittest.main()
