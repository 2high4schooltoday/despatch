from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Literal

Operation = Literal["install", "uninstall", "diagnose"]
StageStatus = Literal["pending", "running", "ok", "failed", "skipped"]
RunStatus = Literal["idle", "running", "ok", "failed", "partial", "cancelled"]


@dataclass(frozen=True)
class StageDef:
    stage_id: str
    title: str
    weight: int


INSTALL_STAGE_DEFS: tuple[StageDef, ...] = (
    StageDef("preflight", "Preflight", 10),
    StageDef("fetch_source", "Source Fetch / Bootstrap", 10),
    StageDef("deps", "Dependency Checks", 15),
    StageDef("build", "Build Binary", 15),
    StageDef("filesystem_and_user", "Filesystem and User Setup", 10),
    StageDef("env_generation", "Environment Generation", 10),
    StageDef("service_install_start", "Service Install and Start", 10),
    StageDef("firewall", "Firewall", 5),
    StageDef("proxy", "Reverse Proxy", 10),
    StageDef("post_checks", "Post-install Checks", 5),
    StageDef("final_summary", "Final Summary", 0),
)

UNINSTALL_STAGE_DEFS: tuple[StageDef, ...] = (
    StageDef("preflight", "Preflight", 10),
    StageDef("backups", "Backup Selected Files", 20),
    StageDef("service", "Service Teardown", 25),
    StageDef("cleanup", "Filesystem Cleanup", 35),
    StageDef("summary", "Final Summary", 10),
)

DIAG_STAGE_DEFS: tuple[StageDef, ...] = (
    StageDef("diagnostics", "Diagnostics", 100),
)


@dataclass
class InstallSpec:
    base_domain: str = "example.com"
    listen_addr: str = ":8080"
    install_service: bool = True
    proxy_setup: bool = True
    proxy_server: str = "nginx"
    proxy_server_name: str = ""
    proxy_tls: bool = False
    proxy_cert: str = ""
    proxy_key: str = ""
    proxy_cert_autofilled_for: str = field(default="", repr=False)
    proxy_key_autofilled_for: str = field(default="", repr=False)
    dovecot_auth_mode: str = "pam"
    dovecot_auth_db_driver: str = ""
    dovecot_auth_db_dsn: str = ""
    dovecot_auth_table: str = "users"
    dovecot_auth_email_col: str = "email"
    dovecot_auth_pass_col: str = "password_hash"
    dovecot_auth_active_col: str = ""
    dovecot_auth_maildir_col: str = ""
    ufw_enable: bool = False
    ufw_open_proxy_ports: bool = True
    ufw_open_direct_port: bool = True
    run_diagnose: bool = True
    auto_install_deps: bool = True
    install_git: bool = True

    def validate(self) -> list[str]:
        errors: list[str] = []
        proxy_server_name = self.proxy_server_name.strip().lower()
        if not self.base_domain or "." not in self.base_domain:
            errors.append("Base domain must be a valid FQDN.")
        if not self.listen_addr:
            errors.append("Listen address is required.")
        if self.proxy_setup:
            if self.proxy_server not in {"nginx", "apache2"}:
                errors.append("Proxy server must be nginx or apache2.")
            if not self.proxy_server_name:
                errors.append("Proxy server name is required when proxy mode is enabled.")
            if self.proxy_tls and (not self.proxy_cert or not self.proxy_key):
                errors.append("TLS cert and key are required when proxy TLS is enabled.")
            cert_host = self._letsencrypt_live_host(self.proxy_cert)
            key_host = self._letsencrypt_live_host(self.proxy_key)
            if proxy_server_name and cert_host and cert_host != proxy_server_name:
                errors.append("TLS certificate path must match the proxy server name when using /etc/letsencrypt/live.")
            if proxy_server_name and key_host and key_host != proxy_server_name:
                errors.append("TLS private key path must match the proxy server name when using /etc/letsencrypt/live.")
        if self.dovecot_auth_mode not in {"pam", "sql"}:
            errors.append("Dovecot auth mode must be pam or sql.")
        if self.dovecot_auth_mode == "sql":
            if not self.dovecot_auth_db_driver:
                errors.append("SQL auth driver is required for SQL mode.")
            if not self.dovecot_auth_db_dsn:
                errors.append("SQL auth DSN is required for SQL mode.")
        return errors

    @staticmethod
    def _letsencrypt_live_host(path: str) -> str:
        candidate = Path(path.strip())
        parts = candidate.parts
        try:
            live_idx = parts.index("live")
        except ValueError:
            return ""
        if live_idx < 3:
            return ""
        if parts[live_idx - 3 : live_idx] != ("/", "etc", "letsencrypt"):
            return ""
        if live_idx + 1 >= len(parts):
            return ""
        return parts[live_idx + 1].strip().lower()

    def to_env(self) -> dict[str, str]:
        return {
            "DESPATCH_NONINTERACTIVE": "1",
            "DESPATCH_BASE_DOMAIN": self.base_domain,
            "DESPATCH_LISTEN_ADDR": self.listen_addr,
            "DESPATCH_INSTALL_SERVICE": "1" if self.install_service else "0",
            "DESPATCH_PROXY_SETUP": "1" if self.proxy_setup else "0",
            "DESPATCH_PROXY_SERVER": self.proxy_server,
            "DESPATCH_PROXY_SERVER_NAME": self.proxy_server_name,
            "DESPATCH_PROXY_TLS": "1" if self.proxy_tls else "0",
            "DESPATCH_PROXY_CERT": self.proxy_cert,
            "DESPATCH_PROXY_KEY": self.proxy_key,
            "DESPATCH_DOVECOT_AUTH_MODE": self.dovecot_auth_mode,
            "DESPATCH_DOVECOT_AUTH_DB_DRIVER": self.dovecot_auth_db_driver,
            "DESPATCH_DOVECOT_AUTH_DB_DSN": self.dovecot_auth_db_dsn,
            "DESPATCH_DOVECOT_AUTH_TABLE": self.dovecot_auth_table,
            "DESPATCH_DOVECOT_AUTH_EMAIL_COL": self.dovecot_auth_email_col,
            "DESPATCH_DOVECOT_AUTH_PASS_COL": self.dovecot_auth_pass_col,
            "DESPATCH_DOVECOT_AUTH_ACTIVE_COL": self.dovecot_auth_active_col,
            "DESPATCH_DOVECOT_AUTH_MAILDIR_COL": self.dovecot_auth_maildir_col,
            "DESPATCH_UFW_ENABLE": "1" if self.ufw_enable else "0",
            "DESPATCH_UFW_OPEN_PROXY_PORTS": "1" if self.ufw_open_proxy_ports else "0",
            "DESPATCH_UFW_OPEN_DIRECT_PORT": "1" if self.ufw_open_direct_port else "0",
            "DESPATCH_RUN_DIAG": "1" if self.run_diagnose else "0",
            "DESPATCH_AUTO_INSTALL_DEPS": "1" if self.auto_install_deps else "0",
            "DESPATCH_INSTALL_GIT": "1" if self.install_git else "0",
            "DESPATCH_CONFIRM_CONTINUE": "1",
            "DESPATCH_PROXY_FALLBACK_DIRECT": "1",
            "DESPATCH_RETRY_TLS_PATHS": "0",
            "DESPATCH_SQL_MANUAL": "1" if self.dovecot_auth_mode == "sql" else "0",
            "DESPATCH_INSTALL_NGINX": "1",
            "DESPATCH_INSTALL_APACHE2": "1",
            "DESPATCH_CONFIRM_UNINSTALL": "0",
            "DESPATCH_REPO_URL": "",
            "DESPATCH_REPO_REF": "",
            "DESPATCH_CHECKOUT_DIR": "",
        }

    @staticmethod
    def _launchpad_scalar(value: object) -> str:
        if isinstance(value, bool):
            return "true" if value else "false"
        return str(value)

    def to_launchpad_set_args(self) -> list[str]:
        values: list[tuple[str, object]] = [
            ("release", "latest"),
            ("baseDomain", self.base_domain),
            ("listenAddr", self.listen_addr),
            ("installService", self.install_service),
            ("deployMode", "proxy" if self.proxy_setup else "direct"),
            ("proxyServer", self.proxy_server),
            ("proxyServerName", self.proxy_server_name),
            ("proxyTls", self.proxy_tls),
            ("proxyCert", self.proxy_cert),
            ("proxyKey", self.proxy_key),
            ("dovecotAuthMode", self.dovecot_auth_mode),
            ("dovecotAuthDbDriver", self.dovecot_auth_db_driver),
            ("dovecotAuthDbDsn", self.dovecot_auth_db_dsn),
            ("dovecotAuthTable", self.dovecot_auth_table),
            ("dovecotAuthEmailCol", self.dovecot_auth_email_col),
            ("dovecotAuthPassCol", self.dovecot_auth_pass_col),
            ("dovecotAuthActiveCol", self.dovecot_auth_active_col),
            ("dovecotAuthMaildirCol", self.dovecot_auth_maildir_col),
            ("autoInstallDeps", self.auto_install_deps),
            ("ufwEnable", self.ufw_enable),
            ("ufwOpenProxyPorts", self.ufw_open_proxy_ports),
            ("ufwOpenDirectPort", self.ufw_open_direct_port),
            ("enableCapCaptcha", False),
            ("capSiteKey", ""),
            ("capSecret", ""),
            ("capUpstream", ""),
            ("capAutoProvision", False),
            ("capImage", ""),
            ("capAdminKey", ""),
            ("capWidgetVersion", ""),
            ("capWasmVersion", ""),
            ("runDiagnose", self.run_diagnose),
        ]
        return [f"{key}={self._launchpad_scalar(value)}" for key, value in values]


@dataclass
class UninstallSpec:
    backup_env: bool = True
    backup_data: bool = True
    remove_app_files: bool = True
    remove_app_data: bool = True
    remove_system_user: bool = True
    remove_nginx_site: bool = True
    remove_apache_site: bool = True
    remove_checkout: bool = False

    def to_env(self) -> dict[str, str]:
        return {
            "DESPATCH_NONINTERACTIVE": "1",
            "DESPATCH_CONFIRM_UNINSTALL": "1",
            "DESPATCH_BACKUP_ENV": "1" if self.backup_env else "0",
            "DESPATCH_BACKUP_DATA": "1" if self.backup_data else "0",
            "DESPATCH_REMOVE_APP_FILES": "1" if self.remove_app_files else "0",
            "DESPATCH_REMOVE_APP_DATA": "1" if self.remove_app_data else "0",
            "DESPATCH_REMOVE_SYSTEM_USER": "1" if self.remove_system_user else "0",
            "DESPATCH_REMOVE_NGINX_SITE": "1" if self.remove_nginx_site else "0",
            "DESPATCH_REMOVE_APACHE_SITE": "1" if self.remove_apache_site else "0",
            "DESPATCH_REMOVE_CHECKOUT": "1" if self.remove_checkout else "0",
        }

    def to_launchpad_set_args(self) -> list[str]:
        values: list[tuple[str, object]] = [
            ("backupEnv", self.backup_env),
            ("backupData", self.backup_data),
            ("removeAppFiles", self.remove_app_files),
            ("removeAppData", self.remove_app_data),
            ("removeSystemUser", self.remove_system_user),
            ("removeNginxSite", self.remove_nginx_site),
            ("removeApacheSite", self.remove_apache_site),
            ("removeCheckout", self.remove_checkout),
        ]
        return [f"{key}={InstallSpec._launchpad_scalar(value)}" for key, value in values]


@dataclass
class DiagnoseSpec:
    pass


@dataclass
class StageState:
    stage_id: str
    title: str
    weight: int
    status: StageStatus = "pending"
    current: int = 0
    total: int = 1
    message: str = ""
    rate_hint: str = ""
    eta_hint: str = ""
    error_code: str = ""
    duration_ms: int = 0


@dataclass
class RunnerError:
    code: str
    message: str
    stage_id: str
    suggested_fix: str = ""


@dataclass
class OperationResult:
    status: Literal["ok", "failed", "partial", "cancelled"]
    errors: list[RunnerError] = field(default_factory=list)
    artifacts: dict[str, str] = field(default_factory=dict)
    next_actions: list[str] = field(default_factory=list)


@dataclass
class RunState:
    run_id: str = ""
    operation: Operation = "diagnose"
    status: RunStatus = "idle"
    stages: dict[str, StageState] = field(default_factory=dict)
    stage_order: list[str] = field(default_factory=list)
    active_stage_id: str = ""
    failed_stage: str = ""
    exit_code: int = 0

    @property
    def overall_progress(self) -> float:
        total_weight = sum(max(s.weight, 0) for s in self.stages.values())
        if total_weight <= 0:
            return 0.0
        done = 0.0
        for stage in self.stages.values():
            weight = max(stage.weight, 0)
            if stage.status in {"ok", "skipped"}:
                done += weight
                continue
            if stage.status == "running" and stage.total > 0:
                done += min(1.0, max(0.0, stage.current / stage.total)) * weight
        return max(0.0, min(1.0, done / total_weight))


@dataclass
class AppPaths:
    root_dir: Path
    scripts_dir: Path
    install_script: Path
    uninstall_script: Path
    diagnose_script: Path
