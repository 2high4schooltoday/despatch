use ammonia::Builder as AmmoniaBuilder;
use base64::engine::general_purpose::{URL_SAFE, URL_SAFE_NO_PAD};
use base64::Engine as _;
use ciborium::value::Value as CborValue;
use contracts::mailsec;
use data_encoding::{BASE32, BASE32_NOPAD};
use hmac::{Hmac, Mac};
use mailparse::{DispositionType, MailHeader, ParsedMail};
use openssl::pkcs7::{Pkcs7, Pkcs7Flags};
use openssl::pkey::{PKey, Private};
use openssl::stack::Stack;
use openssl::symm::Cipher;
use openssl::x509::store::X509StoreBuilder;
use openssl::x509::X509;
use p256::ecdsa::signature::Verifier;
use p256::ecdsa::{Signature as EcdsaSignature, VerifyingKey as EcdsaVerifyingKey};
use pgp::composed::{
    ArmorOptions, Deserializable, Message, MessageBuilder, SignedPublicKey, SignedSecretKey,
};
use pgp::crypto::hash::HashAlgorithm;
use pgp::crypto::sym::SymmetricKeyAlgorithm;
use pgp::types::{KeyDetails, Password};
use rand::thread_rng;
use regex::Regex;
use rsa::pkcs1v15::{Signature as RsaSignature, VerifyingKey as RsaVerifyingKey};
use rsa::{BigUint, RsaPublicKey};
use serde::Deserialize;
use serde_json::{json, Value as JsonValue};
use sha1::Sha1;
use sha2::{Digest, Sha256, Sha512};
use std::collections::HashSet;
use std::fs;
use std::io::Cursor;
use std::os::unix::fs::PermissionsExt;
use std::os::unix::net::{UnixListener, UnixStream};
use std::path::Path;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};
use subtle::ConstantTimeEq;
use thiserror::Error;

const DEFAULT_SOCKET: &str = "/run/despatch/mailsec.sock";
const FLAG_USER_PRESENT: u8 = 0x01;
const FLAG_USER_VERIFIED: u8 = 0x04;
const FLAG_ATTESTED_CREDENTIAL_DATA: u8 = 0x40;
const COSE_KTY_EC2: i64 = 2;
const COSE_KTY_RSA: i64 = 3;
const COSE_ALG_ES256: i64 = -7;
const COSE_ALG_RS256: i64 = -257;

const MAX_RAW_BYTES: usize = 8 * 1024 * 1024;
const MAX_BODY_TEXT_BYTES: usize = 1024 * 1024;
const MAX_BODY_HTML_BYTES: usize = 1024 * 1024;
const MAX_ATTACHMENTS: usize = 128;
const DEFAULT_SNIPPET_LEN: usize = 180;

#[derive(Debug, Error)]
enum ServiceError {
    #[error("io: {0}")]
    Io(#[from] std::io::Error),
    #[error("json: {0}")]
    Json(#[from] serde_json::Error),
}

#[derive(Debug, Deserialize)]
struct ClientData {
    #[serde(rename = "type")]
    typ: String,
    challenge: String,
    origin: String,
    #[serde(rename = "crossOrigin", default)]
    cross_origin: bool,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct RawPayload {
    #[serde(default)]
    raw_b64url: String,
    #[serde(default)]
    raw_base64: String,
    #[serde(default)]
    raw: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct HtmlSanitizePayload {
    html: String,
    #[serde(default)]
    allow_remote_images: bool,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct AuthVerifyPayload {
    #[serde(default)]
    raw_b64url: String,
    #[serde(default)]
    raw_base64: String,
    #[serde(default)]
    raw: String,
    #[serde(default)]
    from: String,
    #[serde(default)]
    reply_to: String,
    #[serde(default)]
    subject: String,
    #[serde(default)]
    body_text: String,
    #[serde(default)]
    body_html: String,
    #[serde(default)]
    attachment_names: Vec<String>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct TOTPVerifyPayload {
    secret: String,
    code: String,
    #[serde(default)]
    time_unix: Option<u64>,
    #[serde(default)]
    period: u64,
    #[serde(default)]
    digits: u32,
    #[serde(default)]
    algorithm: String,
    #[serde(default)]
    skew_past: u64,
    #[serde(default)]
    skew_future: u64,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct PGPEncryptPayload {
    #[serde(default)]
    plaintext_b64url: String,
    #[serde(default)]
    plaintext: String,
    recipient_public_keys: Vec<String>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct PGPDecryptPayload {
    #[serde(default)]
    ciphertext_armored: String,
    #[serde(default)]
    ciphertext_b64url: String,
    private_key_armored: String,
    #[serde(default)]
    passphrase: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct PGPSignPayload {
    #[serde(default)]
    plaintext_b64url: String,
    #[serde(default)]
    plaintext: String,
    private_key_armored: String,
    #[serde(default)]
    passphrase: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct PGPVerifyPayload {
    #[serde(default)]
    signed_message_armored: String,
    #[serde(default)]
    signed_message_b64url: String,
    public_key_armored: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct SMIMEEncryptPayload {
    #[serde(default)]
    plaintext_b64url: String,
    #[serde(default)]
    plaintext: String,
    recipient_certs_pem: Vec<String>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct SMIMEDecryptPayload {
    #[serde(default)]
    ciphertext_smime: String,
    #[serde(default)]
    ciphertext_b64url: String,
    private_key_pem: String,
    cert_pem: String,
    #[serde(default)]
    private_key_passphrase: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct SMIMESignPayload {
    #[serde(default)]
    plaintext_b64url: String,
    #[serde(default)]
    plaintext: String,
    private_key_pem: String,
    cert_pem: String,
    #[serde(default)]
    private_key_passphrase: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct SMIMEVerifyPayload {
    #[serde(default)]
    signed_smime: String,
    #[serde(default)]
    signed_b64url: String,
    #[serde(default)]
    trusted_certs_pem: Vec<String>,
}

#[derive(Debug, Clone, Copy)]
enum TotpAlgorithm {
    Sha1,
    Sha256,
    Sha512,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct WebAuthnRegistrationFinishPayload {
    challenge: String,
    rp_id: String,
    #[serde(default)]
    origins: Vec<String>,
    #[serde(default)]
    origin: String,
    client_data_json_b64url: String,
    attestation_object_b64url: String,
    #[serde(default)]
    credential_id: String,
    #[serde(default)]
    require_user_verification: bool,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct WebAuthnAssertionFinishPayload {
    challenge: String,
    rp_id: String,
    #[serde(default)]
    origins: Vec<String>,
    #[serde(default)]
    origin: String,
    credential_id: String,
    client_data_json_b64url: String,
    authenticator_data_b64url: String,
    signature_b64url: String,
    stored_public_key_cose_b64url: String,
    #[serde(default)]
    stored_sign_count: u64,
    #[serde(default)]
    require_user_verification: bool,
}

#[derive(Debug)]
struct ParsedAuthenticatorData {
    rp_id_hash: [u8; 32],
    flags: u8,
    sign_count: u32,
    credential_id: Option<Vec<u8>>,
    credential_public_key: Option<Vec<u8>>,
}

#[derive(Debug, Default, Clone)]
struct AttachmentInfo {
    id: String,
    filename: String,
    content_type: String,
    size_bytes: u64,
    inline: bool,
}

#[derive(Debug, Default)]
struct ParsedMimeMessage {
    subject: String,
    from: String,
    to: String,
    cc: String,
    bcc: String,
    reply_to: String,
    date: String,
    message_id: String,
    in_reply_to: String,
    references: String,
    body_text: String,
    body_html: String,
    snippet: String,
    remote_image_count: usize,
    attachments: Vec<AttachmentInfo>,
    auth_results_headers: Vec<String>,
}

#[derive(Debug, Default)]
struct BodyExtraction {
    body_text: String,
    body_html: String,
    attachments: Vec<AttachmentInfo>,
}

fn main() {
    if let Err(err) = run() {
        eprintln!("mailsec service failed: {err}");
        std::process::exit(1);
    }
}

fn run() -> Result<(), ServiceError> {
    let socket_path =
        std::env::var("MAILSEC_SOCKET").unwrap_or_else(|_| DEFAULT_SOCKET.to_string());
    if let Some(parent) = Path::new(&socket_path).parent() {
        fs::create_dir_all(parent)?;
    }
    if Path::new(&socket_path).exists() {
        let _ = fs::remove_file(&socket_path);
    }
    let listener = UnixListener::bind(&socket_path)?;
    fs::set_permissions(&socket_path, fs::Permissions::from_mode(0o660))?;

    for stream in listener.incoming() {
        match stream {
            Ok(mut conn) => {
                if let Err(err) = handle_connection(&mut conn) {
                    eprintln!("mailsec connection error: {err}");
                }
            }
            Err(err) => eprintln!("mailsec accept error: {err}"),
        }
    }
    Ok(())
}

fn handle_connection(conn: &mut UnixStream) -> Result<(), ServiceError> {
    conn.set_read_timeout(Some(Duration::from_secs(15)))?;
    conn.set_write_timeout(Some(Duration::from_secs(15)))?;

    let payload = mailsec::read_frame(&mut *conn).map_err(std::io::Error::other)?;
    let req: mailsec::Request = serde_json::from_slice(&payload)?;
    let started = Instant::now();

    let response = match mailsec::validate_request(&req) {
        Ok(()) => run_operation(&req, started),
        Err(err) => mailsec::Response {
            request_id: req.request_id.clone(),
            ok: false,
            code: mailsec::CODE_INVALID_REQUEST.to_string(),
            error: err,
            result: json!({}),
        },
    };

    let body = serde_json::to_vec(&response)?;
    mailsec::write_frame(&mut *conn, &body).map_err(std::io::Error::other)?;
    Ok(())
}

fn run_operation(req: &mailsec::Request, started: Instant) -> mailsec::Response {
    let allowed: HashSet<&str> = HashSet::from([
        "mime.parse",
        "mime.extract_attachments",
        "html.sanitize",
        "auth.verify",
        "totp.verify",
        "crypto.pgp.sign",
        "crypto.pgp.encrypt",
        "crypto.pgp.decrypt",
        "crypto.pgp.verify",
        "crypto.smime.sign",
        "crypto.smime.encrypt",
        "crypto.smime.decrypt",
        "crypto.smime.verify",
        "webauthn.register.finish",
        "webauthn.assertion.finish",
    ]);

    if !allowed.contains(req.op.as_str()) {
        return mailsec::Response {
            request_id: req.request_id.clone(),
            ok: false,
            code: mailsec::CODE_UNSUPPORTED_OPERATION.to_string(),
            error: format!("unsupported op: {}", req.op),
            result: json!({}),
        };
    }

    if started.elapsed().as_millis() as u64 > req.deadline_ms {
        return mailsec::Response {
            request_id: req.request_id.clone(),
            ok: false,
            code: mailsec::CODE_TIMEOUT.to_string(),
            error: "deadline exceeded".to_string(),
            result: json!({}),
        };
    }

    let result = match req.op.as_str() {
        "mime.parse" => parse_mime_operation(req.payload.clone()),
        "mime.extract_attachments" => extract_attachments_operation(req.payload.clone()),
        "html.sanitize" => sanitize_html_operation(req.payload.clone()),
        "auth.verify" => verify_auth_operation(req.payload.clone()),
        "totp.verify" => verify_totp_operation(req.payload.clone()),
        "crypto.pgp.sign" => pgp_sign_operation(req.payload.clone()),
        "crypto.pgp.encrypt" => pgp_encrypt_operation(req.payload.clone()),
        "crypto.pgp.decrypt" => pgp_decrypt_operation(req.payload.clone()),
        "crypto.pgp.verify" => pgp_verify_operation(req.payload.clone()),
        "crypto.smime.sign" => smime_sign_operation(req.payload.clone()),
        "crypto.smime.encrypt" => smime_encrypt_operation(req.payload.clone()),
        "crypto.smime.decrypt" => smime_decrypt_operation(req.payload.clone()),
        "crypto.smime.verify" => smime_verify_operation(req.payload.clone()),
        "webauthn.register.finish" => verify_webauthn_registration(req.payload.clone()),
        "webauthn.assertion.finish" => verify_webauthn_assertion(req.payload.clone()),
        _ => Err(format!("unsupported operation: {}", req.op)),
    };

    match result {
        Ok(payload) => mailsec::Response {
            request_id: req.request_id.clone(),
            ok: true,
            code: mailsec::CODE_OK.to_string(),
            error: String::new(),
            result: payload,
        },
        Err(err) => {
            let code = if err.starts_with("unsupported operation:") {
                mailsec::CODE_UNSUPPORTED_OPERATION
            } else {
                mailsec::CODE_INVALID_REQUEST
            };
            mailsec::Response {
                request_id: req.request_id.clone(),
                ok: false,
                code: code.to_string(),
                error: err,
                result: json!({}),
            }
        }
    }
}

fn parse_mime_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let raw_req: RawPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;
    let raw = decode_raw_payload(&raw_req)?;
    let parsed = parse_mime_message(&raw)?;

    let attachments = parsed
        .attachments
        .iter()
        .map(|a| {
            json!({
                "id": a.id,
                "filename": a.filename,
                "content_type": a.content_type,
                "size_bytes": a.size_bytes,
                "inline": a.inline,
            })
        })
        .collect::<Vec<_>>();

    Ok(json!({
        "subject": parsed.subject,
        "from": parsed.from,
        "to": parsed.to,
        "cc": parsed.cc,
        "bcc": parsed.bcc,
        "reply_to": parsed.reply_to,
        "date": parsed.date,
        "message_id": parsed.message_id,
        "in_reply_to": parsed.in_reply_to,
        "references": parsed.references,
        "snippet": parsed.snippet,
        "body_text": parsed.body_text,
        "body_html": parsed.body_html,
        "has_attachments": !attachments.is_empty(),
        "remote_image_count": parsed.remote_image_count,
        "attachments": attachments,
    }))
}

fn extract_attachments_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let raw_req: RawPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;
    let raw = decode_raw_payload(&raw_req)?;
    let parsed = parse_mime_message(&raw)?;

    let attachments = parsed
        .attachments
        .iter()
        .map(|a| {
            json!({
                "id": a.id,
                "filename": a.filename,
                "content_type": a.content_type,
                "size_bytes": a.size_bytes,
                "inline": a.inline,
            })
        })
        .collect::<Vec<_>>();

    Ok(json!({
        "has_attachments": !attachments.is_empty(),
        "attachments": attachments,
    }))
}

fn sanitize_html_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let req: HtmlSanitizePayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;

    let mut html = req.html;
    let mut blocked_remote_image_count = 0usize;
    if !req.allow_remote_images {
        let remote_img_re =
            Regex::new(r#"(?is)<img\b[^>]*\bsrc\s*=\s*(?:\\?["'])?(?:https?:)?//[^>]*>"#)
                .map_err(|err| format!("invalid sanitizer regex: {err}"))?;
        blocked_remote_image_count = remote_img_re.find_iter(&html).count();
        html = remote_img_re.replace_all(&html, "").to_string();
    }

    let sanitized = AmmoniaBuilder::default().clean(&html).to_string();

    Ok(json!({
        "html": sanitized,
        "remote_images_blocked": blocked_remote_image_count > 0,
        "blocked_remote_image_count": blocked_remote_image_count,
    }))
}

fn verify_auth_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let req: AuthVerifyPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;

    let mut parsed = ParsedMimeMessage::default();
    if has_raw_payload(&req.raw_b64url, &req.raw_base64, &req.raw) {
        let raw_req = RawPayload {
            raw_b64url: req.raw_b64url.clone(),
            raw_base64: req.raw_base64.clone(),
            raw: req.raw.clone(),
        };
        let raw = decode_raw_payload(&raw_req)?;
        parsed = parse_mime_message(&raw)?;
    }

    let from = first_non_empty(vec![req.from.clone(), parsed.from.clone()]);
    let reply_to = first_non_empty(vec![req.reply_to.clone(), parsed.reply_to.clone()]);
    let subject = first_non_empty(vec![req.subject.clone(), parsed.subject.clone()]);
    let body_text = first_non_empty(vec![req.body_text.clone(), parsed.body_text.clone()]);
    let body_html = first_non_empty(vec![req.body_html.clone(), parsed.body_html.clone()]);

    let mut attachment_names = req.attachment_names.clone();
    if attachment_names.is_empty() {
        attachment_names = parsed
            .attachments
            .iter()
            .map(|a| a.filename.clone())
            .filter(|s| !s.trim().is_empty())
            .collect();
    }

    let dkim = parse_auth_status(&parsed.auth_results_headers, "dkim");
    let spf = parse_auth_status(&parsed.auth_results_headers, "spf");
    let dmarc = parse_auth_status(&parsed.auth_results_headers, "dmarc");

    let from_domain = extract_email_domain(&from);
    let reply_to_domain = extract_email_domain(&reply_to);

    let mut phishing_score = 0.0_f64;
    let mut indicators: Vec<String> = Vec::new();

    let dkim_lower = dkim.to_lowercase();
    let spf_lower = spf.to_lowercase();
    let dmarc_lower = dmarc.to_lowercase();
    if is_auth_failed(&dkim_lower) {
        phishing_score += 0.20;
        indicators.push("dkim_failed".to_string());
    }
    if is_auth_failed(&spf_lower) {
        phishing_score += 0.20;
        indicators.push("spf_failed".to_string());
    }
    if is_auth_failed(&dmarc_lower) {
        phishing_score += 0.20;
        indicators.push("dmarc_failed".to_string());
    }

    if !from_domain.is_empty() && !reply_to_domain.is_empty() && from_domain != reply_to_domain {
        phishing_score += 0.25;
        indicators.push("from_replyto_domain_mismatch".to_string());
    }

    if from_domain.starts_with("xn--") {
        phishing_score += 0.20;
        indicators.push("punycode_from_domain".to_string());
    }

    let subject_lc = subject.to_lowercase();
    let bait_terms = [
        "urgent",
        "verify account",
        "password",
        "invoice",
        "suspended",
        "action required",
        "security alert",
    ];
    if bait_terms.iter().any(|term| subject_lc.contains(term)) {
        phishing_score += 0.15;
        indicators.push("suspicious_subject_pattern".to_string());
    }

    let combined_body = format!("{} {}", body_text, body_html);
    let urls = extract_urls(&combined_body)?;
    if urls.len() >= 4 {
        phishing_score += 0.10;
        indicators.push("high_link_density".to_string());
    }
    if urls.iter().any(|u| {
        u.contains("xn--")
            || u.ends_with(".zip")
            || u.ends_with(".mov")
            || u.ends_with(".top")
            || u.ends_with(".xyz")
    }) {
        phishing_score += 0.10;
        indicators.push("suspicious_link_domain".to_string());
    }

    if attachment_names
        .iter()
        .any(|name| has_executable_attachment(name))
    {
        phishing_score += 0.25;
        indicators.push("executable_attachment".to_string());
    }

    if phishing_score > 1.0 {
        phishing_score = 1.0;
    }

    Ok(json!({
        "dkim": dkim,
        "spf": spf,
        "dmarc": dmarc,
        "phishing_score": ((phishing_score * 1000.0).round() / 1000.0),
        "from_domain": from_domain,
        "reply_to_domain": reply_to_domain,
        "indicators": indicators,
    }))
}

fn verify_totp_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let req: TOTPVerifyPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;

    let period = if req.period == 0 { 30 } else { req.period };
    if period == 0 {
        return Err("period must be > 0".to_string());
    }
    let digits = if req.digits == 0 { 6 } else { req.digits };
    if !(6..=8).contains(&digits) {
        return Err("digits must be between 6 and 8".to_string());
    }

    let algorithm = TotpAlgorithm::from_string(&req.algorithm)?;
    let secret = decode_totp_secret(&req.secret)?;

    let code = req
        .code
        .chars()
        .filter(|ch| ch.is_ascii_digit())
        .collect::<String>();
    if code.len() != digits as usize {
        return Err("code has invalid length".to_string());
    }
    if code.is_empty() {
        return Err("code is required".to_string());
    }

    let now_unix = req.time_unix.unwrap_or_else(current_unix_seconds);
    let current_counter = now_unix / period;

    let skew_past = if req.skew_past == 0 {
        1
    } else {
        req.skew_past.min(10)
    };
    let skew_future = if req.skew_future == 0 {
        1
    } else {
        req.skew_future.min(10)
    };

    let start = current_counter.saturating_sub(skew_past);
    let end = current_counter.saturating_add(skew_future);

    let mut valid = false;
    let mut matched_counter: i64 = -1;

    for counter in start..=end {
        let value = hotp(&secret, counter, digits, algorithm)?;
        let candidate = format!("{:0width$}", value, width = digits as usize);
        if candidate.as_bytes().ct_eq(code.as_bytes()).unwrap_u8() == 1 {
            valid = true;
            matched_counter = counter as i64;
            break;
        }
    }

    Ok(json!({
        "valid": valid,
        "matched_counter": matched_counter,
        "current_counter": current_counter,
        "period": period,
        "digits": digits,
    }))
}

fn pgp_sign_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let req: PGPSignPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;
    let plaintext = decode_binary_or_text(&req.plaintext_b64url, &req.plaintext, "plaintext")?;
    let secret_key = parse_pgp_secret_key(&req.private_key_armored)?;
    secret_key
        .verify_bindings()
        .map_err(|err| format!("invalid signing key bindings: {err}"))?;

    let mut rng = thread_rng();
    let mut builder = MessageBuilder::from_bytes("", plaintext.clone());
    builder.sign(
        &secret_key.primary_key,
        Password::from(req.passphrase.as_str()),
        HashAlgorithm::Sha256,
    );
    let signed_armored = builder
        .to_armored_string(&mut rng, ArmorOptions::default())
        .map_err(|err| format!("pgp sign failed: {err}"))?;
    let signed = signed_armored.as_bytes();

    Ok(json!({
        "signed_message_armored": signed_armored,
        "signed_message_b64url": URL_SAFE_NO_PAD.encode(signed),
        "signer_fingerprint": format!("{}", secret_key.fingerprint()),
    }))
}

fn pgp_encrypt_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let req: PGPEncryptPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;
    let plaintext = decode_binary_or_text(&req.plaintext_b64url, &req.plaintext, "plaintext")?;
    if req.recipient_public_keys.is_empty() {
        return Err("recipient_public_keys is required".to_string());
    }

    let mut recipients = Vec::with_capacity(req.recipient_public_keys.len());
    for raw in &req.recipient_public_keys {
        let cert = parse_pgp_public_key(raw)?;
        cert.verify_bindings()
            .map_err(|err| format!("invalid recipient public key bindings: {err}"))?;
        recipients.push(cert);
    }

    let mut rng = thread_rng();
    let mut builder = MessageBuilder::from_bytes("", plaintext.clone())
        .seipd_v1(&mut rng, SymmetricKeyAlgorithm::AES256);
    for cert in &recipients {
        let mut added = false;
        for sub in &cert.public_subkeys {
            if sub.algorithm().can_encrypt() {
                builder
                    .encrypt_to_key(&mut rng, sub)
                    .map_err(|err| format!("pgp encrypt_to_key failed: {err}"))?;
                added = true;
                break;
            }
        }
        if !added && cert.primary_key.algorithm().can_encrypt() {
            builder
                .encrypt_to_key(&mut rng, &cert.primary_key)
                .map_err(|err| format!("pgp encrypt_to_primary failed: {err}"))?;
            added = true;
        }
        if !added {
            return Err(format!(
                "recipient key {} has no encryption-capable key material",
                cert.fingerprint()
            ));
        }
    }

    let encrypted_armored = builder
        .to_armored_string(&mut rng, ArmorOptions::default())
        .map_err(|err| format!("pgp encrypt failed: {err}"))?;
    let encrypted = encrypted_armored.as_bytes();

    Ok(json!({
        "ciphertext_armored": encrypted_armored,
        "ciphertext_b64url": URL_SAFE_NO_PAD.encode(encrypted),
        "recipient_count": recipients.len(),
    }))
}

fn pgp_decrypt_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let req: PGPDecryptPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;
    let ciphertext = decode_binary_or_text(
        &req.ciphertext_b64url,
        &req.ciphertext_armored,
        "ciphertext",
    )?;
    let secret_key = parse_pgp_secret_key(&req.private_key_armored)?;
    secret_key
        .verify_bindings()
        .map_err(|err| format!("invalid recipient secret key bindings: {err}"))?;

    let message = parse_pgp_message(&ciphertext)?;
    let mut decrypted = message
        .decrypt(&Password::from(req.passphrase.as_str()), &secret_key)
        .map_err(|err| format!("pgp decrypt failed: {err}"))?;
    if decrypted.is_compressed() {
        decrypted = decrypted
            .decompress()
            .map_err(|err| format!("pgp decompress failed: {err}"))?;
    }
    let plaintext = decrypted
        .as_data_vec()
        .map_err(|err| format!("pgp plaintext extraction failed: {err}"))?;

    Ok(json!({
        "plaintext_b64url": URL_SAFE_NO_PAD.encode(&plaintext),
        "plaintext_utf8": String::from_utf8_lossy(&plaintext),
        "recipient_fingerprint": format!("{}", secret_key.fingerprint()),
    }))
}

fn pgp_verify_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let req: PGPVerifyPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;
    let signed_message = decode_binary_or_text(
        &req.signed_message_b64url,
        &req.signed_message_armored,
        "signed_message",
    )?;
    let public_key = parse_pgp_public_key(&req.public_key_armored)?;
    public_key
        .verify_bindings()
        .map_err(|err| format!("invalid verifier public key bindings: {err}"))?;

    let mut message = parse_pgp_message(&signed_message)?;
    message
        .verify_read(&public_key)
        .map_err(|err| format!("pgp verification failed: {err}"))?;
    let plaintext = message
        .as_data_vec()
        .map_err(|err| format!("pgp plaintext extraction failed: {err}"))?;

    Ok(json!({
        "valid": true,
        "plaintext_b64url": URL_SAFE_NO_PAD.encode(&plaintext),
        "plaintext_utf8": String::from_utf8_lossy(&plaintext),
        "signer_fingerprint": format!("{}", public_key.fingerprint()),
    }))
}

fn smime_sign_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let req: SMIMESignPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;
    let plaintext = decode_binary_or_text(&req.plaintext_b64url, &req.plaintext, "plaintext")?;
    let cert = parse_smime_cert(&req.cert_pem)?;
    let pkey = parse_smime_private_key(&req.private_key_pem, &req.private_key_passphrase)?;
    let certs = Stack::new().map_err(|err| format!("smime cert stack init failed: {err}"))?;
    let pkcs7 = Pkcs7::sign(&cert, &pkey, &certs, &plaintext, Pkcs7Flags::BINARY)
        .map_err(|err| format!("smime sign failed: {err}"))?;
    let smime = pkcs7
        .to_smime(&plaintext, Pkcs7Flags::BINARY)
        .map_err(|err| format!("smime serialization failed: {err}"))?;

    Ok(json!({
        "signed_smime": String::from_utf8_lossy(&smime),
        "signer_fingerprint_sha256": cert_fingerprint_sha256(&cert),
    }))
}

fn smime_encrypt_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let req: SMIMEEncryptPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;
    let plaintext = decode_binary_or_text(&req.plaintext_b64url, &req.plaintext, "plaintext")?;
    if req.recipient_certs_pem.is_empty() {
        return Err("recipient_certs_pem is required".to_string());
    }
    let certs = parse_smime_certs_stack(&req.recipient_certs_pem)?;
    let pkcs7 = Pkcs7::encrypt(
        &certs,
        &plaintext,
        Cipher::aes_256_cbc(),
        Pkcs7Flags::BINARY,
    )
    .map_err(|err| format!("smime encrypt failed: {err}"))?;
    let smime = pkcs7
        .to_smime(&plaintext, Pkcs7Flags::BINARY)
        .map_err(|err| format!("smime serialization failed: {err}"))?;
    Ok(json!({
        "ciphertext_smime": String::from_utf8_lossy(&smime),
        "recipient_count": req.recipient_certs_pem.len(),
    }))
}

fn smime_decrypt_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let req: SMIMEDecryptPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;
    let ciphertext =
        decode_binary_or_text(&req.ciphertext_b64url, &req.ciphertext_smime, "ciphertext")?;
    let pkey = parse_smime_private_key(&req.private_key_pem, &req.private_key_passphrase)?;
    let cert = parse_smime_cert(&req.cert_pem)?;

    let plaintext = if let Ok((pkcs7, _)) = Pkcs7::from_smime(&ciphertext) {
        pkcs7
            .decrypt(&pkey, &cert, Pkcs7Flags::BINARY)
            .map_err(|err| format!("smime decrypt failed: {err}"))?
    } else {
        let pkcs7 =
            Pkcs7::from_der(&ciphertext).map_err(|err| format!("smime parse failed: {err}"))?;
        pkcs7
            .decrypt(&pkey, &cert, Pkcs7Flags::BINARY)
            .map_err(|err| format!("smime decrypt failed: {err}"))?
    };

    Ok(json!({
        "plaintext_b64url": URL_SAFE_NO_PAD.encode(&plaintext),
        "plaintext_utf8": String::from_utf8_lossy(&plaintext),
    }))
}

fn smime_verify_operation(payload: JsonValue) -> Result<JsonValue, String> {
    let req: SMIMEVerifyPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;
    let signed = decode_binary_or_text(&req.signed_b64url, &req.signed_smime, "signed")?;
    if req.trusted_certs_pem.is_empty() {
        return Err("trusted_certs_pem is required".to_string());
    }
    let trusted = parse_smime_certs_stack(&req.trusted_certs_pem)?;
    let mut store_builder =
        X509StoreBuilder::new().map_err(|err| format!("smime trust store init failed: {err}"))?;
    for cert in &trusted {
        store_builder
            .add_cert(cert.to_owned())
            .map_err(|err| format!("smime trust store add failed: {err}"))?;
    }
    let store = store_builder.build();

    let (pkcs7, indata) =
        Pkcs7::from_smime(&signed).map_err(|err| format!("smime parse failed: {err}"))?;
    let mut out = Vec::new();
    pkcs7
        .verify(
            &trusted,
            &store,
            indata.as_deref(),
            Some(&mut out),
            Pkcs7Flags::BINARY,
        )
        .map_err(|err| format!("smime verify failed: {err}"))?;

    let signers = pkcs7
        .signers(&trusted, Pkcs7Flags::empty())
        .map_err(|err| format!("smime signer extraction failed: {err}"))?;
    let signer_subjects = signers
        .iter()
        .map(|cert| cert.subject_name().entries().next())
        .map(|entry_opt| {
            entry_opt
                .and_then(|entry| entry.data().as_utf8().ok().map(|v| v.to_string()))
                .unwrap_or_default()
        })
        .filter(|value| !value.trim().is_empty())
        .collect::<Vec<_>>();

    Ok(json!({
        "valid": true,
        "plaintext_b64url": URL_SAFE_NO_PAD.encode(&out),
        "plaintext_utf8": String::from_utf8_lossy(&out),
        "signer_subjects": signer_subjects,
    }))
}

fn decode_binary_or_text(b64url: &str, text: &str, field: &str) -> Result<Vec<u8>, String> {
    if !b64url.trim().is_empty() {
        return decode_b64url_field(b64url, field);
    }
    if !text.trim().is_empty() {
        return Ok(text.as_bytes().to_vec());
    }
    Err(format!("{field} is required"))
}

fn parse_pgp_secret_key(raw: &str) -> Result<SignedSecretKey, String> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return Err("private_key_armored is required".to_string());
    }
    SignedSecretKey::from_string(trimmed)
        .map(|(key, _)| key)
        .map_err(|err| format!("invalid pgp secret key: {err}"))
}

fn parse_pgp_public_key(raw: &str) -> Result<SignedPublicKey, String> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return Err("public key is required".to_string());
    }
    SignedPublicKey::from_string(trimmed)
        .map(|(key, _)| key)
        .map_err(|err| format!("invalid pgp public key: {err}"))
}

fn parse_pgp_message(raw: &[u8]) -> Result<Message<'_>, String> {
    Message::from_armor(Cursor::new(raw))
        .map(|(msg, _)| msg)
        .or_else(|_| Message::from_bytes(raw))
        .map_err(|err| format!("invalid pgp message: {err}"))
}

fn parse_smime_cert(cert_pem: &str) -> Result<X509, String> {
    X509::from_pem(cert_pem.trim().as_bytes())
        .map_err(|err| format!("invalid smime cert pem: {err}"))
}

fn parse_smime_private_key(
    private_key_pem: &str,
    passphrase: &str,
) -> Result<PKey<Private>, String> {
    let pem = private_key_pem.trim().as_bytes();
    if pem.is_empty() {
        return Err("private key is required".to_string());
    }
    if passphrase.trim().is_empty() {
        PKey::private_key_from_pem(pem)
            .map_err(|err| format!("invalid smime private key pem: {err}"))
    } else {
        PKey::private_key_from_pem_passphrase(pem, passphrase.as_bytes())
            .map_err(|err| format!("invalid smime private key/passphrase: {err}"))
    }
}

fn parse_smime_certs_stack(cert_pems: &[String]) -> Result<Stack<X509>, String> {
    let mut stack = Stack::new().map_err(|err| format!("smime cert stack init failed: {err}"))?;
    for cert_pem in cert_pems {
        let cert = parse_smime_cert(cert_pem)?;
        stack
            .push(cert)
            .map_err(|err| format!("smime cert stack push failed: {err}"))?;
    }
    Ok(stack)
}

fn cert_fingerprint_sha256(cert: &X509) -> String {
    cert.digest(openssl::hash::MessageDigest::sha256())
        .map(|digest| {
            digest
                .iter()
                .map(|b| format!("{:02X}", b))
                .collect::<String>()
        })
        .unwrap_or_default()
}

impl TotpAlgorithm {
    fn from_string(raw: &str) -> Result<Self, String> {
        let alg = raw.trim();
        if alg.is_empty() || alg.eq_ignore_ascii_case("SHA1") {
            return Ok(Self::Sha1);
        }
        if alg.eq_ignore_ascii_case("SHA256") {
            return Ok(Self::Sha256);
        }
        if alg.eq_ignore_ascii_case("SHA512") {
            return Ok(Self::Sha512);
        }
        Err("algorithm must be SHA1, SHA256, or SHA512".to_string())
    }
}

fn hotp(secret: &[u8], counter: u64, digits: u32, algorithm: TotpAlgorithm) -> Result<u32, String> {
    let msg = counter.to_be_bytes();
    let digest = match algorithm {
        TotpAlgorithm::Sha1 => {
            let mut mac = Hmac::<Sha1>::new_from_slice(secret)
                .map_err(|err| format!("invalid secret for HMAC-SHA1: {err}"))?;
            mac.update(&msg);
            mac.finalize().into_bytes().to_vec()
        }
        TotpAlgorithm::Sha256 => {
            let mut mac = Hmac::<Sha256>::new_from_slice(secret)
                .map_err(|err| format!("invalid secret for HMAC-SHA256: {err}"))?;
            mac.update(&msg);
            mac.finalize().into_bytes().to_vec()
        }
        TotpAlgorithm::Sha512 => {
            let mut mac = Hmac::<Sha512>::new_from_slice(secret)
                .map_err(|err| format!("invalid secret for HMAC-SHA512: {err}"))?;
            mac.update(&msg);
            mac.finalize().into_bytes().to_vec()
        }
    };

    let offset = (digest[digest.len() - 1] & 0x0f) as usize;
    if offset + 3 >= digest.len() {
        return Err("invalid hmac digest".to_string());
    }

    let binary = ((u32::from(digest[offset]) & 0x7f) << 24)
        | (u32::from(digest[offset + 1]) << 16)
        | (u32::from(digest[offset + 2]) << 8)
        | u32::from(digest[offset + 3]);

    let modulo = 10u32
        .checked_pow(digits)
        .ok_or_else(|| "invalid digits range".to_string())?;
    Ok(binary % modulo)
}

fn decode_totp_secret(raw: &str) -> Result<Vec<u8>, String> {
    let cleaned = raw
        .chars()
        .filter(|ch| !ch.is_whitespace() && *ch != '-')
        .collect::<String>()
        .to_uppercase();
    if cleaned.is_empty() {
        return Err("secret is required".to_string());
    }
    BASE32_NOPAD
        .decode(cleaned.as_bytes())
        .or_else(|_| BASE32.decode(cleaned.as_bytes()))
        .map_err(|err| format!("invalid base32 secret: {err}"))
}

fn current_unix_seconds() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

fn decode_raw_payload(req: &RawPayload) -> Result<Vec<u8>, String> {
    if !req.raw_b64url.trim().is_empty() {
        return decode_b64url_field(&req.raw_b64url, "raw_b64url").and_then(validate_raw_size);
    }
    if !req.raw_base64.trim().is_empty() {
        return URL_SAFE
            .decode(req.raw_base64.trim())
            .or_else(|_| URL_SAFE_NO_PAD.decode(req.raw_base64.trim()))
            .map_err(|err| format!("invalid raw_base64: {err}"))
            .and_then(validate_raw_size);
    }
    if !req.raw.trim().is_empty() {
        return validate_raw_size(req.raw.as_bytes().to_vec());
    }
    Err("raw payload is required".to_string())
}

fn has_raw_payload(raw_b64url: &str, raw_base64: &str, raw: &str) -> bool {
    !raw_b64url.trim().is_empty() || !raw_base64.trim().is_empty() || !raw.trim().is_empty()
}

fn validate_raw_size(raw: Vec<u8>) -> Result<Vec<u8>, String> {
    if raw.is_empty() {
        return Err("raw payload is empty".to_string());
    }
    if raw.len() > MAX_RAW_BYTES {
        return Err("raw payload exceeds max allowed size".to_string());
    }
    Ok(raw)
}

fn parse_mime_message(raw: &[u8]) -> Result<ParsedMimeMessage, String> {
    let parsed =
        mailparse::parse_mail(raw).map_err(|err| format!("invalid raw mime message: {err}"))?;

    let mut out = ParsedMimeMessage {
        subject: header_first_value(&parsed.headers, "Subject"),
        from: header_first_value(&parsed.headers, "From"),
        to: header_first_value(&parsed.headers, "To"),
        cc: header_first_value(&parsed.headers, "Cc"),
        bcc: header_first_value(&parsed.headers, "Bcc"),
        reply_to: header_first_value(&parsed.headers, "Reply-To"),
        date: header_first_value(&parsed.headers, "Date"),
        message_id: header_first_value(&parsed.headers, "Message-ID"),
        in_reply_to: header_first_value(&parsed.headers, "In-Reply-To"),
        references: header_first_value(&parsed.headers, "References"),
        ..Default::default()
    };

    out.auth_results_headers = headers_by_name(&parsed.headers, "Authentication-Results");
    out.auth_results_headers.extend(headers_by_name(
        &parsed.headers,
        "ARC-Authentication-Results",
    ));

    let mut body = BodyExtraction::default();
    walk_mime_parts(&parsed, "", &mut body);

    out.body_text = truncate_utf8_bytes(&body.body_text, MAX_BODY_TEXT_BYTES);
    out.body_html = truncate_utf8_bytes(&body.body_html, MAX_BODY_HTML_BYTES);
    out.remote_image_count = count_remote_image_refs(&out.body_html)?;
    out.snippet = if !out.body_text.trim().is_empty() {
        build_snippet(&out.body_text, DEFAULT_SNIPPET_LEN)
    } else {
        let text_from_html = strip_html_tags(&out.body_html)?;
        build_snippet(&text_from_html, DEFAULT_SNIPPET_LEN)
    };
    out.attachments = body.attachments;

    Ok(out)
}

fn walk_mime_parts(part: &ParsedMail<'_>, path: &str, out: &mut BodyExtraction) {
    if part.subparts.is_empty() {
        process_leaf_part(part, path, out);
        return;
    }

    for (i, child) in part.subparts.iter().enumerate() {
        let child_path = if path.is_empty() {
            format!("{}", i + 1)
        } else {
            format!("{}.{}", path, i + 1)
        };
        walk_mime_parts(child, &child_path, out);
    }
}

fn process_leaf_part(part: &ParsedMail<'_>, path: &str, out: &mut BodyExtraction) {
    let content_type = part.ctype.mimetype.to_lowercase();
    let disposition = part.get_content_disposition();
    let disposition_type = match &disposition.disposition {
        DispositionType::Inline => "inline",
        DispositionType::Attachment => "attachment",
        DispositionType::FormData => "form-data",
        DispositionType::Extension(v) => v.as_str(),
    };

    let filename = first_non_empty(vec![
        disposition
            .params
            .get("filename")
            .cloned()
            .unwrap_or_default(),
        part.ctype.params.get("name").cloned().unwrap_or_default(),
    ]);

    let named_inline = disposition_type == "inline" && !filename.is_empty();
    let explicit_attachment = disposition_type == "attachment";
    let attachment_like = explicit_attachment || named_inline;

    if !attachment_like && content_type.starts_with("text/plain") {
        if let Ok(body) = part.get_body() {
            append_text(&mut out.body_text, &body);
        }
        return;
    }

    if !attachment_like && content_type.starts_with("text/html") {
        if let Ok(body) = part.get_body() {
            append_text(&mut out.body_html, &body);
        }
        return;
    }

    if out.attachments.len() >= MAX_ATTACHMENTS {
        return;
    }

    let size_bytes = part
        .get_body_raw()
        .map(|bytes| bytes.len() as u64)
        .unwrap_or_default();

    let id = if path.trim().is_empty() {
        "1".to_string()
    } else {
        path.to_string()
    };

    out.attachments.push(AttachmentInfo {
        id,
        filename,
        content_type,
        size_bytes,
        inline: disposition_type == "inline",
    });
}

fn append_text(dst: &mut String, src: &str) {
    if src.trim().is_empty() {
        return;
    }
    if !dst.is_empty() {
        dst.push('\n');
    }
    dst.push_str(src);
}

fn build_snippet(raw: &str, max: usize) -> String {
    let compact = raw.split_whitespace().collect::<Vec<_>>().join(" ");
    if max == 0 || compact.len() <= max {
        return compact;
    }
    truncate_utf8_bytes(&compact, max)
}

fn truncate_utf8_bytes(raw: &str, max: usize) -> String {
    if raw.len() <= max {
        return raw.to_string();
    }
    let mut end = max;
    while end > 0 && !raw.is_char_boundary(end) {
        end -= 1;
    }
    raw[..end].to_string()
}

fn count_remote_image_refs(html: &str) -> Result<usize, String> {
    if html.trim().is_empty() {
        return Ok(0);
    }
    let re = Regex::new(r#"(?is)<img\b[^>]*\bsrc\s*=\s*(?:\\?["'])?(?:https?:)?//"#)
        .map_err(|err| format!("invalid remote image regex: {err}"))?;
    Ok(re.find_iter(html).count())
}

fn strip_html_tags(html: &str) -> Result<String, String> {
    if html.trim().is_empty() {
        return Ok(String::new());
    }
    let re =
        Regex::new(r#"(?is)<[^>]+>"#).map_err(|err| format!("invalid html-strip regex: {err}"))?;
    Ok(re.replace_all(html, " ").to_string())
}

fn header_first_value(headers: &[MailHeader<'_>], name: &str) -> String {
    for header in headers {
        if header.get_key_ref().eq_ignore_ascii_case(name) {
            return header.get_value().trim().to_string();
        }
    }
    String::new()
}

fn headers_by_name(headers: &[MailHeader<'_>], name: &str) -> Vec<String> {
    headers
        .iter()
        .filter(|header| header.get_key_ref().eq_ignore_ascii_case(name))
        .map(|header| header.get_value())
        .map(|v| v.trim().to_string())
        .filter(|v| !v.is_empty())
        .collect()
}

fn parse_auth_status(headers: &[String], key: &str) -> String {
    if headers.is_empty() {
        return "unknown".to_string();
    }
    let pattern = format!(r"(?i)\b{}\s*=\s*([a-z0-9_-]+)", regex::escape(key));
    let re = match Regex::new(&pattern) {
        Ok(v) => v,
        Err(_) => return "unknown".to_string(),
    };
    for header in headers {
        if let Some(caps) = re.captures(header) {
            if let Some(value) = caps.get(1) {
                return normalize_auth_status(value.as_str());
            }
        }
    }
    "unknown".to_string()
}

fn normalize_auth_status(raw: &str) -> String {
    let value = raw.trim().to_lowercase();
    match value.as_str() {
        "pass" | "fail" | "softfail" | "neutral" | "none" | "temperror" | "permerror"
        | "policy" => value,
        _ => "unknown".to_string(),
    }
}

fn is_auth_failed(status: &str) -> bool {
    matches!(
        status,
        "fail" | "softfail" | "temperror" | "permerror" | "policy"
    )
}

fn extract_email_domain(raw: &str) -> String {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return String::new();
    }

    let candidate = if let Some(start) = trimmed.find('<') {
        if let Some(rel_end) = trimmed[start + 1..].find('>') {
            trimmed[start + 1..start + 1 + rel_end].trim()
        } else {
            trimmed
        }
    } else {
        trimmed
    };

    let at = match candidate.rfind('@') {
        Some(idx) => idx,
        None => return String::new(),
    };

    let mut domain = candidate[at + 1..]
        .trim()
        .trim_matches(|ch: char| {
            !(ch.is_ascii_alphanumeric() || ch == '.' || ch == '-' || ch == '_')
        })
        .to_lowercase();

    while domain.ends_with('.') {
        let _ = domain.pop();
    }
    domain
}

fn extract_urls(text: &str) -> Result<Vec<String>, String> {
    if text.trim().is_empty() {
        return Ok(Vec::new());
    }
    let re = Regex::new(r#"(?i)https?://[^\s<>'\"]+"#)
        .map_err(|err| format!("invalid url regex: {err}"))?;
    Ok(re
        .find_iter(text)
        .map(|m| m.as_str().to_string())
        .collect::<Vec<_>>())
}

fn has_executable_attachment(name: &str) -> bool {
    let lower = name.trim().to_lowercase();
    [
        ".exe", ".dll", ".scr", ".js", ".vbs", ".bat", ".cmd", ".jar", ".com", ".msi",
    ]
    .iter()
    .any(|ext| lower.ends_with(ext))
}

fn first_non_empty(values: Vec<String>) -> String {
    for value in values {
        let trimmed = value.trim();
        if !trimmed.is_empty() {
            return trimmed.to_string();
        }
    }
    String::new()
}

fn verify_webauthn_registration(payload: JsonValue) -> Result<JsonValue, String> {
    let req: WebAuthnRegistrationFinishPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;
    let client_data_json =
        decode_b64url_field(&req.client_data_json_b64url, "client_data_json_b64url")?;
    let client_data: ClientData = serde_json::from_slice(&client_data_json)
        .map_err(|err| format!("invalid clientDataJSON: {err}"))?;

    if client_data.typ != "webauthn.create" {
        return Err("clientData.type must be webauthn.create".to_string());
    }
    if client_data.cross_origin {
        return Err("cross-origin WebAuthn responses are not allowed".to_string());
    }
    verify_challenge(&client_data.challenge, &req.challenge)?;
    verify_origin(&client_data.origin, &req.origins, &req.origin)?;

    let attestation_object =
        decode_b64url_field(&req.attestation_object_b64url, "attestation_object_b64url")?;
    let (attestation_fmt, auth_data_raw) = parse_attestation_object(&attestation_object)?;
    if attestation_fmt != "none" {
        return Err(format!(
            "unsupported attestation format: {attestation_fmt}; use attestation=\"none\""
        ));
    }
    let auth_data = parse_authenticator_data(&auth_data_raw, true)?;
    verify_rp_id_hash(&auth_data.rp_id_hash, &req.rp_id)?;
    if auth_data.flags & FLAG_USER_PRESENT == 0 {
        return Err("user presence flag is missing".to_string());
    }
    if req.require_user_verification && auth_data.flags & FLAG_USER_VERIFIED == 0 {
        return Err("user verification is required".to_string());
    }

    let credential_id = auth_data
        .credential_id
        .ok_or_else(|| "credential_id is missing in attested credential data".to_string())?;
    let credential_id_b64 = URL_SAFE_NO_PAD.encode(&credential_id);
    if !req.credential_id.trim().is_empty() {
        verify_credential_id_match(req.credential_id.trim(), &credential_id)?;
    }

    let credential_public_key = auth_data.credential_public_key.ok_or_else(|| {
        "credential_public_key is missing in attested credential data".to_string()
    })?;
    ensure_cose_key_supported(&credential_public_key)?;

    Ok(json!({
        "credential_id": credential_id_b64,
        "public_key_cose_b64url": URL_SAFE_NO_PAD.encode(&credential_public_key),
        "sign_count": auth_data.sign_count as u64,
        "flags": {
            "user_present": auth_data.flags & FLAG_USER_PRESENT != 0,
            "user_verified": auth_data.flags & FLAG_USER_VERIFIED != 0
        }
    }))
}

fn verify_webauthn_assertion(payload: JsonValue) -> Result<JsonValue, String> {
    let req: WebAuthnAssertionFinishPayload =
        serde_json::from_value(payload).map_err(|err| format!("invalid payload: {err}"))?;
    let client_data_json =
        decode_b64url_field(&req.client_data_json_b64url, "client_data_json_b64url")?;
    let client_data: ClientData = serde_json::from_slice(&client_data_json)
        .map_err(|err| format!("invalid clientDataJSON: {err}"))?;

    if client_data.typ != "webauthn.get" {
        return Err("clientData.type must be webauthn.get".to_string());
    }
    if client_data.cross_origin {
        return Err("cross-origin WebAuthn responses are not allowed".to_string());
    }
    verify_challenge(&client_data.challenge, &req.challenge)?;
    verify_origin(&client_data.origin, &req.origins, &req.origin)?;

    let authenticator_data =
        decode_b64url_field(&req.authenticator_data_b64url, "authenticator_data_b64url")?;
    let auth_data = parse_authenticator_data(&authenticator_data, false)?;
    verify_rp_id_hash(&auth_data.rp_id_hash, &req.rp_id)?;
    if auth_data.flags & FLAG_USER_PRESENT == 0 {
        return Err("user presence flag is missing".to_string());
    }
    if req.require_user_verification && auth_data.flags & FLAG_USER_VERIFIED == 0 {
        return Err("user verification is required".to_string());
    }

    if req.credential_id.trim().is_empty() {
        return Err("credential_id is required".to_string());
    }
    let signature = decode_b64url_field(&req.signature_b64url, "signature_b64url")?;
    let stored_public_key = decode_b64url_field(
        &req.stored_public_key_cose_b64url,
        "stored_public_key_cose_b64url",
    )?;

    let client_data_hash = Sha256::digest(&client_data_json);
    let mut signed_data = Vec::with_capacity(authenticator_data.len() + client_data_hash.len());
    signed_data.extend_from_slice(&authenticator_data);
    signed_data.extend_from_slice(&client_data_hash);
    verify_assertion_signature(&stored_public_key, &signed_data, &signature)?;

    let sign_count = auth_data.sign_count as u64;
    if req.stored_sign_count > 0 && sign_count > 0 && sign_count <= req.stored_sign_count {
        return Err("authenticator sign count replay detected".to_string());
    }

    Ok(json!({
        "credential_id": req.credential_id,
        "sign_count": sign_count,
        "flags": {
            "user_present": auth_data.flags & FLAG_USER_PRESENT != 0,
            "user_verified": auth_data.flags & FLAG_USER_VERIFIED != 0
        }
    }))
}

fn parse_attestation_object(raw: &[u8]) -> Result<(String, Vec<u8>), String> {
    let value: CborValue = ciborium::de::from_reader(Cursor::new(raw))
        .map_err(|err| format!("invalid attestationObject: {err}"))?;
    let map = as_cbor_map(&value)?;

    let fmt_value =
        map_get_by_text(map, "fmt").ok_or_else(|| "attestation fmt is missing".to_string())?;
    let fmt = as_text(fmt_value, "attestation fmt")?.to_string();
    let auth_data_value = map_get_by_text(map, "authData")
        .ok_or_else(|| "attestation authData is missing".to_string())?;
    let auth_data = as_bytes(auth_data_value, "attestation authData")?.to_vec();

    Ok((fmt, auth_data))
}

fn parse_authenticator_data(
    raw: &[u8],
    require_attested_data: bool,
) -> Result<ParsedAuthenticatorData, String> {
    if raw.len() < 37 {
        return Err("authenticatorData is too short".to_string());
    }
    let mut rp_id_hash = [0u8; 32];
    rp_id_hash.copy_from_slice(&raw[0..32]);
    let flags = raw[32];
    let sign_count = u32::from_be_bytes([raw[33], raw[34], raw[35], raw[36]]);

    let mut credential_id = None;
    let mut credential_public_key = None;
    if flags & FLAG_ATTESTED_CREDENTIAL_DATA != 0 {
        if raw.len() < 55 {
            return Err("authenticatorData missing attested credential data".to_string());
        }
        let mut offset = 37usize;
        offset += 16; // AAGUID
        let cred_len = u16::from_be_bytes([raw[offset], raw[offset + 1]]) as usize;
        offset += 2;
        if raw.len() < offset + cred_len {
            return Err("credential_id length exceeds authenticatorData".to_string());
        }
        credential_id = Some(raw[offset..offset + cred_len].to_vec());
        offset += cred_len;
        if raw.len() <= offset {
            return Err("credential_public_key is missing".to_string());
        }
        let mut cbor_cursor = Cursor::new(&raw[offset..]);
        let _: CborValue = ciborium::de::from_reader(&mut cbor_cursor)
            .map_err(|err| format!("invalid credential_public_key cbor: {err}"))?;
        let consumed = cbor_cursor.position() as usize;
        if consumed == 0 {
            return Err("credential_public_key is empty".to_string());
        }
        credential_public_key = Some(raw[offset..offset + consumed].to_vec());
    }

    if require_attested_data && (credential_id.is_none() || credential_public_key.is_none()) {
        return Err("attested credential data is required".to_string());
    }

    Ok(ParsedAuthenticatorData {
        rp_id_hash,
        flags,
        sign_count,
        credential_id,
        credential_public_key,
    })
}

fn verify_rp_id_hash(rp_id_hash: &[u8; 32], rp_id: &str) -> Result<(), String> {
    let rp_id = rp_id.trim();
    if rp_id.is_empty() {
        return Err("rp_id is required".to_string());
    }
    let expected = Sha256::digest(rp_id.as_bytes());
    if rp_id_hash.ct_eq(expected.as_slice()).unwrap_u8() != 1 {
        return Err("rp_id_hash mismatch".to_string());
    }
    Ok(())
}

fn verify_challenge(client_challenge: &str, expected_challenge: &str) -> Result<(), String> {
    let client = decode_or_bytes(client_challenge.trim());
    let expected = decode_or_bytes(expected_challenge.trim());
    if client.len() != expected.len() || client.ct_eq(&expected).unwrap_u8() != 1 {
        return Err("challenge mismatch".to_string());
    }
    Ok(())
}

fn verify_origin(
    origin: &str,
    allowed_origins: &[String],
    fallback_origin: &str,
) -> Result<(), String> {
    let origin = origin.trim();
    if origin.is_empty() {
        return Err("origin is required".to_string());
    }
    let mut allowed = allowed_origins
        .iter()
        .map(|v| v.trim().to_string())
        .filter(|v| !v.is_empty())
        .collect::<Vec<_>>();
    if allowed.is_empty() && !fallback_origin.trim().is_empty() {
        allowed.push(fallback_origin.trim().to_string());
    }
    if allowed.is_empty() {
        return Err("origin allowlist is empty".to_string());
    }
    if allowed.iter().any(|candidate| candidate == origin) {
        return Ok(());
    }
    Err(format!("origin not allowed: {origin}"))
}

fn verify_credential_id_match(claimed_id: &str, canonical_id_bytes: &[u8]) -> Result<(), String> {
    let claimed = decode_or_bytes(claimed_id.trim());
    if claimed.len() != canonical_id_bytes.len()
        || claimed.ct_eq(canonical_id_bytes).unwrap_u8() != 1
    {
        return Err("credential_id mismatch".to_string());
    }
    Ok(())
}

fn decode_b64url_field(raw: &str, field_name: &str) -> Result<Vec<u8>, String> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return Err(format!("{field_name} is required"));
    }
    URL_SAFE_NO_PAD
        .decode(trimmed)
        .or_else(|_| URL_SAFE.decode(trimmed))
        .map_err(|err| format!("invalid {field_name}: {err}"))
}

fn decode_or_bytes(raw: &str) -> Vec<u8> {
    URL_SAFE_NO_PAD
        .decode(raw)
        .or_else(|_| URL_SAFE.decode(raw))
        .unwrap_or_else(|_| raw.as_bytes().to_vec())
}

fn verify_assertion_signature(
    cose_key_raw: &[u8],
    signed_data: &[u8],
    signature_raw: &[u8],
) -> Result<(), String> {
    let cose_value: CborValue = ciborium::de::from_reader(Cursor::new(cose_key_raw))
        .map_err(|err| format!("invalid stored cose key: {err}"))?;
    let cose_map = as_cbor_map(&cose_value)?;
    let kty = map_get_i64(cose_map, 1, "cose kty")?;
    let alg = map_get_i64(cose_map, 3, "cose alg")?;

    match (kty, alg) {
        (COSE_KTY_EC2, COSE_ALG_ES256) => {
            verify_es256_assertion(cose_map, signed_data, signature_raw)
        }
        (COSE_KTY_RSA, COSE_ALG_RS256) => {
            verify_rs256_assertion(cose_map, signed_data, signature_raw)
        }
        _ => Err(format!(
            "unsupported cose key type/algorithm pair: kty={kty} alg={alg}"
        )),
    }
}

fn ensure_cose_key_supported(cose_key_raw: &[u8]) -> Result<(), String> {
    let cose_value: CborValue = ciborium::de::from_reader(Cursor::new(cose_key_raw))
        .map_err(|err| format!("invalid credential_public_key cose: {err}"))?;
    let cose_map = as_cbor_map(&cose_value)?;
    let kty = map_get_i64(cose_map, 1, "cose kty")?;
    let alg = map_get_i64(cose_map, 3, "cose alg")?;
    match (kty, alg) {
        (COSE_KTY_EC2, COSE_ALG_ES256) | (COSE_KTY_RSA, COSE_ALG_RS256) => Ok(()),
        _ => Err(format!(
            "unsupported cose key type/algorithm pair: kty={kty} alg={alg}"
        )),
    }
}

fn verify_es256_assertion(
    cose_map: &[(CborValue, CborValue)],
    signed_data: &[u8],
    signature_raw: &[u8],
) -> Result<(), String> {
    let x = map_get_bytes(cose_map, -2, "cose x")?;
    let y = map_get_bytes(cose_map, -3, "cose y")?;
    if x.len() != 32 || y.len() != 32 {
        return Err("invalid ES256 key size".to_string());
    }
    let mut sec1 = Vec::with_capacity(65);
    sec1.push(0x04);
    sec1.extend_from_slice(x);
    sec1.extend_from_slice(y);

    let verifying_key = EcdsaVerifyingKey::from_sec1_bytes(&sec1)
        .map_err(|err| format!("invalid ES256 key: {err}"))?;
    let signature = EcdsaSignature::from_der(signature_raw)
        .map_err(|err| format!("invalid ECDSA signature: {err}"))?;
    verifying_key
        .verify(signed_data, &signature)
        .map_err(|_| "invalid assertion signature".to_string())
}

fn verify_rs256_assertion(
    cose_map: &[(CborValue, CborValue)],
    signed_data: &[u8],
    signature_raw: &[u8],
) -> Result<(), String> {
    let modulus = map_get_bytes(cose_map, -1, "cose modulus")?;
    let exponent = map_get_bytes(cose_map, -2, "cose exponent")?;
    let public_key = RsaPublicKey::new(
        BigUint::from_bytes_be(modulus),
        BigUint::from_bytes_be(exponent),
    )
    .map_err(|err| format!("invalid RS256 key: {err}"))?;
    let verifying_key = RsaVerifyingKey::<Sha256>::new(public_key);
    let signature = RsaSignature::try_from(signature_raw)
        .map_err(|err| format!("invalid RSA signature: {err}"))?;
    verifying_key
        .verify(signed_data, &signature)
        .map_err(|_| "invalid assertion signature".to_string())
}

fn as_cbor_map(value: &CborValue) -> Result<&Vec<(CborValue, CborValue)>, String> {
    match value {
        CborValue::Map(map) => Ok(map),
        _ => Err("expected cbor map".to_string()),
    }
}

fn as_text<'a>(value: &'a CborValue, name: &str) -> Result<&'a str, String> {
    match value {
        CborValue::Text(v) => Ok(v.as_str()),
        _ => Err(format!("{name} must be cbor text")),
    }
}

fn as_bytes<'a>(value: &'a CborValue, name: &str) -> Result<&'a [u8], String> {
    match value {
        CborValue::Bytes(v) => Ok(v.as_slice()),
        _ => Err(format!("{name} must be cbor bytes")),
    }
}

fn map_get_by_text<'a>(map: &'a [(CborValue, CborValue)], key: &str) -> Option<&'a CborValue> {
    map.iter().find_map(|(k, v)| match k {
        CborValue::Text(name) if name == key => Some(v),
        _ => None,
    })
}

fn map_get_by_i64<'a>(map: &'a [(CborValue, CborValue)], key: i64) -> Option<&'a CborValue> {
    map.iter().find_map(|(k, v)| match k {
        CborValue::Integer(i) if i128::from(*i) == key as i128 => Some(v),
        _ => None,
    })
}

fn map_get_i64(map: &[(CborValue, CborValue)], key: i64, name: &str) -> Result<i64, String> {
    let value = map_get_by_i64(map, key).ok_or_else(|| format!("{name} is missing"))?;
    match value {
        CborValue::Integer(i) => {
            i64::try_from(i128::from(*i)).map_err(|err| format!("{name} is not an integer: {err}"))
        }
        _ => Err(format!("{name} must be integer")),
    }
}

fn map_get_bytes<'a>(
    map: &'a [(CborValue, CborValue)],
    key: i64,
    name: &str,
) -> Result<&'a [u8], String> {
    let value = map_get_by_i64(map, key).ok_or_else(|| format!("{name} is missing"))?;
    as_bytes(value, name)
}

#[cfg(test)]
mod tests {
    use super::*;
    use p256::ecdsa::signature::Signer;
    use p256::ecdsa::SigningKey;
    use rand_core::OsRng;

    #[test]
    fn verify_challenge_accepts_equivalent_base64() {
        let challenge = URL_SAFE_NO_PAD.encode(b"hello-world");
        let padded = URL_SAFE.encode(b"hello-world");
        verify_challenge(&challenge, &padded).expect("challenge should match");
    }

    #[test]
    fn verify_es256_signature_from_cose_key() {
        let signing_key = SigningKey::random(&mut OsRng);
        let verify_key = signing_key.verifying_key();
        let encoded = verify_key.to_encoded_point(false);
        let x = encoded.x().expect("x coordinate").to_vec();
        let y = encoded.y().expect("y coordinate").to_vec();

        let cose = CborValue::Map(vec![
            (
                CborValue::Integer(1_i64.into()),
                CborValue::Integer(2_i64.into()),
            ),
            (
                CborValue::Integer(3_i64.into()),
                CborValue::Integer((-7_i64).into()),
            ),
            (
                CborValue::Integer((-1_i64).into()),
                CborValue::Integer(1_i64.into()),
            ),
            (CborValue::Integer((-2_i64).into()), CborValue::Bytes(x)),
            (CborValue::Integer((-3_i64).into()), CborValue::Bytes(y)),
        ]);
        let mut cose_bytes = Vec::new();
        ciborium::ser::into_writer(&cose, &mut cose_bytes).expect("encode cose key");

        let signed_data = b"assertion-bytes";
        let signature: EcdsaSignature = signing_key.sign(signed_data);
        let signature_der = signature.to_der();
        verify_assertion_signature(&cose_bytes, signed_data, signature_der.as_bytes())
            .expect("signature should verify");
    }

    #[test]
    fn parse_authenticator_data_rejects_short_payload() {
        assert!(parse_authenticator_data(b"short", false).is_err());
    }

    #[test]
    fn sanitize_html_blocks_remote_images() {
        let payload = json!({
            "html": r#"<div>ok</div><img src="https://evil.example/pixel.png"><script>alert(1)</script>"#,
            "allow_remote_images": false,
        });
        let out = sanitize_html_operation(payload).expect("sanitize html");
        assert!(out["remote_images_blocked"].as_bool().unwrap_or(false));
        assert_eq!(out["blocked_remote_image_count"].as_u64().unwrap_or(0), 1);
        let html = out["html"].as_str().unwrap_or("");
        assert!(!html.contains("<script"));
        assert!(!html.contains("pixel.png"));
    }

    #[test]
    fn parse_auth_status_from_header() {
        let status = parse_auth_status(
            &["mx; dkim=pass header.d=example.com; spf=fail smtp.mailfrom=bad".to_string()],
            "dkim",
        );
        assert_eq!(status, "pass");
        let spf = parse_auth_status(
            &["mx; dkim=pass header.d=example.com; spf=fail smtp.mailfrom=bad".to_string()],
            "spf",
        );
        assert_eq!(spf, "fail");
    }

    #[test]
    fn verify_totp_rfc6238_vector_sha1() {
        let payload = json!({
            "secret": "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
            "code": "94287082",
            "time_unix": 59,
            "period": 30,
            "digits": 8,
            "algorithm": "SHA1",
            "skew_past": 0,
            "skew_future": 0,
        });
        let out = verify_totp_operation(payload).expect("totp verify");
        assert!(out["valid"].as_bool().unwrap_or(false));
    }

    #[test]
    fn client_data_allows_unknown_fields() {
        let parsed: ClientData = serde_json::from_str(
            r#"{
            "type":"webauthn.create",
            "challenge":"abc",
            "origin":"https://example.test",
            "crossOrigin":false,
            "other_keys_can_be_added_here":"yes"
        }"#,
        )
        .expect("clientDataJSON should accept unknown keys");
        assert_eq!(parsed.typ, "webauthn.create");
        assert_eq!(parsed.challenge, "abc");
    }

    #[test]
    fn client_data_requires_challenge_field() {
        let err = serde_json::from_str::<ClientData>(
            r#"{
            "type":"webauthn.get",
            "origin":"https://example.test",
            "crossOrigin":false
        }"#,
        )
        .expect_err("challenge is required");
        assert!(
            err.to_string().contains("challenge"),
            "expected challenge-related decode error, got {err}"
        );
    }
}
