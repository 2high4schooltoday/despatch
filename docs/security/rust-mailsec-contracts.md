# Rust Mail Security Service Contract Freeze (v1)

This document freezes Go-to-Rust IPC contracts for `mailsec_service`.

## 1. Transport

- UNIX domain socket only
- Frame format:
  - `4-byte` big-endian payload length
  - JSON payload body
- Maximum frame size: `8 MiB`
- Unknown JSON fields are rejected (`deny_unknown_fields`)
- Exception: WebAuthn `clientDataJSON` parsing accepts unknown keys from authenticators while still requiring `type`, `challenge`, and `origin`.

## 2. Request Envelope

Required fields:
- `request_id: string`
- `op: string`
- `account_id: string`
- `message_id: string`
- `payload: object`
- `deadline_ms: integer`

## 3. Response Envelope

Required fields:
- `request_id: string`
- `ok: bool`
- `code: string`
- `error: string`
- `result: object`

## 4. Well-Known Operation Codes

Current operation names:
- `mime.parse`
- `mime.extract_attachments`
- `html.sanitize`
- `auth.verify`
- `totp.verify`
- `webauthn.register.finish`
- `webauthn.assertion.finish`
- `crypto.pgp.sign`
- `crypto.pgp.encrypt`
- `crypto.pgp.decrypt`
- `crypto.pgp.verify`
- `crypto.smime.sign`
- `crypto.smime.encrypt`
- `crypto.smime.decrypt`
- `crypto.smime.verify`

Implemented result payload contracts:
- `mime.parse`: returns normalized header/body fields plus `attachments[]`.
- `mime.extract_attachments`: returns `attachments[]` metadata only.
- `html.sanitize`: returns sanitized `html` and remote-image blocking metadata.
- `auth.verify`: returns `dkim`, `spf`, `dmarc`, `phishing_score`, and `indicators[]`.
- `totp.verify`: returns `valid`, `matched_counter`, and applied TOTP parameters.
- `webauthn.register.finish`: returns `credential_id`, `public_key_cose_b64url`, `sign_count`.
- `webauthn.assertion.finish`: returns `credential_id`, `sign_count`.
- `crypto.pgp.sign`:
  - request payload: `plaintext` or `plaintext_b64url`, `private_key_armored`, optional `passphrase`
  - result payload: `signed_message_armored`, `signed_message_b64url`, `signer_fingerprint`
- `crypto.pgp.encrypt`:
  - request payload: `plaintext` or `plaintext_b64url`, `recipient_public_keys[]`
  - result payload: `ciphertext_armored`, `ciphertext_b64url`, `recipient_count`
- `crypto.pgp.decrypt`:
  - request payload: `ciphertext_armored` or `ciphertext_b64url`, `private_key_armored`, optional `passphrase`
  - result payload: `plaintext_b64url`, `plaintext_utf8`, `recipient_fingerprint`
- `crypto.pgp.verify`:
  - request payload: `signed_message_armored` or `signed_message_b64url`, `public_key_armored`
  - result payload: `valid`, `plaintext_b64url`, `plaintext_utf8`, `signer_fingerprint`
- `crypto.smime.sign`:
  - request payload: `plaintext` or `plaintext_b64url`, `private_key_pem`, `cert_pem`, optional `private_key_passphrase`
  - result payload: `signed_smime`, `signer_fingerprint_sha256`
- `crypto.smime.encrypt`:
  - request payload: `plaintext` or `plaintext_b64url`, `recipient_certs_pem[]`
  - result payload: `ciphertext_smime`, `recipient_count`
- `crypto.smime.decrypt`:
  - request payload: `ciphertext_smime` or `ciphertext_b64url`, `private_key_pem`, `cert_pem`, optional `private_key_passphrase`
  - result payload: `plaintext_b64url`, `plaintext_utf8`
- `crypto.smime.verify`:
  - request payload: `signed_smime` or `signed_b64url`, `trusted_certs_pem[]`
  - result payload: `valid`, `plaintext_b64url`, `plaintext_utf8`, `signer_subjects[]`

Current response code values:
- `ok`
- `invalid_request`
- `invalid_frame`
- `unsupported_operation`
- `timeout`
- `internal_error`

## 5. Compatibility Policy

- Contract tests in `rust/contracts` must pass before release packaging.
- Any schema change requires:
  - new version marker in this document,
  - fixture updates,
  - and synchronized Go/Rust parser updates.
