use chrono::{SecondsFormat, Utc};
use contracts::updater::{
    sanitize_path_token, ApplyRequest, ApplyStatus, APPLY_STATE_COMPLETED, APPLY_STATE_FAILED,
    APPLY_STATE_IN_PROGRESS, APPLY_STATE_ROLLED_BACK,
};
use nix::unistd::{chown, Gid, Uid, User};
use percent_encoding::{utf8_percent_encode, NON_ALPHANUMERIC};
use regex::Regex;
use reqwest::blocking::Client;
use reqwest::header::{ACCEPT, AUTHORIZATION, IF_NONE_MATCH, USER_AGENT};
use serde::de::DeserializeOwned;
use serde::Deserialize;
use serde_json::Value;
use sha2::{Digest, Sha256};
use std::env;
use std::fs::{self, File, OpenOptions};
use std::io::{self, Write};
use std::net::{IpAddr, Ipv4Addr};
use std::os::unix::fs::{OpenOptionsExt, PermissionsExt};
use std::path::{Component, Path, PathBuf};
use std::process::{Command, Stdio};
use std::thread;
use std::time::{Duration, Instant};
use tar::Archive;
use thiserror::Error;
use wait_timeout::ChildExt;
use walkdir::WalkDir;

#[derive(Debug, Clone)]
struct Config {
    update_enabled: bool,
    update_repo_owner: String,
    update_repo_name: String,
    update_http_timeout_sec: u64,
    update_github_token: String,
    update_backup_keep: usize,
    update_base_dir: PathBuf,
    update_install_dir: PathBuf,
    update_service_name: String,
    update_systemd_unit_dir: PathBuf,
    listen_addr: String,
    mailsec_enabled: bool,
    mailsec_socket: PathBuf,
}

#[derive(Debug, Deserialize, Clone)]
struct GithubRelease {
    tag_name: String,
    #[allow(dead_code)]
    name: String,
    #[allow(dead_code)]
    published_at: Option<String>,
    #[allow(dead_code)]
    html_url: Option<String>,
    draft: bool,
    assets: Vec<GithubReleaseAsset>,
}

#[derive(Debug, Deserialize, Clone)]
struct GithubReleaseAsset {
    name: String,
    browser_download_url: String,
}

#[derive(Debug)]
struct ApplyResult {
    to_version: String,
    rolled_back: bool,
}

#[derive(Debug, Error)]
enum WorkerError {
    #[error("{0}")]
    Message(String),
    #[error(transparent)]
    Io(#[from] io::Error),
    #[error(transparent)]
    Http(#[from] reqwest::Error),
    #[error(transparent)]
    Json(#[from] serde_json::Error),
    #[error(transparent)]
    Url(#[from] url::ParseError),
    #[error(transparent)]
    Nix(#[from] nix::Error),
}

struct PathCleanup {
    path: PathBuf,
}

impl PathCleanup {
    fn new(path: PathBuf) -> Self {
        Self { path }
    }
}

impl Drop for PathCleanup {
    fn drop(&mut self) {
        let _ = fs::remove_file(&self.path);
    }
}

fn main() {
    if let Err(err) = run() {
        eprintln!("update worker failed: {err}");
        std::process::exit(1);
    }
}

fn run() -> Result<(), WorkerError> {
    let cfg = Config::from_env()?;
    if !cfg.update_enabled {
        return Ok(());
    }

    ensure_dirs(
        &[
            request_dir(&cfg),
            status_dir(&cfg),
            lock_dir(&cfg),
            work_dir(&cfg),
            backups_dir(&cfg),
        ],
        0o750,
    )?;

    let lock_path = lock_path(&cfg);
    let mut lock_file = match OpenOptions::new()
        .create_new(true)
        .write(true)
        .mode(0o640)
        .open(&lock_path)
    {
        Ok(file) => file,
        Err(err) if err.kind() == io::ErrorKind::AlreadyExists => return Ok(()),
        Err(err) => return Err(WorkerError::Io(err)),
    };
    writeln!(
        lock_file,
        "pid={} started_at={}",
        std::process::id(),
        now_rfc3339()
    )?;
    drop(lock_file);
    let _lock_guard = PathCleanup::new(lock_path);

    let req_path = request_path(&cfg);
    let mut req: ApplyRequest = match read_json_file(&req_path) {
        Ok(value) => value,
        Err(err) if err.kind() == io::ErrorKind::NotFound => return Ok(()),
        Err(err) => return Err(WorkerError::Io(err)),
    };

    if req.request_id.trim().is_empty() {
        req.request_id = format!("update-{}", Utc::now().timestamp());
    }
    if req.requested_at.trim().is_empty() {
        req.requested_at = now_rfc3339();
    }

    let started_at = now_rfc3339();
    let target_version = req.target_version.trim().to_string();

    let mut final_status = ApplyStatus {
        state: APPLY_STATE_FAILED.to_string(),
        request_id: req.request_id.clone(),
        requested_at: req.requested_at.clone(),
        started_at: started_at.clone(),
        finished_at: String::new(),
        target_version: target_version.clone(),
        from_version: current_version(),
        to_version: String::new(),
        rolled_back: false,
        error: String::new(),
    };

    write_status(
        &cfg,
        &ApplyStatus {
            state: APPLY_STATE_IN_PROGRESS.to_string(),
            request_id: req.request_id.clone(),
            requested_at: req.requested_at.clone(),
            started_at: started_at.clone(),
            finished_at: String::new(),
            target_version: target_version.clone(),
            from_version: current_version(),
            to_version: String::new(),
            rolled_back: false,
            error: String::new(),
        },
    )?;

    let apply_result = apply_release(&cfg, &req);
    match apply_result {
        Ok(result) => {
            final_status.state = APPLY_STATE_COMPLETED.to_string();
            final_status.to_version = result.to_version;
            final_status.rolled_back = result.rolled_back;
        }
        Err(err) => {
            if matches!(err, WorkerError::Message(_))
                && err.to_string().contains("invalid target version")
            {
                final_status.error = "invalid target version".to_string();
            } else {
                final_status.error = err.to_string();
            }
            if final_status.rolled_back {
                final_status.state = APPLY_STATE_ROLLED_BACK.to_string();
            }
            final_status.finished_at = now_rfc3339();
            let _ = write_status(&cfg, &final_status);
            let _ = fs::remove_file(&req_path);
            return Err(err);
        }
    }

    final_status.finished_at = now_rfc3339();
    write_status(&cfg, &final_status)?;
    let _ = fs::remove_file(&req_path);
    Ok(())
}

impl Config {
    fn from_env() -> Result<Self, WorkerError> {
        Ok(Self {
            update_enabled: env_bool("UPDATE_ENABLED", true),
            update_repo_owner: env_var("UPDATE_REPO_OWNER", "2high4schooltoday"),
            update_repo_name: env_var("UPDATE_REPO_NAME", "new-mail-client"),
            update_http_timeout_sec: env_u64("UPDATE_HTTP_TIMEOUT_SEC", 10)?,
            update_github_token: env::var("UPDATE_GITHUB_TOKEN").unwrap_or_default(),
            update_backup_keep: env_u64("UPDATE_BACKUP_KEEP", 3)? as usize,
            update_base_dir: PathBuf::from(env_var(
                "UPDATE_BASE_DIR",
                "/var/lib/mailclient/update",
            )),
            update_install_dir: PathBuf::from(env_var("UPDATE_INSTALL_DIR", "/opt/mailclient")),
            update_service_name: env_var("UPDATE_SERVICE_NAME", "mailclient"),
            update_systemd_unit_dir: PathBuf::from(env_var(
                "UPDATE_SYSTEMD_UNIT_DIR",
                "/etc/systemd/system",
            )),
            listen_addr: env_var("LISTEN_ADDR", ":8080"),
            mailsec_enabled: env_bool("MAILSEC_ENABLED", false),
            mailsec_socket: PathBuf::from(env_var(
                "MAILSEC_SOCKET",
                "/run/mailclient/mailsec.sock",
            )),
        })
    }
}

fn env_var(key: &str, default: &str) -> String {
    match env::var(key) {
        Ok(v) if !v.trim().is_empty() => v,
        _ => default.to_string(),
    }
}

fn env_bool(key: &str, default: bool) -> bool {
    match env::var(key) {
        Ok(v) => {
            let norm = v.trim().to_lowercase();
            matches!(norm.as_str(), "1" | "true" | "yes" | "y" | "on")
        }
        Err(_) => default,
    }
}

fn env_u64(key: &str, default: u64) -> Result<u64, WorkerError> {
    match env::var(key) {
        Ok(v) => {
            let parsed = v
                .trim()
                .parse::<u64>()
                .map_err(|_| WorkerError::Message(format!("{key} must be a positive integer")))?;
            if parsed == 0 {
                return Err(WorkerError::Message(format!("{key} must be positive")));
            }
            Ok(parsed)
        }
        Err(_) => Ok(default),
    }
}

fn now_rfc3339() -> String {
    Utc::now().to_rfc3339_opts(SecondsFormat::Secs, true)
}

fn current_version() -> String {
    option_env!("MAILCLIENT_BUILD_VERSION")
        .unwrap_or("dev")
        .trim()
        .to_string()
}

fn request_dir(cfg: &Config) -> PathBuf {
    cfg.update_base_dir.join("request")
}

fn status_dir(cfg: &Config) -> PathBuf {
    cfg.update_base_dir.join("status")
}

fn lock_dir(cfg: &Config) -> PathBuf {
    cfg.update_base_dir.join("lock")
}

fn work_dir(cfg: &Config) -> PathBuf {
    cfg.update_base_dir.join("work")
}

fn backups_dir(cfg: &Config) -> PathBuf {
    cfg.update_base_dir.join("backups")
}

fn request_path(cfg: &Config) -> PathBuf {
    request_dir(cfg).join("update-request.json")
}

fn status_path(cfg: &Config) -> PathBuf {
    status_dir(cfg).join("update-status.json")
}

fn lock_path(cfg: &Config) -> PathBuf {
    lock_dir(cfg).join("update.lock")
}

fn ensure_dirs(paths: &[PathBuf], mode: u32) -> Result<(), WorkerError> {
    for path in paths {
        fs::create_dir_all(path)?;
        fs::set_permissions(path, fs::Permissions::from_mode(mode))?;
    }
    Ok(())
}

fn read_json_file<T: DeserializeOwned>(path: &Path) -> Result<T, io::Error> {
    let raw = fs::read(path)?;
    serde_json::from_slice(&raw).map_err(|err| io::Error::new(io::ErrorKind::InvalidData, err))
}

fn write_json_atomic(path: &Path, payload: &Value, mode: u32) -> Result<(), WorkerError> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)?;
    }
    let mut raw = serde_json::to_string_pretty(payload)?;
    raw.push('\n');

    let tmp_path = path.with_extension("tmp");
    {
        let mut file = OpenOptions::new()
            .create(true)
            .truncate(true)
            .write(true)
            .mode(mode)
            .open(&tmp_path)?;
        file.write_all(raw.as_bytes())?;
        file.sync_all()?;
    }
    fs::rename(&tmp_path, path)?;
    fs::set_permissions(path, fs::Permissions::from_mode(mode))?;
    Ok(())
}

fn write_status(cfg: &Config, status: &ApplyStatus) -> Result<(), WorkerError> {
    let mut status = status.clone();
    if status.state.trim().is_empty() {
        status.state = APPLY_STATE_FAILED.to_string();
    }
    let payload = serde_json::to_value(&status)?;
    write_json_atomic(&status_path(cfg), &payload, 0o640)?;
    ensure_mailclient_readable(&status_path(cfg))?;
    Ok(())
}

fn ensure_mailclient_readable(path: &Path) -> Result<(), WorkerError> {
    let user = lookup_mailclient_user()?;
    chown(
        path,
        Some(Uid::from_raw(user.uid.as_raw())),
        Some(Gid::from_raw(user.gid.as_raw())),
    )?;
    fs::set_permissions(path, fs::Permissions::from_mode(0o640))?;
    Ok(())
}

fn apply_release(cfg: &Config, req: &ApplyRequest) -> Result<ApplyResult, WorkerError> {
    let target = req.target_version.trim();
    if !target.is_empty() {
        let rx =
            Regex::new(r"^[A-Za-z0-9._-]+$").map_err(|e| WorkerError::Message(e.to_string()))?;
        if !rx.is_match(target) {
            return Err(WorkerError::Message("invalid target version".to_string()));
        }
    }

    let release = resolve_release(cfg, target)?;
    let arch = env::consts::ARCH;
    let (archive_name, archive_url) = resolve_archive_asset(&release, arch).ok_or_else(|| {
        WorkerError::Message(format!(
            "release archive for GOARCH={arch} not found (available: {})",
            release
                .assets
                .iter()
                .map(|a| a.name.trim())
                .collect::<Vec<_>>()
                .join(", ")
        ))
    })?;

    let checksum_url = release
        .assets
        .iter()
        .find(|a| a.name.trim().eq_ignore_ascii_case("checksums.txt"))
        .map(|a| a.browser_download_url.trim().to_string())
        .ok_or_else(|| WorkerError::Message("release asset checksums.txt not found".to_string()))?;

    let run_id = if req.request_id.trim().is_empty() {
        format!("run-{}", Utc::now().timestamp())
    } else {
        req.request_id.trim().to_string()
    };

    let run_work = work_dir(cfg).join(sanitize_path_token(&run_id));
    let _ = fs::remove_dir_all(&run_work);
    fs::create_dir_all(&run_work)?;

    let archive_path = run_work.join(&archive_name);
    let checksum_path = run_work.join("checksums.txt");
    download_asset(cfg, &archive_url, &archive_path)?;
    download_asset(cfg, &checksum_url, &checksum_path)?;
    verify_checksum_file(&checksum_path, &archive_path, &archive_name)?;

    let extracted = run_work.join("extract");
    extract_tar_gz(&archive_path, &extracted)?;
    let payload_root = find_payload_root(&extracted)?;

    let stage_dir = cfg
        .update_install_dir
        .join(format!(".update-stage-{}", sanitize_path_token(&run_id)));
    let _ = fs::remove_dir_all(&stage_dir);
    fs::create_dir_all(&stage_dir)?;

    copy_file(
        &payload_root.join("mailclient"),
        &stage_dir.join("mailclient"),
        0o755,
    )?;
    copy_file(
        &payload_root.join("mailclient-pam-reset-helper"),
        &stage_dir.join("mailclient-pam-reset-helper"),
        0o755,
    )?;
    copy_file(
        &payload_root.join("mailclient-update-worker"),
        &stage_dir.join("mailclient-update-worker"),
        0o755,
    )?;
    copy_dir(&payload_root.join("web"), &stage_dir.join("web"))?;
    copy_dir(
        &payload_root.join("migrations"),
        &stage_dir.join("migrations"),
    )?;
    let mailsec_payload = payload_root.join("mailclient-mailsec-service");
    let mailsec_payload_present = mailsec_payload.exists();
    if mailsec_payload_present {
        copy_file(
            &mailsec_payload,
            &stage_dir.join("mailclient-mailsec-service"),
            0o755,
        )?;
    }
    let mailsec_unit_path = payload_root
        .join("deploy")
        .join("mailclient-mailsec.service");
    let mailsec_unit_present = mailsec_unit_path.exists();
    let current_mailsec = cfg.update_install_dir.join("mailclient-mailsec-service");
    let current_mailsec_present = current_mailsec.exists();
    let installed_mailsec_unit = cfg
        .update_systemd_unit_dir
        .join("mailclient-mailsec.service");
    let installed_mailsec_unit_present = installed_mailsec_unit.exists();
    let install_deploy_mailsec_unit = cfg
        .update_install_dir
        .join("deploy")
        .join("mailclient-mailsec.service");
    let install_deploy_mailsec_unit_present = install_deploy_mailsec_unit.exists();

    if cfg.mailsec_enabled && !mailsec_payload_present && !current_mailsec_present {
        return Err(WorkerError::Message(
            "mailsec is enabled but mailclient-mailsec-service is missing in both current install and release payload".to_string(),
        ));
    }
    if cfg.mailsec_enabled
        && !mailsec_unit_present
        && !installed_mailsec_unit_present
        && !install_deploy_mailsec_unit_present
    {
        return Err(WorkerError::Message(
            "mailsec is enabled but mailclient-mailsec.service is missing in payload, install deploy/, and systemd unit directory".to_string(),
        ));
    }

    let prev_bin = cfg
        .update_install_dir
        .join(format!(".prev-mailclient-{}", sanitize_path_token(&run_id)));
    let prev_pam = cfg.update_install_dir.join(format!(
        ".prev-pam-reset-helper-{}",
        sanitize_path_token(&run_id)
    ));
    let prev_worker = cfg.update_install_dir.join(format!(
        ".prev-update-worker-{}",
        sanitize_path_token(&run_id)
    ));
    let prev_web = cfg
        .update_install_dir
        .join(format!(".prev-web-{}", sanitize_path_token(&run_id)));
    let prev_mig = cfg
        .update_install_dir
        .join(format!(".prev-migrations-{}", sanitize_path_token(&run_id)));
    let prev_mailsec = cfg
        .update_install_dir
        .join(format!(".prev-mailsec-{}", sanitize_path_token(&run_id)));

    let current_bin = cfg.update_install_dir.join("mailclient");
    let current_pam = cfg.update_install_dir.join("mailclient-pam-reset-helper");
    let current_worker = cfg.update_install_dir.join("mailclient-update-worker");
    let current_web = cfg.update_install_dir.join("web");
    let current_mig = cfg.update_install_dir.join("migrations");

    let stage_bin = stage_dir.join("mailclient");
    let stage_pam = stage_dir.join("mailclient-pam-reset-helper");
    let stage_worker = stage_dir.join("mailclient-update-worker");
    let stage_web = stage_dir.join("web");
    let stage_mig = stage_dir.join("migrations");
    let stage_mailsec = stage_dir.join("mailclient-mailsec-service");

    let _ = fs::remove_file(&prev_bin);
    let _ = fs::remove_file(&prev_pam);
    let _ = fs::remove_file(&prev_worker);
    let _ = fs::remove_dir_all(&prev_web);
    let _ = fs::remove_dir_all(&prev_mig);
    let _ = fs::remove_file(&prev_mailsec);

    let swap_items = vec![
        (current_bin.clone(), stage_bin, prev_bin.clone()),
        (current_pam.clone(), stage_pam, prev_pam.clone()),
        (current_worker.clone(), stage_worker, prev_worker.clone()),
        (current_web.clone(), stage_web, prev_web.clone()),
        (current_mig.clone(), stage_mig, prev_mig.clone()),
    ];

    let mut swapped: Vec<(PathBuf, PathBuf)> = Vec::new();
    for (current, staged, previous) in &swap_items {
        if let Err(err) = swap_path(current, staged, previous) {
            rollback_paths(&swapped)?;
            return Err(err);
        }
        swapped.push((current.clone(), previous.clone()));
    }
    if mailsec_payload_present {
        if let Err(err) = swap_path_optional(&current_mailsec, &stage_mailsec, &prev_mailsec) {
            rollback_paths(&swapped)?;
            return Err(err);
        }
        swapped.push((current_mailsec.clone(), prev_mailsec.clone()));
    }

    if let Err(err) = chown_runtime_artifacts(&cfg.update_install_dir) {
        rollback_paths(&swapped)?;
        return Err(WorkerError::Message(format!("chown failed: {err}")));
    }
    let mut mailsec_unit_source: Option<PathBuf> = None;
    if mailsec_unit_present {
        mailsec_unit_source = Some(mailsec_unit_path.clone());
    } else if install_deploy_mailsec_unit_present {
        mailsec_unit_source = Some(install_deploy_mailsec_unit.clone());
    }
    let mailsec_unit_dst = cfg
        .update_systemd_unit_dir
        .join("mailclient-mailsec.service");
    if let Some(src) = mailsec_unit_source {
        if let Err(err) = copy_file(&src, &mailsec_unit_dst, 0o644) {
            rollback_paths(&swapped)?;
            return Err(WorkerError::Message(format!(
                "mailsec unit install failed: {err}"
            )));
        }
        if let Err(err) = run_cmd("systemctl", &["daemon-reload"], Duration::from_secs(60)) {
            rollback_paths(&swapped)?;
            return Err(WorkerError::Message(format!(
                "mailsec daemon-reload failed: {err}"
            )));
        }
    }
    let mailsec_unit_now_present = mailsec_unit_dst.exists();
    if cfg.mailsec_enabled && !mailsec_unit_now_present {
        rollback_paths(&swapped)?;
        return Err(WorkerError::Message(
            "mailsec is enabled but systemd unit mailclient-mailsec.service is still missing after update".to_string(),
        ));
    }
    if cfg.mailsec_enabled
        || mailsec_payload_present
        || mailsec_unit_present
        || mailsec_unit_now_present
    {
        if let Err(err) = run_cmd(
            "systemctl",
            &["enable", "--now", "mailclient-mailsec"],
            Duration::from_secs(60),
        ) {
            rollback_paths(&swapped)?;
            return Err(WorkerError::Message(format!(
                "mailsec enable failed: {err}"
            )));
        }
        if cfg.mailsec_enabled && !wait_for_path(&cfg.mailsec_socket, Duration::from_secs(10)) {
            rollback_paths(&swapped)?;
            return Err(WorkerError::Message(format!(
                "mailsec is enabled but socket was not created at {} after start",
                cfg.mailsec_socket.display()
            )));
        }
    }

    if let Err(err) = run_cmd(
        "systemctl",
        &["restart", cfg.update_service_name.trim()],
        Duration::from_secs(60),
    ) {
        rollback_paths(&swapped)?;
        let _ = run_cmd(
            "systemctl",
            &["restart", cfg.update_service_name.trim()],
            Duration::from_secs(60),
        );
        return Err(WorkerError::Message(format!(
            "service restart failed: {err}"
        )));
    }

    if let Err(err) = check_service_health(&cfg.listen_addr) {
        rollback_paths(&swapped)?;
        let _ = run_cmd(
            "systemctl",
            &["restart", cfg.update_service_name.trim()],
            Duration::from_secs(60),
        );
        return Err(WorkerError::Message(format!("health check failed: {err}")));
    }

    let backup_dest = backups_dir(cfg).join(format!(
        "{}-{}",
        Utc::now().format("%Y%m%dT%H%M%S"),
        sanitize_path_token(&run_id)
    ));
    fs::create_dir_all(&backup_dest)?;
    move_if_exists(&prev_bin, &backup_dest.join("mailclient"))?;
    move_if_exists(&prev_pam, &backup_dest.join("mailclient-pam-reset-helper"))?;
    move_if_exists(&prev_worker, &backup_dest.join("mailclient-update-worker"))?;
    move_if_exists(&prev_web, &backup_dest.join("web"))?;
    move_if_exists(&prev_mig, &backup_dest.join("migrations"))?;
    move_if_exists(
        &prev_mailsec,
        &backup_dest.join("mailclient-mailsec-service"),
    )?;

    trim_backups(&backups_dir(cfg), cfg.update_backup_keep)?;

    Ok(ApplyResult {
        to_version: release.tag_name.trim().to_string(),
        rolled_back: false,
    })
}

fn resolve_release(cfg: &Config, target_version: &str) -> Result<GithubRelease, WorkerError> {
    if !target_version.is_empty() {
        return release_by_tag(cfg, target_version);
    }

    match latest_release(cfg, None) {
        Ok(rel) => Ok(rel),
        Err(WorkerError::Message(msg)) if msg.contains("status 404") => {
            latest_any_release(cfg, None)
        }
        Err(err) => Err(err),
    }
}

fn latest_release(cfg: &Config, etag: Option<&str>) -> Result<GithubRelease, WorkerError> {
    let path = format!(
        "/repos/{}/{}/releases/latest",
        path_encode(&cfg.update_repo_owner),
        path_encode(&cfg.update_repo_name)
    );
    request_json(cfg, &path, etag)
}

fn latest_any_release(cfg: &Config, etag: Option<&str>) -> Result<GithubRelease, WorkerError> {
    let path = format!(
        "/repos/{}/{}/releases?per_page=10",
        path_encode(&cfg.update_repo_owner),
        path_encode(&cfg.update_repo_name)
    );
    let list: Vec<GithubRelease> = request_json(cfg, &path, etag)?;
    for rel in list {
        if rel.draft {
            continue;
        }
        if rel.tag_name.trim().is_empty() {
            continue;
        }
        return Ok(rel);
    }
    Err(WorkerError::Message(
        "github releases list is empty".to_string(),
    ))
}

fn release_by_tag(cfg: &Config, tag: &str) -> Result<GithubRelease, WorkerError> {
    let path = format!(
        "/repos/{}/{}/releases/tags/{}",
        path_encode(&cfg.update_repo_owner),
        path_encode(&cfg.update_repo_name),
        path_encode(tag)
    );
    request_json(cfg, &path, None)
}

fn path_encode(value: &str) -> String {
    utf8_percent_encode(value, NON_ALPHANUMERIC).to_string()
}

fn request_json<T: DeserializeOwned>(
    cfg: &Config,
    path: &str,
    etag: Option<&str>,
) -> Result<T, WorkerError> {
    let url = format!("https://api.github.com{path}");
    let client = Client::builder()
        .timeout(Duration::from_secs(cfg.update_http_timeout_sec))
        .build()?;

    let mut req = client
        .get(url)
        .header(ACCEPT, "application/vnd.github+json")
        .header(USER_AGENT, "mailclient-updater/1");

    if let Some(etag) = etag {
        if !etag.trim().is_empty() {
            req = req.header(IF_NONE_MATCH, etag.trim());
        }
    }
    if !cfg.update_github_token.trim().is_empty() {
        req = req.header(
            AUTHORIZATION,
            format!("Bearer {}", cfg.update_github_token.trim()),
        );
    }

    let resp = req.send()?;
    let status = resp.status().as_u16();
    if status != 200 {
        return Err(WorkerError::Message(format!(
            "github api returned status {status}"
        )));
    }
    resp.json::<T>().map_err(WorkerError::from)
}

fn archive_asset_candidates(arch: &str) -> Vec<String> {
    let mut aliases = vec![arch.trim().to_ascii_lowercase()];
    match arch.trim().to_ascii_lowercase().as_str() {
        "amd64" | "x86_64" | "x64" => {
            aliases.extend(["amd64", "x86_64", "x64"].iter().map(|s| s.to_string()));
        }
        "arm64" | "aarch64" => {
            aliases.extend(["arm64", "aarch64"].iter().map(|s| s.to_string()));
        }
        _ => {}
    }

    aliases.sort();
    aliases.dedup();

    let mut out = Vec::new();
    for alias in aliases {
        out.push(format!("mailclient-linux-{alias}.tar.gz"));
        out.push(format!("mailclient-linux-{alias}.tgz"));
        out.push(format!("mailclient_{alias}_linux.tar.gz"));
        out.push(format!("mailclient_{alias}_linux.tgz"));
    }
    out
}

fn resolve_archive_asset(release: &GithubRelease, arch: &str) -> Option<(String, String)> {
    let candidates = archive_asset_candidates(arch);
    for wanted in candidates {
        if let Some(asset) = release.assets.iter().find(|a| {
            a.name.trim().eq_ignore_ascii_case(&wanted) && !a.browser_download_url.trim().is_empty()
        }) {
            return Some((
                asset.name.trim().to_string(),
                asset.browser_download_url.trim().to_string(),
            ));
        }
    }
    None
}

fn download_asset(cfg: &Config, raw_url: &str, dest: &Path) -> Result<(), WorkerError> {
    let parsed = url::Url::parse(raw_url)?;
    if parsed.scheme() != "https" {
        return Err(WorkerError::Message(
            "refusing non-https asset url".to_string(),
        ));
    }

    let client = Client::builder()
        .timeout(Duration::from_secs(cfg.update_http_timeout_sec))
        .build()?;

    let mut req = client
        .get(parsed)
        .header(USER_AGENT, "mailclient-updater/1");

    if !cfg.update_github_token.trim().is_empty() {
        req = req.header(
            AUTHORIZATION,
            format!("Bearer {}", cfg.update_github_token.trim()),
        );
    }

    let mut resp = req.send()?;
    if !resp.status().is_success() {
        return Err(WorkerError::Message(format!(
            "asset download failed with status {}",
            resp.status().as_u16()
        )));
    }

    if let Some(parent) = dest.parent() {
        fs::create_dir_all(parent)?;
    }
    let mut out = OpenOptions::new()
        .create(true)
        .truncate(true)
        .write(true)
        .mode(0o640)
        .open(dest)?;
    io::copy(&mut resp, &mut out)?;
    Ok(())
}

fn verify_checksum_file(
    checksum_path: &Path,
    archive_path: &Path,
    archive_name: &str,
) -> Result<(), WorkerError> {
    let raw = fs::read_to_string(checksum_path)?;
    let mut expected = String::new();

    for line in raw.lines() {
        let fields: Vec<_> = line.split_whitespace().collect();
        if fields.len() < 2 {
            continue;
        }
        if fields[1] == archive_name || fields[1].trim_start_matches('*') == archive_name {
            expected = fields[0].to_string();
            break;
        }
    }

    if expected.is_empty() {
        return Err(WorkerError::Message(format!(
            "checksum for {archive_name} not found"
        )));
    }

    let mut file = File::open(archive_path)?;
    let mut hasher = Sha256::new();
    io::copy(&mut file, &mut hasher)?;
    let got = format!("{:x}", hasher.finalize());

    if !expected.eq_ignore_ascii_case(got.trim()) {
        return Err(WorkerError::Message(format!(
            "checksum mismatch for {archive_name}"
        )));
    }
    Ok(())
}

fn extract_tar_gz(src: &Path, dest: &Path) -> Result<(), WorkerError> {
    fs::create_dir_all(dest)?;

    let file = File::open(src)?;
    let decoder = flate2::read::GzDecoder::new(file);
    let mut archive = Archive::new(decoder);

    for entry in archive.entries()? {
        let mut entry = entry?;
        let rel_path = entry.path()?.to_path_buf();
        let clean_rel = sanitize_archive_relative(&rel_path)?;
        let target = dest.join(&clean_rel);

        if !target.starts_with(dest) {
            return Err(WorkerError::Message(format!(
                "invalid archive path: {}",
                rel_path.display()
            )));
        }

        let entry_type = entry.header().entry_type();
        if entry_type.is_dir() {
            fs::create_dir_all(&target)?;
            continue;
        }
        if !entry_type.is_file() {
            continue;
        }

        if let Some(parent) = target.parent() {
            fs::create_dir_all(parent)?;
        }
        entry.unpack(&target)?;
    }

    Ok(())
}

fn sanitize_archive_relative(path: &Path) -> Result<PathBuf, WorkerError> {
    let mut out = PathBuf::new();
    for component in path.components() {
        match component {
            Component::Normal(part) => out.push(part),
            Component::CurDir => {}
            Component::ParentDir | Component::RootDir | Component::Prefix(_) => {
                return Err(WorkerError::Message(format!(
                    "invalid archive path: {}",
                    path.display()
                )))
            }
        }
    }
    if out.as_os_str().is_empty() {
        return Err(WorkerError::Message(format!(
            "invalid archive path: {}",
            path.display()
        )));
    }
    Ok(out)
}

fn find_payload_root(extract_dir: &Path) -> Result<PathBuf, WorkerError> {
    let required = [
        "mailclient",
        "mailclient-pam-reset-helper",
        "mailclient-update-worker",
        "web",
        "migrations",
    ];

    if has_required_paths(extract_dir, &required) {
        return Ok(extract_dir.to_path_buf());
    }

    let entries: Vec<_> = fs::read_dir(extract_dir)?.filter_map(Result::ok).collect();
    if entries.len() == 1 && entries[0].file_type()?.is_dir() {
        let root = extract_dir.join(entries[0].file_name());
        if has_required_paths(&root, &required) {
            return Ok(root);
        }
    }

    Err(WorkerError::Message(
        "release payload missing required files (mailclient, mailclient-pam-reset-helper, mailclient-update-worker, web, migrations)".to_string(),
    ))
}

fn has_required_paths(root: &Path, required: &[&str]) -> bool {
    required.iter().all(|rel| root.join(rel).exists())
}

fn copy_file(src: &Path, dst: &Path, mode: u32) -> Result<(), WorkerError> {
    if let Some(parent) = dst.parent() {
        fs::create_dir_all(parent)?;
    }

    let mut in_file = File::open(src)?;
    let mut out_file = OpenOptions::new()
        .create(true)
        .truncate(true)
        .write(true)
        .mode(mode)
        .open(dst)?;

    io::copy(&mut in_file, &mut out_file)?;
    fs::set_permissions(dst, fs::Permissions::from_mode(mode))?;
    Ok(())
}

fn copy_dir(src: &Path, dst: &Path) -> Result<(), WorkerError> {
    for entry in WalkDir::new(src) {
        let entry = entry.map_err(|e| WorkerError::Message(e.to_string()))?;
        let rel = entry
            .path()
            .strip_prefix(src)
            .map_err(|e| WorkerError::Message(e.to_string()))?;
        let target = dst.join(rel);

        if entry.file_type().is_dir() {
            fs::create_dir_all(&target)?;
            continue;
        }

        if let Some(parent) = target.parent() {
            fs::create_dir_all(parent)?;
        }

        let mode = entry
            .metadata()
            .map_err(|e| WorkerError::Message(e.to_string()))?
            .permissions()
            .mode();
        copy_file(entry.path(), &target, mode)?;
    }
    Ok(())
}

fn swap_path(current: &Path, staged: &Path, previous: &Path) -> Result<(), WorkerError> {
    fs::rename(current, previous)?;
    if let Err(err) = fs::rename(staged, current) {
        let _ = fs::rename(previous, current);
        return Err(WorkerError::Io(err));
    }
    Ok(())
}

fn swap_path_optional(current: &Path, staged: &Path, previous: &Path) -> Result<(), WorkerError> {
    let current_exists = current.exists();
    if current_exists {
        fs::rename(current, previous)?;
    }
    if let Err(err) = fs::rename(staged, current) {
        if current_exists {
            let _ = fs::rename(previous, current);
        }
        return Err(WorkerError::Io(err));
    }
    Ok(())
}

fn rollback_paths(swapped: &[(PathBuf, PathBuf)]) -> Result<(), WorkerError> {
    for (current, previous) in swapped.iter().rev() {
        remove_path(current)?;
        if previous.exists() {
            fs::rename(previous, current)?;
        }
    }
    Ok(())
}

fn remove_path(path: &Path) -> Result<(), WorkerError> {
    match fs::metadata(path) {
        Ok(meta) if meta.is_dir() => {
            fs::remove_dir_all(path)?;
            Ok(())
        }
        Ok(_) => {
            fs::remove_file(path)?;
            Ok(())
        }
        Err(err) if err.kind() == io::ErrorKind::NotFound => Ok(()),
        Err(err) => Err(WorkerError::Io(err)),
    }
}

fn move_if_exists(from: &Path, to: &Path) -> Result<(), WorkerError> {
    if !from.exists() {
        return Ok(());
    }
    fs::rename(from, to)?;
    Ok(())
}

fn trim_backups(base: &Path, keep: usize) -> Result<(), WorkerError> {
    let mut dirs: Vec<_> = fs::read_dir(base)?
        .filter_map(Result::ok)
        .filter(|entry| entry.file_type().map(|t| t.is_dir()).unwrap_or(false))
        .collect();

    dirs.sort_by_key(|entry| {
        entry
            .metadata()
            .and_then(|m| m.modified())
            .ok()
            .unwrap_or(std::time::SystemTime::UNIX_EPOCH)
    });
    dirs.reverse();

    for entry in dirs.into_iter().skip(keep) {
        let _ = fs::remove_dir_all(entry.path());
    }
    Ok(())
}

fn wait_for_path(path: &Path, timeout: Duration) -> bool {
    let deadline = Instant::now() + timeout;
    while Instant::now() <= deadline {
        if path.exists() {
            return true;
        }
        thread::sleep(Duration::from_millis(200));
    }
    false
}

fn chown_runtime_artifacts(install_dir: &Path) -> Result<(), WorkerError> {
    let user = lookup_mailclient_user()?;
    let uid = Uid::from_raw(user.uid.as_raw());
    let gid = Gid::from_raw(user.gid.as_raw());

    let paths = [
        install_dir.join("mailclient"),
        install_dir.join("mailclient-pam-reset-helper"),
        install_dir.join("mailclient-update-worker"),
        install_dir.join("web"),
        install_dir.join("migrations"),
    ];

    for path in paths {
        chown_recursive(&path, uid, gid)?;
    }
    let mailsec = install_dir.join("mailclient-mailsec-service");
    if mailsec.exists() {
        chown_recursive(&mailsec, uid, gid)?;
    }

    Ok(())
}

fn lookup_mailclient_user() -> Result<User, WorkerError> {
    match User::from_name("mailclient") {
        Ok(Some(user)) => Ok(user),
        Ok(None) => Err(WorkerError::Message(
            "mailclient user not found".to_string(),
        )),
        Err(err) => Err(WorkerError::Message(format!(
            "failed to lookup mailclient user: {err}"
        ))),
    }
}

fn chown_recursive(path: &Path, uid: Uid, gid: Gid) -> Result<(), WorkerError> {
    let meta = fs::metadata(path)?;
    if !meta.is_dir() {
        chown(path, Some(uid), Some(gid))?;
        return Ok(());
    }

    for entry in WalkDir::new(path).into_iter() {
        let entry = entry.map_err(|e| WorkerError::Message(e.to_string()))?;
        chown(entry.path(), Some(uid), Some(gid))?;
    }

    Ok(())
}

fn run_cmd(cmd: &str, args: &[&str], timeout: Duration) -> Result<(), WorkerError> {
    let mut child = Command::new(cmd)
        .args(args)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()?;

    match child.wait_timeout(timeout)? {
        Some(status) if status.success() => Ok(()),
        Some(status) => {
            let out = child.wait_with_output()?;
            Err(WorkerError::Message(format!(
                "{cmd} {} failed (status={}): {}",
                args.join(" "),
                status.code().unwrap_or(-1),
                String::from_utf8_lossy(&out.stderr).trim()
            )))
        }
        None => {
            let _ = child.kill();
            let _ = child.wait();
            Err(WorkerError::Message(format!(
                "{cmd} {} timed out",
                args.join(" ")
            )))
        }
    }
}

fn check_service_health(listen_addr: &str) -> Result<(), WorkerError> {
    let base = local_base_url(listen_addr);
    let client = Client::builder().timeout(Duration::from_secs(5)).build()?;
    let endpoints = ["/health/live", "/api/v1/setup/status"];
    let deadline = Instant::now() + Duration::from_secs(40);

    loop {
        let mut all_ok = true;

        for endpoint in &endpoints {
            let url = format!("{base}{endpoint}");
            let resp = client.get(url).send();
            match resp {
                Ok(res) if res.status().as_u16() == 200 => {}
                _ => {
                    all_ok = false;
                    break;
                }
            }
        }

        if all_ok {
            return Ok(());
        }
        if Instant::now() > deadline {
            return Err(WorkerError::Message(
                "health checks failed after restart".to_string(),
            ));
        }
        thread::sleep(Duration::from_millis(1200));
    }
}

fn local_base_url(listen_addr: &str) -> String {
    let addr = listen_addr.trim();
    if addr.is_empty() {
        return "http://127.0.0.1:8080".to_string();
    }
    if addr.starts_with(':') {
        return format!("http://127.0.0.1{addr}");
    }

    if let Ok(socket) = addr.parse::<std::net::SocketAddr>() {
        let host = match socket.ip() {
            IpAddr::V4(v4) if v4.octets() == [0, 0, 0, 0] => {
                IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1))
            }
            IpAddr::V6(v6) if v6.is_unspecified() => IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)),
            other => other,
        };
        return format!("http://{}:{}", host, socket.port());
    }

    if let Some((host, port)) = addr.rsplit_once(':') {
        let host = host.trim().trim_matches('[').trim_matches(']');
        if !port.trim().is_empty() {
            let norm_host = if host.is_empty() || host == "0.0.0.0" || host == "::" {
                "127.0.0.1"
            } else {
                host
            };
            return format!("http://{norm_host}:{}", port.trim());
        }
    }

    "http://127.0.0.1:8080".to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn archive_candidates_include_aliases() {
        let out = archive_asset_candidates("aarch64");
        assert!(out.iter().any(|v| v == "mailclient-linux-arm64.tar.gz"));
        assert!(out.iter().any(|v| v == "mailclient-linux-aarch64.tar.gz"));
    }

    #[test]
    fn local_base_url_maps_unspecified_hosts() {
        assert_eq!(local_base_url(":8080"), "http://127.0.0.1:8080");
        assert_eq!(local_base_url("0.0.0.0:9090"), "http://127.0.0.1:9090");
    }
}
