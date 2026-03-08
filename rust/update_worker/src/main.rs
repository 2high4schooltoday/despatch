use base64::{engine::general_purpose, Engine as _};
use chrono::{Local, SecondsFormat, TimeZone, Utc};
use contracts::updater::{
    sanitize_path_token, ApplyRequest, ApplyStatus, APPLY_MODE_APPLY, APPLY_MODE_PREPARE,
    APPLY_STATE_COMPLETED, APPLY_STATE_FAILED, APPLY_STATE_IN_PROGRESS, APPLY_STATE_ROLLED_BACK,
};
use ed25519_dalek::{Signature, Verifier, VerifyingKey};
use nix::unistd::{chown, geteuid, Gid, Uid, User};
use percent_encoding::{utf8_percent_encode, NON_ALPHANUMERIC};
use regex::Regex;
use reqwest::blocking::Client;
use reqwest::header::{ACCEPT, AUTHORIZATION, IF_NONE_MATCH, USER_AGENT};
use serde::de::DeserializeOwned;
use serde::Deserialize;
use serde_json::Value;
use sha2::{Digest, Sha256};
use std::collections::HashSet;
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
    update_require_signature: bool,
    update_signature_asset: String,
    update_signing_public_keys: Vec<String>,
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

#[derive(Debug, Deserialize, serde::Serialize, Clone, Default)]
struct AutoUpdateStateRecord {
    #[serde(default)]
    state: String,
    #[serde(default)]
    target_version: String,
    #[serde(default)]
    downloaded_at: String,
    #[serde(default)]
    scheduled_for: String,
    #[serde(default)]
    error: String,
    #[serde(default)]
    deferred_version: String,
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
        // Prevent a stale request file from repeatedly retriggering the path unit
        // when updates are intentionally disabled.
        let _ = remove_pending_request_paths(&cfg);
        return Ok(());
    }

    ensure_updater_dirs(&cfg)?;

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

    let req_path = match first_pending_request_path(&cfg) {
        Ok(path) => path,
        Err(WorkerError::Io(err)) if err.kind() == io::ErrorKind::NotFound => return Ok(()),
        Err(err) => return Err(err),
    };

    let mut req: ApplyRequest = match read_json_file(&req_path) {
        Ok(value) => value,
        Err(err) if err.kind() == io::ErrorKind::NotFound => return Ok(()),
        Err(err) if err.kind() == io::ErrorKind::InvalidData => {
            discard_invalid_request_payload(&cfg, &req_path, &err.to_string());
            return Ok(());
        }
        Err(err) => return Err(WorkerError::Io(err)),
    };

    if req.request_id.trim().is_empty() {
        req.request_id = format!("update-{}", Utc::now().timestamp());
    }
    if req.requested_at.trim().is_empty() {
        req.requested_at = now_rfc3339();
    }
    req.mode = normalize_apply_mode(&req.mode);

    if req.mode == APPLY_MODE_PREPARE {
        if let Ok(mut rec) = read_auto_update_state(&cfg) {
            if rec.target_version.trim().is_empty()
                || rec.target_version.trim() == req.target_version.trim()
            {
                rec.state = "preparing".to_string();
                rec.target_version = req.target_version.trim().to_string();
                rec.error.clear();
                let _ = write_auto_update_state(&cfg, &rec);
            }
        }
        match prepare_release_payload(&cfg, req.target_version.trim()) {
            Ok((release, _prepared_dir)) => {
                let rec = AutoUpdateStateRecord {
                    state: "scheduled".to_string(),
                    target_version: release.tag_name.trim().to_string(),
                    downloaded_at: now_rfc3339(),
                    scheduled_for: next_nightly_window(),
                    error: String::new(),
                    deferred_version: String::new(),
                };
                write_auto_update_state(&cfg, &rec)?;
                let _ = fs::remove_file(&req_path);
                return Ok(());
            }
            Err(err) => {
                if let Ok(mut rec) = read_auto_update_state(&cfg) {
                    rec.state = "failed".to_string();
                    rec.target_version = req.target_version.trim().to_string();
                    rec.error = err.to_string();
                    let _ = write_auto_update_state(&cfg, &rec);
                }
                let _ = fs::remove_file(&req_path);
                return Err(err);
            }
        }
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
    if let Ok(mut rec) = read_auto_update_state(&cfg) {
        if !target_version.is_empty() && rec.target_version.trim() == target_version {
            rec.state = "applying".to_string();
            rec.error.clear();
            let _ = write_auto_update_state(&cfg, &rec);
        }
    }
    match fs::remove_file(&req_path) {
        Ok(()) => {}
        Err(err) if err.kind() == io::ErrorKind::NotFound => {}
        Err(err) => return Err(WorkerError::Io(err)),
    }

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
            if let Ok(mut rec) = read_auto_update_state(&cfg) {
                if !target_version.is_empty() && rec.target_version.trim() == target_version {
                    rec.state = "failed".to_string();
                    rec.error = err.to_string();
                    let _ = write_auto_update_state(&cfg, &rec);
                }
            }
            let _ = fs::remove_file(&req_path);
            return Err(err);
        }
    }

    final_status.finished_at = now_rfc3339();
    write_status(&cfg, &final_status)?;
    if let Ok(rec) = read_auto_update_state(&cfg) {
        if !final_status.to_version.trim().is_empty()
            && rec.target_version.trim() == final_status.to_version.trim()
        {
            let _ = write_auto_update_state(
                &cfg,
                &AutoUpdateStateRecord {
                    state: "idle".to_string(),
                    ..AutoUpdateStateRecord::default()
                },
            );
            let _ = fs::remove_dir_all(prepared_payload_dir(&cfg, final_status.to_version.trim()));
        }
    }
    let _ = fs::remove_file(&req_path);
    Ok(())
}

impl Config {
    fn from_env() -> Result<Self, WorkerError> {
        let mut cfg = Self {
            update_enabled: env_bool("UPDATE_ENABLED", true),
            update_repo_owner: env_var("UPDATE_REPO_OWNER", "2high4schooltoday"),
            update_repo_name: env_var("UPDATE_REPO_NAME", "despatch"),
            update_http_timeout_sec: env_u64("UPDATE_HTTP_TIMEOUT_SEC", 10)?,
            update_github_token: env::var("UPDATE_GITHUB_TOKEN").unwrap_or_default(),
            update_backup_keep: env_u64("UPDATE_BACKUP_KEEP", 3)? as usize,
            update_base_dir: PathBuf::from(env_var("UPDATE_BASE_DIR", "/var/lib/despatch/update")),
            update_install_dir: PathBuf::from(env_var("UPDATE_INSTALL_DIR", "/opt/despatch")),
            update_service_name: env_var("UPDATE_SERVICE_NAME", "despatch"),
            update_systemd_unit_dir: PathBuf::from(env_var(
                "UPDATE_SYSTEMD_UNIT_DIR",
                "/etc/systemd/system",
            )),
            update_require_signature: env_bool("UPDATE_REQUIRE_SIGNATURE", true),
            update_signature_asset: env_var("UPDATE_SIGNATURE_ASSET", "checksums.txt.sig"),
            update_signing_public_keys: env_csv("UPDATE_SIGNING_PUBLIC_KEYS"),
            listen_addr: env_var("LISTEN_ADDR", ":8080"),
            mailsec_enabled: env_bool("MAILSEC_ENABLED", false),
            mailsec_socket: PathBuf::from(env_var("MAILSEC_SOCKET", "/run/despatch/mailsec.sock")),
        };
        cfg.update_signing_public_keys =
            parse_signing_public_keys(&cfg.update_signing_public_keys)?;
        if cfg.update_require_signature && cfg.update_signature_asset.trim().is_empty() {
            return Err(WorkerError::Message(
                "UPDATE_SIGNATURE_ASSET is required when UPDATE_REQUIRE_SIGNATURE=true".to_string(),
            ));
        }
        if cfg.update_enabled
            && cfg.update_require_signature
            && cfg.update_signing_public_keys.is_empty()
        {
            return Err(WorkerError::Message(
                "UPDATE_SIGNING_PUBLIC_KEYS is required when update signatures are enforced"
                    .to_string(),
            ));
        }
        Ok(cfg)
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

fn env_csv(key: &str) -> Vec<String> {
    match env::var(key) {
        Ok(raw) => raw
            .split(',')
            .map(|v| v.trim().to_string())
            .filter(|v| !v.is_empty())
            .collect(),
        Err(_) => Vec::new(),
    }
}

fn parse_signing_public_keys(keys: &[String]) -> Result<Vec<String>, WorkerError> {
    if keys.is_empty() {
        return Ok(Vec::new());
    }
    let mut out = Vec::with_capacity(keys.len());
    let mut seen = HashSet::with_capacity(keys.len());
    for raw in keys {
        let key = raw.trim();
        if key.is_empty() {
            continue;
        }
        let decoded = general_purpose::STANDARD
            .decode(key.as_bytes())
            .or_else(|_| general_purpose::STANDARD_NO_PAD.decode(key.as_bytes()))
            .map_err(|_| WorkerError::Message(format!("invalid update signing key: {key}")))?;
        if decoded.len() != 32 {
            return Err(WorkerError::Message(format!(
                "invalid update signing key length: {key}"
            )));
        }
        let normalized = general_purpose::STANDARD.encode(decoded);
        if seen.insert(normalized.clone()) {
            out.push(normalized);
        }
    }
    Ok(out)
}

fn now_rfc3339() -> String {
    Utc::now().to_rfc3339_opts(SecondsFormat::Secs, true)
}

fn current_version() -> String {
    option_env!("DESPATCH_BUILD_VERSION")
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

fn request_queue_path(req: &ApplyRequest, cfg: &Config) -> PathBuf {
    let request_id = sanitize_path_token(req.request_id.trim());
    let ts = chrono::DateTime::parse_from_rfc3339(req.requested_at.trim())
        .map(|v| v.timestamp_nanos_opt().unwrap_or_default())
        .unwrap_or_else(|_| Utc::now().timestamp_nanos_opt().unwrap_or_default());
    request_dir(cfg).join(format!("update-request-{ts:020}-{request_id}.json"))
}

fn pending_request_paths(cfg: &Config) -> Result<Vec<PathBuf>, WorkerError> {
    let mut out = Vec::new();
    let legacy = request_path(cfg);
    if legacy.exists() {
        out.push(legacy);
    }
    let mut queued = Vec::new();
    if request_dir(cfg).exists() {
        for entry in fs::read_dir(request_dir(cfg))? {
            let entry = entry?;
            if !entry.file_type()?.is_file() {
                continue;
            }
            let name = entry.file_name();
            let name = name.to_string_lossy();
            if name.starts_with("update-request-") && name.ends_with(".json") {
                queued.push(entry.path());
            }
        }
    }
    queued.sort();
    queued.dedup();
    out.extend(queued);
    Ok(out)
}

fn first_pending_request_path(cfg: &Config) -> Result<PathBuf, WorkerError> {
    pending_request_paths(cfg)?
        .into_iter()
        .next()
        .ok_or_else(|| WorkerError::Io(io::Error::from(io::ErrorKind::NotFound)))
}

fn remove_pending_request_paths(cfg: &Config) -> Result<(), WorkerError> {
    for path in pending_request_paths(cfg)? {
        if let Err(err) = fs::remove_file(&path) {
            if err.kind() != io::ErrorKind::NotFound {
                return Err(WorkerError::Io(err));
            }
        }
    }
    Ok(())
}

fn status_path(cfg: &Config) -> PathBuf {
    status_dir(cfg).join("update-status.json")
}

fn lock_path(cfg: &Config) -> PathBuf {
    lock_dir(cfg).join("update.lock")
}

fn auto_status_path(cfg: &Config) -> PathBuf {
    status_dir(cfg).join("update-auto-status.json")
}

fn prepared_payload_dir(cfg: &Config, version: &str) -> PathBuf {
    work_dir(cfg).join(format!("prepared-{}", sanitize_path_token(version)))
}

fn normalize_apply_mode(raw: &str) -> String {
    match raw.trim().to_ascii_lowercase().as_str() {
        "" | APPLY_MODE_APPLY => APPLY_MODE_APPLY.to_string(),
        APPLY_MODE_PREPARE => APPLY_MODE_PREPARE.to_string(),
        _ => APPLY_MODE_APPLY.to_string(),
    }
}

fn read_auto_update_state(cfg: &Config) -> Result<AutoUpdateStateRecord, WorkerError> {
    if !auto_status_path(cfg).exists() {
        return Ok(AutoUpdateStateRecord {
            state: "idle".to_string(),
            ..AutoUpdateStateRecord::default()
        });
    }
    let mut rec: AutoUpdateStateRecord = read_json_file(&auto_status_path(cfg))?;
    if rec.state.trim().is_empty() {
        rec.state = "idle".to_string();
    }
    Ok(rec)
}

fn write_auto_update_state(cfg: &Config, rec: &AutoUpdateStateRecord) -> Result<(), WorkerError> {
    let payload = serde_json::to_value(rec)?;
    write_json_atomic(&auto_status_path(cfg), &payload, 0o640, 0o770)?;
    ensure_despatch_readable(&auto_status_path(cfg))?;
    Ok(())
}

fn next_nightly_window() -> String {
    let now = Local::now();
    let today = now
        .date_naive()
        .and_hms_opt(2, 0, 0)
        .unwrap_or_else(|| now.naive_local());
    let scheduled = if now.naive_local() < today {
        today
    } else {
        (now + chrono::Duration::days(1))
            .date_naive()
            .and_hms_opt(2, 0, 0)
            .unwrap_or_else(|| now.naive_local())
    };
    match Local.from_local_datetime(&scheduled).single() {
        Some(value) => value.to_rfc3339_opts(SecondsFormat::Secs, true),
        None => now.to_rfc3339_opts(SecondsFormat::Secs, true),
    }
}

#[derive(Clone, Copy)]
enum UpdaterDirGroup {
    Root,
    Despatch,
}

#[derive(Clone)]
struct UpdaterDirSpec {
    path: PathBuf,
    mode: u32,
    group: UpdaterDirGroup,
}

fn updater_dir_specs(cfg: &Config) -> Vec<UpdaterDirSpec> {
    vec![
        UpdaterDirSpec {
            path: cfg.update_base_dir.clone(),
            mode: 0o750,
            group: UpdaterDirGroup::Despatch,
        },
        UpdaterDirSpec {
            path: request_dir(cfg),
            mode: 0o770,
            group: UpdaterDirGroup::Despatch,
        },
        UpdaterDirSpec {
            path: status_dir(cfg),
            mode: 0o770,
            group: UpdaterDirGroup::Despatch,
        },
        UpdaterDirSpec {
            path: lock_dir(cfg),
            mode: 0o750,
            group: UpdaterDirGroup::Root,
        },
        UpdaterDirSpec {
            path: work_dir(cfg),
            mode: 0o750,
            group: UpdaterDirGroup::Root,
        },
        UpdaterDirSpec {
            path: backups_dir(cfg),
            mode: 0o750,
            group: UpdaterDirGroup::Root,
        },
    ]
}

fn ensure_updater_dirs(cfg: &Config) -> Result<(), WorkerError> {
    let specs = updater_dir_specs(cfg);
    let running_as_root = geteuid().is_root();
    let despatch_gid = if running_as_root {
        Some(lookup_despatch_user()?.gid.as_raw())
    } else {
        None
    };
    for spec in specs {
        fs::create_dir_all(&spec.path)?;
        fs::set_permissions(&spec.path, fs::Permissions::from_mode(spec.mode))?;
        if let Some(despatch_gid) = despatch_gid {
            let gid = match spec.group {
                UpdaterDirGroup::Root => 0,
                UpdaterDirGroup::Despatch => despatch_gid,
            };
            chown(&spec.path, Some(Uid::from_raw(0)), Some(Gid::from_raw(gid)))?;
        }
    }
    Ok(())
}

fn read_json_file<T: DeserializeOwned>(path: &Path) -> Result<T, io::Error> {
    reject_symlink(path)?;
    let raw = fs::read(path)?;
    serde_json::from_slice(&raw).map_err(|err| io::Error::new(io::ErrorKind::InvalidData, err))
}

fn write_json_atomic(
    path: &Path,
    payload: &Value,
    mode: u32,
    parent_mode: u32,
) -> Result<(), WorkerError> {
    reject_symlink(path)?;
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)?;
        fs::set_permissions(parent, fs::Permissions::from_mode(parent_mode))?;
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

fn reject_symlink(path: &Path) -> Result<(), io::Error> {
    match fs::symlink_metadata(path) {
        Ok(meta) => {
            if meta.file_type().is_symlink() {
                return Err(io::Error::new(
                    io::ErrorKind::PermissionDenied,
                    format!("refusing symlink path: {}", path.display()),
                ));
            }
            Ok(())
        }
        Err(err) if err.kind() == io::ErrorKind::NotFound => Ok(()),
        Err(err) => Err(err),
    }
}

fn write_status(cfg: &Config, status: &ApplyStatus) -> Result<(), WorkerError> {
    let mut status = status.clone();
    if status.state.trim().is_empty() {
        status.state = APPLY_STATE_FAILED.to_string();
    }
    let payload = serde_json::to_value(&status)?;
    write_json_atomic(&status_path(cfg), &payload, 0o640, 0o770)?;
    ensure_despatch_readable(&status_path(cfg))?;
    Ok(())
}

fn discard_invalid_request_payload(cfg: &Config, req_path: &Path, detail: &str) {
    let now = now_rfc3339();
    let status = ApplyStatus {
        state: APPLY_STATE_FAILED.to_string(),
        request_id: format!("invalid-request-{}", Utc::now().timestamp()),
        requested_at: now.clone(),
        started_at: now.clone(),
        finished_at: now,
        target_version: String::new(),
        from_version: current_version(),
        to_version: String::new(),
        rolled_back: false,
        error: format!("invalid updater request payload; discarded ({detail})"),
    };
    let _ = write_status(cfg, &status);
    let _ = fs::remove_file(req_path);
}

fn ensure_despatch_readable(path: &Path) -> Result<(), WorkerError> {
    let user = lookup_despatch_user()?;
    chown(path, None, Some(Gid::from_raw(user.gid.as_raw())))?;
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

    let run_id = if req.request_id.trim().is_empty() {
        format!("run-{}", Utc::now().timestamp())
    } else {
        req.request_id.trim().to_string()
    };
    let (release, payload_root) = prepare_release_payload(cfg, target)?;

    let stage_dir = cfg
        .update_install_dir
        .join(format!(".update-stage-{}", sanitize_path_token(&run_id)));
    let _ = fs::remove_dir_all(&stage_dir);
    fs::create_dir_all(&stage_dir)?;

    copy_file(
        &payload_root.join("despatch"),
        &stage_dir.join("despatch"),
        0o755,
    )?;
    copy_file(
        &payload_root.join("despatch-pam-reset-helper"),
        &stage_dir.join("despatch-pam-reset-helper"),
        0o755,
    )?;
    copy_file(
        &payload_root.join("despatch-update-worker"),
        &stage_dir.join("despatch-update-worker"),
        0o755,
    )?;
    copy_dir(&payload_root.join("web"), &stage_dir.join("web"))?;
    copy_dir(
        &payload_root.join("migrations"),
        &stage_dir.join("migrations"),
    )?;
    copy_dir(&payload_root.join("deploy"), &stage_dir.join("deploy"))?;
    let mailsec_payload = payload_root.join("despatch-mailsec-service");
    let mailsec_payload_present = mailsec_payload.exists();
    if mailsec_payload_present {
        copy_file(
            &mailsec_payload,
            &stage_dir.join("despatch-mailsec-service"),
            0o755,
        )?;
    }
    let current_mailsec = cfg.update_install_dir.join("despatch-mailsec-service");
    let current_mailsec_present = current_mailsec.exists();
    let mailsec_unit_known_to_systemd = systemd_unit_known("despatch-mailsec.service");

    if cfg.mailsec_enabled && !mailsec_payload_present && !current_mailsec_present {
        return Err(WorkerError::Message(
            "mailsec is enabled but despatch-mailsec-service is missing in both current install and release payload".to_string(),
        ));
    }

    let prev_bin = cfg
        .update_install_dir
        .join(format!(".prev-despatch-{}", sanitize_path_token(&run_id)));
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
    let prev_deploy = cfg
        .update_install_dir
        .join(format!(".prev-deploy-{}", sanitize_path_token(&run_id)));
    let prev_mailsec = cfg
        .update_install_dir
        .join(format!(".prev-mailsec-{}", sanitize_path_token(&run_id)));

    let current_bin = cfg.update_install_dir.join("despatch");
    let current_pam = cfg.update_install_dir.join("despatch-pam-reset-helper");
    let current_worker = cfg.update_install_dir.join("despatch-update-worker");
    let current_web = cfg.update_install_dir.join("web");
    let current_mig = cfg.update_install_dir.join("migrations");
    let current_deploy = cfg.update_install_dir.join("deploy");

    let stage_bin = stage_dir.join("despatch");
    let stage_pam = stage_dir.join("despatch-pam-reset-helper");
    let stage_worker = stage_dir.join("despatch-update-worker");
    let stage_web = stage_dir.join("web");
    let stage_mig = stage_dir.join("migrations");
    let stage_deploy = stage_dir.join("deploy");
    let stage_mailsec = stage_dir.join("despatch-mailsec-service");

    let _ = fs::remove_file(&prev_bin);
    let _ = fs::remove_file(&prev_pam);
    let _ = fs::remove_file(&prev_worker);
    let _ = fs::remove_dir_all(&prev_web);
    let _ = fs::remove_dir_all(&prev_mig);
    let _ = fs::remove_dir_all(&prev_deploy);
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
    if let Err(err) = swap_path_optional(&current_deploy, &stage_deploy, &prev_deploy) {
        rollback_paths(&swapped)?;
        return Err(err);
    }
    swapped.push((current_deploy.clone(), prev_deploy.clone()));

    let updater_service_src = current_deploy.join("despatch-updater.service");
    let updater_path_src = current_deploy.join("despatch-updater.path");
    if !updater_service_src.exists() || !updater_path_src.exists() {
        rollback_paths(&swapped)?;
        return Err(WorkerError::Message(
            "deploy payload is missing despatch-updater.path or despatch-updater.service after refresh".to_string(),
        ));
    }
    copy_file(
        &updater_service_src,
        &cfg.update_systemd_unit_dir.join("despatch-updater.service"),
        0o644,
    )?;
    copy_file(
        &updater_path_src,
        &cfg.update_systemd_unit_dir.join("despatch-updater.path"),
        0o644,
    )?;

    let mailsec_unit_source = current_deploy.join("despatch-mailsec.service");
    let mailsec_unit_dst = cfg.update_systemd_unit_dir.join("despatch-mailsec.service");
    if mailsec_unit_source.exists() {
        if let Err(err) = copy_file(&mailsec_unit_source, &mailsec_unit_dst, 0o644) {
            if is_read_only_or_permission_error(&err) && mailsec_unit_known_to_systemd {
                // Keep using existing unit when systemd unit dir is read-only.
            } else {
                rollback_paths(&swapped)?;
                return Err(WorkerError::Message(format!(
                    "mailsec unit install failed: {err}"
                )));
            }
        }
    }
    if let Err(err) = run_cmd("systemctl", &["daemon-reload"], Duration::from_secs(60)) {
        rollback_paths(&swapped)?;
        return Err(WorkerError::Message(format!(
            "systemd daemon-reload failed: {err}"
        )));
    }
    let _ = run_cmd(
        "systemctl",
        &[
            "reset-failed",
            "despatch-updater.service",
            "despatch-updater.path",
        ],
        Duration::from_secs(60),
    );
    if let Err(err) = run_cmd(
        "systemctl",
        &["enable", "--now", "despatch-updater.path"],
        Duration::from_secs(60),
    ) {
        rollback_paths(&swapped)?;
        return Err(WorkerError::Message(format!(
            "updater path activation failed: {err}"
        )));
    }
    let mailsec_unit_now_present =
        mailsec_unit_dst.exists() || systemd_unit_known("despatch-mailsec.service");
    if cfg.mailsec_enabled && !mailsec_unit_now_present {
        rollback_paths(&swapped)?;
        return Err(WorkerError::Message(
            "mailsec is enabled but systemd unit despatch-mailsec.service is still missing after update".to_string(),
        ));
    }
    if cfg.mailsec_enabled || mailsec_payload_present || mailsec_unit_now_present {
        if let Err(err) = run_cmd(
            "systemctl",
            &["enable", "--now", "despatch-mailsec"],
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

    if let Err(err) = migrate_legacy_password_reset_env(&cfg.update_install_dir.join(".env")) {
        rollback_paths(&swapped)?;
        return Err(WorkerError::Message(format!(
            "password reset env migration failed: {err}"
        )));
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
    move_if_exists(&prev_bin, &backup_dest.join("despatch"))?;
    move_if_exists(&prev_pam, &backup_dest.join("despatch-pam-reset-helper"))?;
    move_if_exists(&prev_worker, &backup_dest.join("despatch-update-worker"))?;
    move_if_exists(&prev_web, &backup_dest.join("web"))?;
    move_if_exists(&prev_mig, &backup_dest.join("migrations"))?;
    move_if_exists(&prev_deploy, &backup_dest.join("deploy"))?;
    move_if_exists(&prev_mailsec, &backup_dest.join("despatch-mailsec-service"))?;

    trim_backups(&backups_dir(cfg), cfg.update_backup_keep)?;

    Ok(ApplyResult {
        to_version: release.tag_name.trim().to_string(),
        rolled_back: false,
    })
}

fn prepare_release_payload(
    cfg: &Config,
    target_version: &str,
) -> Result<(GithubRelease, PathBuf), WorkerError> {
    let release = resolve_release(cfg, target_version)?;
    let prepared_dir = prepared_payload_dir(cfg, release.tag_name.trim());
    if prepared_payload_ready(&prepared_dir) {
        trim_prepared_payloads(&work_dir(cfg), &prepared_dir)?;
        return Ok((release, prepared_dir));
    }

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
    let signature_name = cfg.update_signature_asset.trim().to_string();
    let signature_url = if cfg.update_require_signature {
        Some(
            release
                .assets
                .iter()
                .find(|a| a.name.trim().eq_ignore_ascii_case(signature_name.trim()))
                .map(|a| a.browser_download_url.trim().to_string())
                .ok_or_else(|| {
                    WorkerError::Message(format!(
                        "release asset {} not found",
                        signature_name.trim()
                    ))
                })?,
        )
    } else {
        None
    };

    let run_work = work_dir(cfg).join(format!(
        ".prepare-{}-{}",
        sanitize_path_token(release.tag_name.trim()),
        Utc::now().timestamp_nanos_opt().unwrap_or_default()
    ));
    let _ = fs::remove_dir_all(&run_work);
    fs::create_dir_all(&run_work)?;

    let archive_path = run_work.join(&archive_name);
    let checksum_path = run_work.join("checksums.txt");
    download_asset(cfg, &archive_url, &archive_path)?;
    download_asset(cfg, &checksum_url, &checksum_path)?;
    if let Some(signature_url) = signature_url {
        let signature_path = run_work.join(signature_name.trim());
        download_asset(cfg, &signature_url, &signature_path)?;
        verify_checksum_signature(
            &checksum_path,
            &signature_path,
            &cfg.update_signing_public_keys,
        )?;
    }
    verify_checksum_file(&checksum_path, &archive_path, &archive_name)?;

    let extracted = run_work.join("extract");
    extract_tar_gz(&archive_path, &extracted)?;
    let payload_root = find_payload_root(&extracted)?;

    let tmp_prepared = prepared_dir.with_extension("tmp");
    let _ = fs::remove_dir_all(&tmp_prepared);
    fs::create_dir_all(&tmp_prepared)?;
    copy_file(&payload_root.join("despatch"), &tmp_prepared.join("despatch"), 0o755)?;
    copy_file(
        &payload_root.join("despatch-pam-reset-helper"),
        &tmp_prepared.join("despatch-pam-reset-helper"),
        0o755,
    )?;
    copy_file(
        &payload_root.join("despatch-update-worker"),
        &tmp_prepared.join("despatch-update-worker"),
        0o755,
    )?;
    copy_dir(&payload_root.join("web"), &tmp_prepared.join("web"))?;
    copy_dir(&payload_root.join("migrations"), &tmp_prepared.join("migrations"))?;
    copy_dir(&payload_root.join("deploy"), &tmp_prepared.join("deploy"))?;
    let mailsec_payload = payload_root.join("despatch-mailsec-service");
    if mailsec_payload.exists() {
        copy_file(
            &mailsec_payload,
            &tmp_prepared.join("despatch-mailsec-service"),
            0o755,
        )?;
    }
    let _ = fs::remove_dir_all(&prepared_dir);
    fs::rename(&tmp_prepared, &prepared_dir)?;
    let _ = fs::remove_dir_all(&run_work);
    trim_prepared_payloads(&work_dir(cfg), &prepared_dir)?;
    Ok((release, prepared_dir))
}

fn prepared_payload_ready(path: &Path) -> bool {
    let required = [
        path.join("despatch"),
        path.join("despatch-pam-reset-helper"),
        path.join("despatch-update-worker"),
        path.join("web"),
        path.join("migrations"),
        path.join("deploy"),
    ];
    required.iter().all(|candidate| candidate.exists())
}

fn trim_prepared_payloads(base_dir: &Path, keep_path: &Path) -> Result<(), WorkerError> {
    if !base_dir.exists() {
        return Ok(());
    }
    for entry in fs::read_dir(base_dir)? {
        let entry = entry?;
        if !entry.file_type()?.is_dir() {
            continue;
        }
        let name = entry.file_name();
        let name = name.to_string_lossy();
        if !name.starts_with("prepared-") {
            continue;
        }
        let path = entry.path();
        if path == keep_path {
            continue;
        }
        let _ = fs::remove_dir_all(path);
    }
    Ok(())
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
        .header(USER_AGENT, "despatch-updater/1");

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
        out.push(format!("despatch-linux-{alias}.tar.gz"));
        out.push(format!("despatch-linux-{alias}.tgz"));
        out.push(format!("despatch_{alias}_linux.tar.gz"));
        out.push(format!("despatch_{alias}_linux.tgz"));
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

    let mut req = client.get(parsed).header(USER_AGENT, "despatch-updater/1");

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

fn verify_checksum_signature(
    checksum_path: &Path,
    signature_path: &Path,
    public_keys: &[String],
) -> Result<(), WorkerError> {
    if public_keys.is_empty() {
        return Err(WorkerError::Message(
            "no update signing keys configured".to_string(),
        ));
    }
    let checksum = fs::read(checksum_path)?;
    let signature_raw = fs::read(signature_path)?;
    let signature = decode_detached_signature(&signature_raw)?;

    for key in public_keys {
        let verifying_key = decode_verifying_key(key)?;
        if verifying_key.verify(&checksum, &signature).is_ok() {
            return Ok(());
        }
    }
    Err(WorkerError::Message(
        "signature verification failed for checksums file".to_string(),
    ))
}

fn decode_verifying_key(raw: &str) -> Result<VerifyingKey, WorkerError> {
    let key = raw.trim();
    if key.is_empty() {
        return Err(WorkerError::Message("empty update signing key".to_string()));
    }
    let decoded = general_purpose::STANDARD
        .decode(key)
        .or_else(|_| general_purpose::STANDARD_NO_PAD.decode(key))
        .map_err(|_| WorkerError::Message("invalid update signing key encoding".to_string()))?;
    let arr: [u8; 32] = decoded
        .try_into()
        .map_err(|_| WorkerError::Message("invalid update signing key size".to_string()))?;
    VerifyingKey::from_bytes(&arr)
        .map_err(|_| WorkerError::Message("invalid update signing key bytes".to_string()))
}

fn decode_detached_signature(raw: &[u8]) -> Result<Signature, WorkerError> {
    let text = String::from_utf8_lossy(raw).trim().to_string();
    if text.is_empty() {
        return Err(WorkerError::Message("empty update signature".to_string()));
    }
    let decoded = general_purpose::STANDARD
        .decode(text.as_bytes())
        .or_else(|_| general_purpose::STANDARD_NO_PAD.decode(text.as_bytes()))
        .map_err(|_| WorkerError::Message("invalid update signature encoding".to_string()));
    if let Ok(decoded) = decoded {
        let arr: [u8; 64] = decoded
            .try_into()
            .map_err(|_| WorkerError::Message("invalid update signature size".to_string()))?;
        return Ok(Signature::from_bytes(&arr));
    }
    if raw.len() == 64 {
        let arr: [u8; 64] = raw
            .try_into()
            .map_err(|_| WorkerError::Message("invalid update signature size".to_string()))?;
        return Ok(Signature::from_bytes(&arr));
    }
    Err(WorkerError::Message(
        "invalid update signature encoding".to_string(),
    ))
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
        "despatch",
        "despatch-pam-reset-helper",
        "despatch-update-worker",
        "web",
        "migrations",
        "deploy",
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
        "release payload missing required files (despatch, despatch-pam-reset-helper, despatch-update-worker, web, migrations, deploy)".to_string(),
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

fn migrate_legacy_password_reset_env(path: &Path) -> Result<bool, WorkerError> {
    let metadata = match fs::metadata(path) {
        Ok(meta) => meta,
        Err(err) if err.kind() == io::ErrorKind::NotFound => return Ok(false),
        Err(err) => return Err(WorkerError::Io(err)),
    };
    let raw = fs::read_to_string(path)?;
    let mut lines: Vec<String> = raw
        .replace("\r\n", "\n")
        .lines()
        .map(|line| line.to_string())
        .collect();
    let values = env_file_values(&lines);
    if !should_migrate_legacy_password_reset_env(&values) {
        return Ok(false);
    }

    let current_from = env_value_trimmed(&values, "PASSWORD_RESET_FROM");
    if should_rewrite_password_reset_from(&current_from, &values) {
        let target = format!(
            "no-reply@{}",
            derive_password_reset_sender_domain(&preferred_public_reset_host(&values))
        );
        env_file_set(&mut lines, "PASSWORD_RESET_FROM", &target);
    }
    if should_rewrite_password_reset_base_url(&env_value_trimmed(
        &values,
        "PASSWORD_RESET_BASE_URL",
    )) {
        let base_url = derive_password_reset_base_url(&values);
        env_file_set(&mut lines, "PASSWORD_RESET_BASE_URL", &base_url);
    }
    env_file_set(&mut lines, "SMTP_PORT", "25");
    env_file_set(&mut lines, "SMTP_STARTTLS", "false");
    env_file_set(&mut lines, "SMTP_TLS", "false");
    if env_value_trimmed(&values, "DOVECOT_AUTH_MODE").eq_ignore_ascii_case("pam")
        && !env_value_trimmed(&values, "PASSWORD_RESET_SENDER").eq_ignore_ascii_case("log")
    {
        env_file_set(&mut lines, "PASSWORD_RESET_EXTERNAL_SENDER_READY", "true");
    }

    let mut output = lines.join("\n");
    output.push('\n');
    let tmp = path.with_extension("tmp-reset-migration");
    {
        let mut file = OpenOptions::new()
            .create(true)
            .truncate(true)
            .write(true)
            .mode(metadata.permissions().mode() & 0o777)
            .open(&tmp)?;
        file.write_all(output.as_bytes())?;
        file.sync_all()?;
    }
    fs::rename(&tmp, path)?;
    fs::set_permissions(
        path,
        fs::Permissions::from_mode(metadata.permissions().mode() & 0o777),
    )?;
    Ok(true)
}

fn env_file_values(lines: &[String]) -> Vec<(String, String)> {
    let mut out = Vec::new();
    for line in lines {
        if let Some((key, value)) = line.split_once('=') {
            let key = key.trim();
            if key.is_empty() || key.starts_with('#') {
                continue;
            }
            out.push((key.to_string(), value.trim().to_string()));
        }
    }
    out
}

fn env_value_trimmed(values: &[(String, String)], key: &str) -> String {
    values
        .iter()
        .find(|(candidate, _)| candidate == key)
        .map(|(_, value)| value.trim().to_string())
        .unwrap_or_default()
}

fn env_file_set(lines: &mut Vec<String>, key: &str, value: &str) {
    let prefix = format!("{key}=");
    if let Some(idx) = lines.iter().position(|line| line.starts_with(&prefix)) {
        lines[idx] = format!("{prefix}{value}");
        return;
    }
    lines.push(format!("{prefix}{value}"));
}

fn should_migrate_legacy_password_reset_env(values: &[(String, String)]) -> bool {
    if env_value_trimmed(values, "PASSWORD_RESET_SENDER").eq_ignore_ascii_case("log") {
        return false;
    }
    if !is_loopback_reset_smtp_host(&env_value_trimmed(values, "SMTP_HOST")) {
        return false;
    }
    if env_value_trimmed(values, "SMTP_PORT") != "587" {
        return false;
    }
    if !env_flag_true(values, "SMTP_STARTTLS") || env_flag_true(values, "SMTP_TLS") {
        return false;
    }
    if !env_value_trimmed(values, "PASSWORD_RESET_SMTP_USER").is_empty()
        || !env_value_trimmed(values, "SMTP_USER").is_empty()
    {
        return false;
    }
    true
}

fn env_flag_true(values: &[(String, String)], key: &str) -> bool {
    matches!(
        env_value_trimmed(values, key).to_ascii_lowercase().as_str(),
        "1" | "true" | "yes" | "y" | "on"
    )
}

fn is_loopback_reset_smtp_host(host: &str) -> bool {
    matches!(
        host.trim().to_ascii_lowercase().as_str(),
        "127.0.0.1" | "localhost" | "::1" | "[::1]"
    )
}

fn preferred_public_reset_host(values: &[(String, String)]) -> String {
    if env_value_trimmed(values, "DEPLOY_MODE").eq_ignore_ascii_case("proxy") {
        let host = env_value_trimmed(values, "PROXY_SERVER_NAME");
        if !host.is_empty() {
            return host;
        }
    }
    env_value_trimmed(values, "BASE_DOMAIN")
}

fn derive_password_reset_sender_domain(host: &str) -> String {
    let domain = host.trim().to_ascii_lowercase();
    if domain.starts_with("mail.") && domain.len() > "mail.".len() {
        return domain["mail.".len()..].to_string();
    }
    if domain.is_empty() {
        return "example.com".to_string();
    }
    domain
}

fn should_rewrite_password_reset_from(current: &str, values: &[(String, String)]) -> bool {
    let current = current.trim().to_ascii_lowercase();
    if current.is_empty() || current.ends_with("@example.com") {
        return true;
    }
    let base = env_value_trimmed(values, "BASE_DOMAIN").to_ascii_lowercase();
    let public_host = preferred_public_reset_host(values).to_ascii_lowercase();
    current == format!("no-reply@{base}") || current == format!("no-reply@{public_host}")
}

fn should_rewrite_password_reset_base_url(current: &str) -> bool {
    let current = current.trim().to_ascii_lowercase();
    current.is_empty()
        || current.contains("127.0.0.1")
        || current.contains("localhost")
        || current.contains("example.com")
}

fn derive_password_reset_base_url(values: &[(String, String)]) -> String {
    if env_value_trimmed(values, "DEPLOY_MODE").eq_ignore_ascii_case("proxy") {
        let mut host = env_value_trimmed(values, "PROXY_SERVER_NAME");
        if host.is_empty() {
            host = env_value_trimmed(values, "BASE_DOMAIN");
        }
        if host.is_empty() {
            host = "127.0.0.1".to_string();
        }
        if env_flag_true(values, "PROXY_TLS") {
            return format!("https://{host}");
        }
        return format!("http://{host}");
    }

    let listen = env_value_trimmed(values, "LISTEN_ADDR");
    let (mut host, mut port) = if let Some(port) = listen.strip_prefix(':') {
        (env_value_trimmed(values, "BASE_DOMAIN"), port.to_string())
    } else if let Some((host, port)) = listen.rsplit_once(':') {
        (
            host.trim().trim_matches('[').trim_matches(']').to_string(),
            port.trim().to_string(),
        )
    } else {
        (env_value_trimmed(values, "BASE_DOMAIN"), "8080".to_string())
    };

    if host.is_empty() || host == "0.0.0.0" || host == "::" || is_loopback_reset_smtp_host(&host) {
        host = env_value_trimmed(values, "BASE_DOMAIN");
    }
    if host.is_empty() {
        host = "127.0.0.1".to_string();
    }
    if port.is_empty() {
        port = "8080".to_string();
    }
    format!("http://{host}:{port}")
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

fn lookup_despatch_user() -> Result<User, WorkerError> {
    match User::from_name("despatch") {
        Ok(Some(user)) => Ok(user),
        Ok(None) => Err(WorkerError::Message("despatch user not found".to_string())),
        Err(err) => Err(WorkerError::Message(format!(
            "failed to lookup despatch user: {err}"
        ))),
    }
}

fn is_read_only_or_permission_error(err: &WorkerError) -> bool {
    match err {
        WorkerError::Io(io_err) => {
            io_err.kind() == io::ErrorKind::PermissionDenied || io_err.raw_os_error() == Some(30)
        }
        WorkerError::Message(msg) => {
            let lower = msg.to_ascii_lowercase();
            lower.contains("read-only file system") || lower.contains("permission denied")
        }
        _ => false,
    }
}

fn systemd_unit_known(unit_name: &str) -> bool {
    let unit = unit_name.trim();
    if unit.is_empty() {
        return false;
    }
    match run_cmd_output(
        "systemctl",
        &["show", "--property=LoadState", "--value", unit],
        Duration::from_secs(5),
    ) {
        Ok(state) => is_systemd_load_state_known(&state),
        Err(_) => false,
    }
}

fn is_systemd_load_state_known(raw: &str) -> bool {
    let state = raw.trim().to_ascii_lowercase();
    !state.is_empty() && state != "not-found" && state != "error" && state != "bad-setting"
}

fn run_cmd(cmd: &str, args: &[&str], timeout: Duration) -> Result<(), WorkerError> {
    run_cmd_output(cmd, args, timeout).map(|_| ())
}

fn run_cmd_output(cmd: &str, args: &[&str], timeout: Duration) -> Result<String, WorkerError> {
    let mut child = Command::new(cmd)
        .args(args)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()?;

    match child.wait_timeout(timeout)? {
        Some(status) if status.success() => {
            let out = child.wait_with_output()?;
            Ok(String::from_utf8_lossy(&out.stdout).trim().to_string())
        }
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

    fn test_config(update_base_dir: PathBuf) -> Config {
        Config {
            update_enabled: true,
            update_repo_owner: "2high4schooltoday".to_string(),
            update_repo_name: "despatch".to_string(),
            update_http_timeout_sec: 10,
            update_github_token: String::new(),
            update_backup_keep: 3,
            update_base_dir,
            update_install_dir: PathBuf::from("/tmp/despatch-install-test"),
            update_service_name: "despatch".to_string(),
            update_systemd_unit_dir: PathBuf::from("/etc/systemd/system"),
            update_require_signature: false,
            update_signature_asset: "checksums.txt.sig".to_string(),
            update_signing_public_keys: Vec::new(),
            listen_addr: ":8080".to_string(),
            mailsec_enabled: false,
            mailsec_socket: PathBuf::from("/tmp/despatch-mailsec.sock"),
        }
    }

    #[test]
    fn updater_directory_contract_modes_are_stable() {
        let cfg = test_config(PathBuf::from("/tmp/despatch-update-contract-test"));
        let specs = updater_dir_specs(&cfg);
        let req = specs
            .iter()
            .find(|spec| spec.path == request_dir(&cfg))
            .expect("request dir spec");
        assert_eq!(req.mode, 0o770);
        assert!(matches!(req.group, UpdaterDirGroup::Despatch));

        let status = specs
            .iter()
            .find(|spec| spec.path == status_dir(&cfg))
            .expect("status dir spec");
        assert_eq!(status.mode, 0o770);
        assert!(matches!(status.group, UpdaterDirGroup::Despatch));

        let lock = specs
            .iter()
            .find(|spec| spec.path == lock_dir(&cfg))
            .expect("lock dir spec");
        assert_eq!(lock.mode, 0o750);
        assert!(matches!(lock.group, UpdaterDirGroup::Root));
    }

    #[test]
    fn ensure_updater_dirs_applies_mode_contract() {
        let unique = format!(
            "despatch-update-worker-{}-{}",
            std::process::id(),
            Utc::now().timestamp_nanos_opt().unwrap_or_default()
        );
        let base = std::env::temp_dir().join(unique);
        let cfg = test_config(base.clone());
        ensure_updater_dirs(&cfg).expect("ensure dirs");

        let req_mode = fs::metadata(request_dir(&cfg))
            .expect("request metadata")
            .permissions()
            .mode()
            & 0o777;
        let status_mode = fs::metadata(status_dir(&cfg))
            .expect("status metadata")
            .permissions()
            .mode()
            & 0o777;
        let lock_mode = fs::metadata(lock_dir(&cfg))
            .expect("lock metadata")
            .permissions()
            .mode()
            & 0o777;
        assert_eq!(req_mode, 0o770);
        assert_eq!(status_mode, 0o770);
        assert_eq!(lock_mode, 0o750);

        let _ = fs::remove_dir_all(base);
    }

    #[test]
    fn discard_invalid_request_payload_removes_request_file() {
        let unique = format!(
            "despatch-update-worker-invalid-request-{}-{}",
            std::process::id(),
            Utc::now().timestamp_nanos_opt().unwrap_or_default()
        );
        let base = std::env::temp_dir().join(unique);
        let cfg = test_config(base.clone());

        fs::create_dir_all(request_dir(&cfg)).expect("create request dir");
        let req_path = request_path(&cfg);
        fs::write(&req_path, b"{").expect("write malformed request");

        discard_invalid_request_payload(&cfg, &req_path, "unexpected end of json input");

        assert!(!req_path.exists());
        let _ = fs::remove_dir_all(base);
    }

    #[test]
    fn pending_request_paths_include_legacy_and_queue_requests() {
        let unique = format!(
            "despatch-update-worker-queue-{}-{}",
            std::process::id(),
            Utc::now().timestamp_nanos_opt().unwrap_or_default()
        );
        let base = std::env::temp_dir().join(unique);
        let cfg = test_config(base.clone());
        fs::create_dir_all(request_dir(&cfg)).expect("create request dir");
        fs::write(request_path(&cfg), b"{}").expect("write legacy request");
        let queued_req = ApplyRequest {
            request_id: "second".to_string(),
            requested_at: "1970-01-01T00:00:00Z".to_string(),
            requested_by: String::new(),
            mode: String::new(),
            target_version: String::new(),
        };
        fs::write(request_queue_path(&queued_req, &cfg), b"{}").expect("write queued request");

        let paths = pending_request_paths(&cfg).expect("pending request paths");
        assert_eq!(paths.len(), 2);
        assert_eq!(paths[0], request_path(&cfg));
        assert!(paths[1]
            .file_name()
            .and_then(|v| v.to_str())
            .unwrap_or_default()
            .starts_with("update-request-"));

        let _ = fs::remove_dir_all(base);
    }

    #[test]
    fn find_payload_root_requires_deploy_dir() {
        let unique = format!(
            "despatch-update-worker-payload-{}-{}",
            std::process::id(),
            Utc::now().timestamp_nanos_opt().unwrap_or_default()
        );
        let root = std::env::temp_dir().join(unique);
        fs::create_dir_all(root.join("web")).expect("mkdir web");
        fs::create_dir_all(root.join("migrations")).expect("mkdir migrations");
        fs::write(root.join("despatch"), b"ok").expect("write despatch");
        fs::write(root.join("despatch-pam-reset-helper"), b"ok").expect("write pam helper");
        fs::write(root.join("despatch-update-worker"), b"ok").expect("write update worker");

        assert!(find_payload_root(&root).is_err());

        fs::create_dir_all(root.join("deploy")).expect("mkdir deploy");
        assert!(find_payload_root(&root).is_ok());

        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn archive_candidates_include_aliases() {
        let out = archive_asset_candidates("aarch64");
        assert!(out.iter().any(|v| v == "despatch-linux-arm64.tar.gz"));
        assert!(out.iter().any(|v| v == "despatch-linux-aarch64.tar.gz"));
    }

    #[test]
    fn local_base_url_maps_unspecified_hosts() {
        assert_eq!(local_base_url(":8080"), "http://127.0.0.1:8080");
        assert_eq!(local_base_url("0.0.0.0:9090"), "http://127.0.0.1:9090");
    }

    #[test]
    fn systemd_load_state_known_filters_not_found() {
        assert!(!is_systemd_load_state_known(""));
        assert!(!is_systemd_load_state_known("not-found"));
        assert!(!is_systemd_load_state_known("error"));
        assert!(is_systemd_load_state_known("loaded"));
        assert!(is_systemd_load_state_known("masked"));
    }

    #[test]
    fn read_only_or_permission_error_detection() {
        assert!(is_read_only_or_permission_error(&WorkerError::Io(
            io::Error::from_raw_os_error(30)
        )));
        assert!(is_read_only_or_permission_error(&WorkerError::Io(
            io::Error::new(io::ErrorKind::PermissionDenied, "nope")
        )));
        assert!(!is_read_only_or_permission_error(&WorkerError::Message(
            "random failure".to_string()
        )));
    }

    #[test]
    fn migrate_legacy_password_reset_env_rewrites_broken_loopback_settings() {
        let unique = format!(
            "despatch-update-worker-reset-env-{}-{}",
            std::process::id(),
            Utc::now().timestamp_nanos_opt().unwrap_or_default()
        );
        let base = std::env::temp_dir().join(unique);
        fs::create_dir_all(&base).expect("mkdir temp");
        let env_path = base.join(".env");
        fs::write(
            &env_path,
            [
                "BASE_DOMAIN=mail.2h4s2d.ru",
                "DEPLOY_MODE=proxy",
                "PROXY_SERVER_NAME=mail.2h4s2d.ru",
                "PROXY_TLS=1",
                "LISTEN_ADDR=127.0.0.1:8080",
                "SMTP_HOST=127.0.0.1",
                "SMTP_PORT=587",
                "SMTP_TLS=false",
                "SMTP_STARTTLS=true",
                "PASSWORD_RESET_SENDER=smtp",
                "PASSWORD_RESET_FROM=no-reply@mail.2h4s2d.ru",
                "PASSWORD_RESET_BASE_URL=",
                "DOVECOT_AUTH_MODE=pam",
                "PASSWORD_RESET_EXTERNAL_SENDER_READY=false",
                "",
            ]
            .join("\n"),
        )
        .expect("write env");

        let migrated = migrate_legacy_password_reset_env(&env_path).expect("migrate env");
        assert!(migrated);

        let updated = fs::read_to_string(&env_path).expect("read migrated env");
        assert!(updated.contains("SMTP_PORT=25"));
        assert!(updated.contains("SMTP_STARTTLS=false"));
        assert!(updated.contains("SMTP_TLS=false"));
        assert!(updated.contains("PASSWORD_RESET_FROM=no-reply@2h4s2d.ru"));
        assert!(updated.contains("PASSWORD_RESET_BASE_URL=https://mail.2h4s2d.ru"));
        assert!(updated.contains("PASSWORD_RESET_EXTERNAL_SENDER_READY=true"));

        let _ = fs::remove_dir_all(base);
    }

    #[test]
    fn migrate_legacy_password_reset_env_skips_custom_smtp_setup() {
        let unique = format!(
            "despatch-update-worker-reset-env-skip-{}-{}",
            std::process::id(),
            Utc::now().timestamp_nanos_opt().unwrap_or_default()
        );
        let base = std::env::temp_dir().join(unique);
        fs::create_dir_all(&base).expect("mkdir temp");
        let env_path = base.join(".env");
        fs::write(
            &env_path,
            [
                "BASE_DOMAIN=mail.2h4s2d.ru",
                "SMTP_HOST=smtp.example.net",
                "SMTP_PORT=587",
                "SMTP_TLS=false",
                "SMTP_STARTTLS=true",
                "PASSWORD_RESET_SENDER=smtp",
                "PASSWORD_RESET_FROM=ops@example.net",
                "",
            ]
            .join("\n"),
        )
        .expect("write env");

        let migrated = migrate_legacy_password_reset_env(&env_path).expect("migrate env");
        assert!(!migrated);

        let _ = fs::remove_dir_all(base);
    }
}
