use serde::{Deserialize, Serialize};
use std::io::{Read, Write};

pub mod pam {
    use super::*;

    pub const MAX_FRAME_SIZE: usize = 8192;
    pub const MAX_USERNAME_BYTES: usize = 128;
    pub const MAX_PASSWORD_BYTES: usize = 4096;
    pub const CODE_OK: &str = "ok";
    pub const CODE_ERROR: &str = "helper_failed";

    #[derive(Debug, Serialize, Deserialize, Clone)]
    #[serde(deny_unknown_fields)]
    pub struct Request {
        pub request_id: String,
        pub username: String,
        pub new_password: String,
    }

    #[derive(Debug, Serialize, Deserialize, Clone)]
    #[serde(deny_unknown_fields)]
    pub struct Response {
        pub request_id: String,
        pub ok: bool,
        pub code: String,
    }

    pub fn validate_request(req: &Request) -> Result<(), String> {
        if req.request_id.trim().is_empty() {
            return Err("request_id is required".to_string());
        }
        let username = req.username.trim();
        if username.is_empty() {
            return Err("username is required".to_string());
        }
        if username.as_bytes().len() > MAX_USERNAME_BYTES {
            return Err("username exceeds limit".to_string());
        }
        if req.new_password.is_empty() {
            return Err("new_password is required".to_string());
        }
        if req.new_password.as_bytes().len() > MAX_PASSWORD_BYTES {
            return Err("new_password exceeds limit".to_string());
        }
        Ok(())
    }

    pub fn read_frame<R: Read>(mut reader: R, max_len: usize) -> Result<Vec<u8>, String> {
        let mut len_buf = [0u8; 4];
        reader
            .read_exact(&mut len_buf)
            .map_err(|e| format!("read frame length: {e}"))?;
        let n = u32::from_be_bytes(len_buf) as usize;
        if n == 0 || n > max_len {
            return Err("invalid frame length".to_string());
        }
        let mut out = vec![0u8; n];
        reader
            .read_exact(&mut out)
            .map_err(|e| format!("read frame payload: {e}"))?;
        Ok(out)
    }

    pub fn write_frame<W: Write>(mut writer: W, payload: &[u8]) -> Result<(), String> {
        if payload.is_empty() || payload.len() > MAX_FRAME_SIZE {
            return Err("invalid payload length".to_string());
        }
        let len_buf = (payload.len() as u32).to_be_bytes();
        writer
            .write_all(&len_buf)
            .map_err(|e| format!("write frame length: {e}"))?;
        writer
            .write_all(payload)
            .map_err(|e| format!("write frame payload: {e}"))?;
        Ok(())
    }

    pub fn safe_log_value(raw: &str) -> String {
        let trimmed = raw.trim();
        if trimmed.is_empty() {
            return "-".to_string();
        }
        trimmed
            .chars()
            .filter(|c| *c != '\n' && *c != '\r' && *c != '\t')
            .collect()
    }
}

pub mod updater {
    use super::*;

    pub const APPLY_STATE_IDLE: &str = "idle";
    pub const APPLY_STATE_QUEUED: &str = "queued";
    pub const APPLY_STATE_IN_PROGRESS: &str = "in_progress";
    pub const APPLY_STATE_COMPLETED: &str = "completed";
    pub const APPLY_STATE_FAILED: &str = "failed";
    pub const APPLY_STATE_ROLLED_BACK: &str = "rolled_back";
    pub const APPLY_MODE_APPLY: &str = "apply";
    pub const APPLY_MODE_PREPARE: &str = "prepare";

    #[derive(Debug, Serialize, Deserialize, Clone, Default)]
    #[serde(deny_unknown_fields)]
    pub struct ApplyRequest {
        #[serde(default)]
        pub request_id: String,
        #[serde(default)]
        pub requested_at: String,
        #[serde(default)]
        pub requested_by: String,
        #[serde(default)]
        pub mode: String,
        #[serde(default)]
        pub target_version: String,
    }

    #[derive(Debug, Serialize, Deserialize, Clone, Default)]
    #[serde(deny_unknown_fields)]
    pub struct ApplyStatus {
        pub state: String,
        #[serde(skip_serializing_if = "String::is_empty", default)]
        pub request_id: String,
        #[serde(skip_serializing_if = "String::is_empty", default)]
        pub requested_at: String,
        #[serde(skip_serializing_if = "String::is_empty", default)]
        pub started_at: String,
        #[serde(skip_serializing_if = "String::is_empty", default)]
        pub finished_at: String,
        #[serde(skip_serializing_if = "String::is_empty", default)]
        pub target_version: String,
        #[serde(skip_serializing_if = "String::is_empty", default)]
        pub from_version: String,
        #[serde(skip_serializing_if = "String::is_empty", default)]
        pub to_version: String,
        #[serde(skip_serializing_if = "is_false", default)]
        pub rolled_back: bool,
        #[serde(skip_serializing_if = "String::is_empty", default)]
        pub error: String,
    }

    fn is_false(v: &bool) -> bool {
        !*v
    }

    pub fn sanitize_path_token(v: &str) -> String {
        let trimmed = v.trim();
        if trimmed.is_empty() {
            return "unknown".to_string();
        }
        let mapped: String = trimmed
            .chars()
            .map(|c| {
                if c.is_ascii_alphanumeric() || matches!(c, '-' | '_' | '.') {
                    c
                } else {
                    '-'
                }
            })
            .collect();
        let out = mapped.trim_matches('-').to_string();
        if out.is_empty() {
            "unknown".to_string()
        } else {
            out
        }
    }
}

pub mod mailsec {
    use super::*;

    pub const MAX_FRAME_SIZE: usize = 8 * 1024 * 1024;
    pub const CODE_OK: &str = "ok";
    pub const CODE_INVALID_REQUEST: &str = "invalid_request";
    pub const CODE_INVALID_FRAME: &str = "invalid_frame";
    pub const CODE_UNSUPPORTED_OPERATION: &str = "unsupported_operation";
    pub const CODE_TIMEOUT: &str = "timeout";
    pub const CODE_INTERNAL_ERROR: &str = "internal_error";

    #[derive(Debug, Serialize, Deserialize, Clone)]
    #[serde(deny_unknown_fields)]
    pub struct Request {
        pub request_id: String,
        pub op: String,
        pub account_id: String,
        pub message_id: String,
        pub payload: serde_json::Value,
        pub deadline_ms: u64,
    }

    #[derive(Debug, Serialize, Deserialize, Clone)]
    #[serde(deny_unknown_fields)]
    pub struct Response {
        pub request_id: String,
        pub ok: bool,
        pub code: String,
        #[serde(default)]
        pub error: String,
        #[serde(default)]
        pub result: serde_json::Value,
    }

    pub fn validate_request(req: &Request) -> Result<(), String> {
        if req.request_id.trim().is_empty() {
            return Err("request_id is required".to_string());
        }
        if req.op.trim().is_empty() {
            return Err("op is required".to_string());
        }
        if req.account_id.trim().is_empty() {
            return Err("account_id is required".to_string());
        }
        if req.message_id.trim().is_empty() {
            return Err("message_id is required".to_string());
        }
        if req.deadline_ms == 0 {
            return Err("deadline_ms must be > 0".to_string());
        }
        Ok(())
    }

    pub fn read_frame<R: Read>(reader: R) -> Result<Vec<u8>, String> {
        super::pam::read_frame(reader, MAX_FRAME_SIZE)
    }

    pub fn write_frame<W: Write>(writer: W, payload: &[u8]) -> Result<(), String> {
        if payload.len() > MAX_FRAME_SIZE {
            return Err("invalid payload length".to_string());
        }
        super::pam::write_frame(writer, payload)
    }
}

#[cfg(test)]
mod tests {
    use super::mailsec;
    use super::pam;
    use super::updater;

    #[test]
    fn frame_round_trip() {
        let payload = br#"{"ok":true}"#;
        let mut out = Vec::new();
        pam::write_frame(&mut out, payload).expect("write frame");
        let back = pam::read_frame(&out[..], pam::MAX_FRAME_SIZE).expect("read frame");
        assert_eq!(back, payload);
    }

    #[test]
    fn sanitize_path_token_maps_symbols() {
        assert_eq!(
            updater::sanitize_path_token(" release/v1.0.0 "),
            "release-v1.0.0"
        );
    }

    #[test]
    fn mailsec_frame_round_trip() {
        let payload = br#"{"op":"mime.parse"}"#;
        let mut out = Vec::new();
        mailsec::write_frame(&mut out, payload).expect("write frame");
        let back = mailsec::read_frame(&out[..]).expect("read frame");
        assert_eq!(back, payload);
    }
}
