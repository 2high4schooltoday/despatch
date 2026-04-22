<p align="center"><strong>Despatch</strong></p>

<p align="center">
  <a href="https://github.com/2high4schooltoday/despatch/actions/workflows/release.yml"><img src="https://github.com/2high4schooltoday/despatch/actions/workflows/release.yml/badge.svg" alt="Release"></a>
  <a href="https://github.com/2high4schooltoday/despatch/actions/workflows/launchpad-installer.yml"><img src="https://github.com/2high4schooltoday/despatch/actions/workflows/launchpad-installer.yml/badge.svg" alt="Launchpad Installer"></a>
  <a href="https://github.com/2high4schooltoday/despatch/releases"><img src="https://img.shields.io/github/v/release/2high4schooltoday/despatch?display_name=tag" alt="Latest release"></a>
</p>

Despatch is an open source control plane for existing Postfix + Dovecot + Maildir installations.

Sounds like another mail panel. Let's try again.

**Despatch will not replace your mail stack, but it will make running it feel a lot less medieval.**

Install one verified Launchpad binary, answer a guided setup flow, and get:
- webmail backed by real IMAP and SMTP
- approvals, registration, audit logs, and password resets
- diagnostics for app health, reverse proxying, firewall state, and DNS drift
- native Launchpad install, diagnose, status, and uninstall operations

<details>
<summary>Table of Contents</summary>

- [Getting Started](#getting-started)
- [Implemented](#implemented)
- [ARM optimization defaults](#arm-optimization-defaults)
- [UI release gates](#ui-release-gates)
- [Web UI model](#web-ui-model)
- [Installer details](#installer-details)
- [Uninstall](#uninstall-safe-interactive)
- [Internet access diagnostics](#internet-access-diagnostics)
- [Web panel software updates](#web-panel-software-updates-ubuntu)
- [CAPTCHA with CAP standalone](#captcha-with-cap-standalone-ubuntu)
- [SMTP sender diagnostics](#smtp-sender-diagnostics-ubuntu)
- [Dovecot provisioning notes](#dovecot-provisioning-notes)
- [API](#api)
- [Test](#test)
- [Security behavior updates](#security-behavior-updates)
- [Mail Security Service](#mail-security-service)

</details>

## Getting Started
Validated for Ubuntu Server on:
- ARM64 (`aarch64` / `arm64`)
- x86-64 (`x86_64` / `amd64`)

### What you need
- an existing Postfix + Dovecot + Maildir host
- `sudo` access on the target Linux machine
- outbound access to GitHub Releases for the first install

### Basic installation
The universal installer is the primary installation source right now. Start there unless you explicitly want the smaller architecture-specific binaries.

With `curl`:

```sh
INSTALLER_URL="$(curl -fsSL https://api.github.com/repos/2high4schooltoday/despatch/releases \
  | sed -n 's/.*"browser_download_url": "\(https:\/\/github.com\/2high4schooltoday\/despatch\/releases\/download\/[^\"]*\/despatch-installer-installer-linux-universal\)".*/\1/p' \
  | head -n 1)"
curl -fL -o despatch-installer "$INSTALLER_URL"
chmod +x despatch-installer
./despatch-installer
```

With `wget`:

```sh
INSTALLER_URL="$(wget -qO- https://api.github.com/repos/2high4schooltoday/despatch/releases \
  | sed -n 's/.*"browser_download_url": "\(https:\/\/github.com\/2high4schooltoday\/despatch\/releases\/download\/[^\"]*\/despatch-installer-installer-linux-universal\)".*/\1/p' \
  | head -n 1)"
wget -O despatch-installer "$INSTALLER_URL"
chmod +x despatch-installer
./despatch-installer
```

This uses the GitHub Releases API because `releases/latest` does not reliably point at alpha installer assets.

After the installer finishes:
1. Open the reported URL.
2. Use `https://<your-domain>` in proxy mode or `http://<server-ip>:8080` in direct mode.
3. Complete the web OOBE.
4. Choose region, set domain, and create the first admin account.

Architecture-specific installers remain available if you know the target ahead of time:
- `despatch-installer-installer-linux-x86_64`
- `despatch-installer-installer-linux-arm64`

### Manual inspection and provenance
Despatch publishes verified Launchpad installers plus signed application archives for every release.

If you already have Launchpad locally, verify the downloaded installer before running it:

```sh
launchpad installer verify ./despatch-installer
```

Each release also publishes:
- `despatch-installer-installer-manifest.json`
- `despatch-installer-installer-sha256sums.txt`
- `checksums.txt`
- `checksums.txt.sig`

The installer assets carry GitHub attestation-backed Launchpad proof, and the application archives consumed by the installer are additionally signed through `checksums.txt.sig`.

### From a local checkout
Local entrypoints still work and now route into the native Launchpad project:
- `./despatch`
- `./scripts/tui.sh`
- `./scripts/tui_plain.sh`

Compatibility wrappers remain available:
- `./scripts/auto_install.sh`
- `./scripts/uninstall.sh`
- `./scripts/diagnose_access.sh`
- `wget -O despatch.py https://raw.githubusercontent.com/2high4schooltoday/despatch/main/scripts/despatch.py && chmod +x despatch.py && ./despatch.py`

## Implemented
- Single-binary Go server with REST API + web UI.
- Registration with manual admin approval workflow.
- Unified auth model with Argon2id password hashing.
- Session cookies + CSRF protection for mutating routes.
- Request-aware cookie security policy (`COOKIE_SECURE_MODE=auto|always|never`) for direct/proxy deployment compatibility.
- Startup safety checks for session key strength, cookie security, and CAPTCHA config.
- Admin user lifecycle controls: approve/reject/suspend/unsuspend/reset password.
- Rejected registration policy: rejected pending users are removed from active `users` records (legacy cleanup migration included).
- Password reset token flow.
- Audit log endpoint + admin UI list/detail drill-down.
- Real IMAP integration (list/search/read/flags/move).
- Real SMTP submission with optional STARTTLS/TLS and attachment send.
- Dovecot auth SQL provisioning adapter (MySQL/Postgres via `database/sql`).
- Dovecot auth backend modes:
  - `sql` mode (provisioning via Dovecot SQL table)
  - `pam` mode (no SQL provisioning; login validated through IMAP/PAM credentials)
- SQLite app DB with WAL mode and conservative pool sizing for low-resource ARM hosts.
- Nginx/Apache2 and systemd deployment templates.

## ARM optimization defaults
- SQLite pool defaults: `max_open=4`, `max_idle=2`.
- HTTP timeouts configured with low overhead defaults.
- API pagination hard-capped (`page_size <= 100`).
- In-memory limiter bucket cleanup to avoid unbounded growth.
- Persistent failed-login counters in SQLite `rate_limit_events`.
- No heavy background workers.

## UI release gates
Run these before packaging a release:
- `go test ./...`
- `cargo test --manifest-path rust/Cargo.toml --workspace`
- `cargo audit --manifest-path rust/Cargo.toml`
- `cargo deny check --manifest-path rust/Cargo.toml --config rust/deny.toml advisories bans sources`
- `./scripts/check_warm_palette.sh`
- `./scripts/check_ui_contract.sh`
- `./scripts/check_ui_selector_contract.sh`
- `./scripts/check_surface_depth.sh`

## Web UI model
- Navigation state is context-aware:
  - Pre-auth: `Auth` only.
  - Authenticated: `Mail`, `Settings`, and `Admin` (admin role only).
  - Setup-required: `Setup` only.
- Setup Assistant (OOBE) follows a linear, single-decision-per-screen template:
  - No inner title bar/chrome.
  - Back and discard controls are inside the setup window.
  - Primary action stays anchored at the bottom-right.
  - Global topbar is hidden while setup is required.
- Selector-contract CI gate protects critical Setup/Auth/Settings/Admin selectors from shipping out-of-sync with markup.
- Auth uses the same calm OOBE-style template with task selectors (not tabs):
  - `Sign In`
  - `Register` (with CAPTCHA when enabled, recovery email required)
  - `Password Reset`
- Settings uses macOS-style domain navigation with shallow depth (sidebar + vertical sections):
  - `Sign-In`: passkeys and passkey detail pages
  - `Devices`: trusted devices and device detail pages
  - `Sessions`: active sessions and session detail pages
- Admin keeps a separate domain sidebar:
  - `Users`, `Registrations`, `System`, `Audit Log`
  - `System` includes software updates and feature flag controls
- Passkey sign-in UX:
  - single passkey action button
  - built-in username-less account discovery
  - no email prompt before passkey sign-in
- Mail and Compose are unified:
  - mailbox + message list + viewer stay in one Mail workspace
  - compose opens as an elevated modal panel from `New Message`
  - compose uses a layered toolbar + metadata + canvas layout with rich-text editing controls
  - compose supports `To/Cc/Bcc`, inline image embedding, regular attachments, and sender identity selection
  - compose falls back to manual sender confirmation when account identities are unavailable
  - compose draft is persisted locally until successful send

## Installer details
Interactive, prompt-driven install flows are available from:
- the verified universal standalone installer
- `./despatch`
- `./scripts/tui.sh`

Keyboard controls for the TUI:
- `Tab` cycles controls
- arrow keys move within cards, fields, and actions
- `Enter` activates the current control
- `Esc` goes back
- `L` toggles logs during progress or completion
- `Ctrl+X` cancels the active run

Plain console-only menu:
- `./scripts/tui_plain.sh`
- this is only a shell menu wrapper around the same native Launchpad operations

What the installer auto-detects:
- Dovecot SQL config (`dovecot-sql.conf.ext`)
- Dovecot auth mode hints (`pam` vs `sql`)
- SQL driver / connect hints / auth table + columns
- localhost IMAP/SMTP ports and TLS mode hints
- sensible default domain from host (`/etc/mailname` or FQDN)
- installed reverse proxy (`nginx` or `apache2`) and can configure it automatically

TLS default posture:
- Installer keeps IMAP/SMTP certificate verification enabled by default.
- For loopback IMAP/SMTP with TLS/STARTTLS, installer auto-sets `*_INSECURE_SKIP_VERIFY=true` to avoid local certificate SAN mismatches.
- Prefer certificate SAN/FQDN alignment over `*_INSECURE_SKIP_VERIFY=true`.
- Non-loopback `*_INSECURE_SKIP_VERIFY=true` is rejected unless `MAIL_ALLOW_INSECURE_SKIP_VERIFY_NON_LOOPBACK=true` is explicitly set.

Installer deployment modes:
- `proxy` mode (recommended): app binds to `127.0.0.1:8080`, reverse proxy serves public traffic on `80/443`.
- `direct` mode: app serves directly on `:8080`.
- Final install message is mode-aware and shows the correct URL for your chosen mode.

What is intentionally deferred to web UI OOBE:
- Admin email/password creation
- Final first-run setup confirmation

Note: fully custom SQL/auth setups may still need manual `.env` tweaks.

PAM mode notes:
- Set `DOVECOT_AUTH_MODE=pam` (installer now prompts and writes this automatically).
- In PAM mode, web login is validated against IMAP credentials (Dovecot/PAM), not local hash only.
- Password reset endpoints use the local privileged helper (`despatch-pam-reset-helper`) when `PAM_RESET_HELPER_ENABLED=true`.
- Public reset request endpoint remains account-enumeration safe (generic acceptance response when enabled).
- Public reset request accepts either account email or recovery email as identifier.
- Recovery email must be different from login email. Existing users with missing/invalid recovery email are prompted after sign-in (soft prompt, skippable per session).
- PAM password reset now falls back to account email when `mail_login` is unset.
- In PAM mode, mailbox account creation/provisioning remains external to this app (system/PAM side).

Password reset sender identity:
- Installer now auto-configures same-host SMTP delivery through local Postfix relay on `127.0.0.1:25` when available, instead of preferring local submission on `587`.
- Installer derives `PASSWORD_RESET_FROM` automatically; when your base domain starts with `mail.`, reset mail defaults to `no-reply@<parent-domain>` so a host like `mail.example.com` becomes `no-reply@example.com`.
- Installer also writes `PASSWORD_RESET_BASE_URL` from your chosen public host and enables `PASSWORD_RESET_EXTERNAL_SENDER_READY=true` automatically for PAM service installs when it configured the local same-host relay path itself (`127.0.0.1:25`).
- Current updater releases also migrate the old broken same-host reset-mail pattern in `/opt/despatch/.env` automatically during upgrade, but only when the legacy loopback submission setup (`127.0.0.1:587` with STARTTLS and no SMTP auth) is detected.
- Default sender is `no-reply@<base_domain>` when `PASSWORD_RESET_FROM` is unset or placeholder; if the base domain starts with `mail.`, the sender falls back to the parent domain instead.
- Public password reset requires SMTP delivery (`PASSWORD_RESET_SENDER=smtp`). `log` mode is allowed only when public reset is disabled.
- If your reset sender must authenticate to SMTP separately from mailbox users, set `PASSWORD_RESET_SMTP_USER` and `PASSWORD_RESET_SMTP_PASS`.
- SQL auth mode auto-provisions sender mailbox identity through the configured auth provisioner.
- PAM mode marks sender identity as externally managed and reports diagnostics in readiness/capabilities. `PASSWORD_RESET_EXTERNAL_SENDER_READY=true` is required but not sufficient on its own; reset sender SMTP reachability/auth/policy must also pass.
- Password reset SMTP delivery follows `SMTP_TLS` / `SMTP_STARTTLS` / `SMTP_INSECURE_SKIP_VERIFY` runtime settings.

Installer troubleshooting:
- Installer now reports native Launchpad preflight/action failures directly.
- If UFW rule application fails, installer continues and prints manual commands instead of aborting.
- If UFW is installed but inactive, installer warns and can optionally enable it; if that fails, install still continues.

## Uninstall (safe, interactive)
Removes only Despatch-managed artifacts and leaves Postfix/Dovecot/other services untouched.

From local checkout:
- `./scripts/uninstall.sh`

Standalone from server console (`wget`):
- use the packaged standalone Launchpad installer artifact and choose the `uninstall` operation

## Internet access diagnostics
Run connectivity doctor at any time:
- `./scripts/diagnose_access.sh`

It checks:
- `despatch` service health and local `/health/live`
- reverse proxy status/routing (Nginx/Apache2)
- listening ports
- `ufw` rules
- DNS vs server public IP (when tools are available)

## Web panel software updates (Ubuntu)
Admin panel can check GitHub Releases and queue manual upgrades.

Requires updater units:
- `despatch-updater.path`
- `despatch-updater.service`

Privileged binaries installed by current releases:
- `/opt/despatch/despatch-pam-reset-helper`
- `/opt/despatch/despatch-update-worker`

Default runtime paths:
- request queue: `/var/lib/despatch/update/request/update-request-*.json`
- legacy request file (still accepted during transition): `/var/lib/despatch/update/request/update-request.json`
- status: `/var/lib/despatch/update/status/update-status.json`
- lock: `/var/lib/despatch/update/lock/update.lock`
- backups: `/var/lib/despatch/update/backups/`

Hardened ownership/mode matrix:
- `/opt/despatch` and shipped binaries: `root:root`, binaries `0755`
- `/opt/despatch/.env`: `root:despatch`, `0640` (read-only for app user)
- `/var/lib/despatch/update`: `root:despatch`, `0750` (group traversal required for app writes)
- `/var/lib/despatch/update/request` and `/status`: `root:despatch`, `0770`, files `0660`
- `/var/lib/despatch/update/lock`, `/work`, `/backups`: `root:root`, `0750`

If update check/apply shows `permission denied` on update status/request files, repair ownership:

```bash
sudo chown root:despatch /var/lib/despatch/update
sudo chmod 0750 /var/lib/despatch/update
sudo chown root:despatch /var/lib/despatch/update/request /var/lib/despatch/update/status
sudo chmod 0770 /var/lib/despatch/update/request /var/lib/despatch/update/status
sudo find /var/lib/despatch/update/request /var/lib/despatch/update/status -type f -exec chown root:despatch {} \; -exec chmod 0660 {} \;
sudo chown root:root /var/lib/despatch/update/lock /var/lib/despatch/update/work /var/lib/despatch/update/backups
sudo chmod 0750 /var/lib/despatch/update/lock /var/lib/despatch/update/work /var/lib/despatch/update/backups
sudo -u despatch sh -c 'printf ok > /var/lib/despatch/update/request/.probe && rm -f /var/lib/despatch/update/request/.probe'
sudo -u despatch sh -c 'printf ok > /var/lib/despatch/update/status/.probe && rm -f /var/lib/despatch/update/status/.probe'
sudo systemctl restart despatch despatch-updater.path
```

Relevant `.env` options:
- `UPDATE_ENABLED`
- `UPDATE_REPO_OWNER`
- `UPDATE_REPO_NAME`
- `UPDATE_CHECK_INTERVAL_MIN`
- `UPDATE_HTTP_TIMEOUT_SEC`
- `UPDATE_GITHUB_TOKEN` (optional)
- `UPDATE_BACKUP_KEEP`
- `UPDATE_REQUIRE_SIGNATURE` (default: `true`)
- `UPDATE_SIGNATURE_ASSET` (default: `checksums.txt.sig`)
- `UPDATE_SIGNING_PUBLIC_KEYS` (comma-separated Ed25519 public keys, base64)

Updater authenticity policy:
- `checksums.txt` must have a valid detached Ed25519 signature before archive hash validation.
- Signature verification fails closed when `UPDATE_REQUIRE_SIGNATURE=true`.
- Keep at least one current key in `UPDATE_SIGNING_PUBLIC_KEYS`; during rotation, publish old+new keys together.

PAM helper options:
- `PAM_RESET_HELPER_ENABLED`
- `PAM_RESET_HELPER_SOCKET`
- `PAM_RESET_HELPER_TIMEOUT_SEC`
- `PAM_RESET_ALLOWED_UID`
- `PAM_RESET_ALLOWED_GID`

Secret rotation required on upgrade from older builds:
- Older revisions tracked runtime artifacts (`.env`, `data/app.db*`) in git history.
- Rotate at minimum: `SESSION_ENCRYPT_KEY`, `UPDATE_GITHUB_TOKEN`, CAPTCHA secrets, SMTP/API credentials.
- Revoke active sessions after key rotation.

Emergency fallback to Go privileged paths (single host rollback):

```bash
sudo install -D -m 0644 /opt/despatch/deploy/fallback/despatch-pam-reset-helper-go.override.conf /etc/systemd/system/despatch-pam-reset-helper.service.d/override.conf
sudo install -D -m 0644 /opt/despatch/deploy/fallback/despatch-updater-go.override.conf /etc/systemd/system/despatch-updater.service.d/override.conf
sudo systemctl daemon-reload
sudo systemctl restart despatch-pam-reset-helper.socket despatch-pam-reset-helper.service
sudo systemctl restart despatch-updater.path
```

Updater runtime behavior:
- Admin apply queues a request file and relies on `despatch-updater.path` to hand it to the privileged worker.
- Current updater units watch request-directory changes instead of persistent file-exists matches, so a failed worker cannot drive the path unit into a self-trigger loop from one stale queue file.
- `despatch-updater.path` must stay `active`; if it is inactive, Admin now reports the runtime as not configured instead of leaving updates stuck in `queued`.
- If `despatch-updater.path` enters `failed`, clear the failed state before restarting it: `sudo systemctl reset-failed despatch-updater.service despatch-updater.path && sudo systemctl restart despatch-updater.path`.
- Current releases refresh updater unit templates from `deploy/` during apply so updater fixes self-propagate after one repaired/manual rollout.

## CAPTCHA with CAP standalone (Ubuntu)
Registration supports `turnstile`, `hcaptcha`, and self-hosted [`tiagozip/cap`](https://github.com/tiagozip/cap).

When `CAPTCHA_PROVIDER=cap`, frontend loads widget config from:
- `GET /api/v1/public/captcha/config`

Recommended Ubuntu self-host setup:
1. Run CAP standalone service (Docker or systemd) on localhost, for example `127.0.0.1:8077`.
2. Proxy `/cap/` publicly through your reverse proxy to CAP service.
3. Configure app env:
   - `CAPTCHA_ENABLED=true`
   - `CAPTCHA_PROVIDER=cap`
   - `CAPTCHA_SITE_KEY=<your-site-key>`
   - `CAPTCHA_WIDGET_API_URL=/cap/<your-site-key>/`
   - `CAPTCHA_VERIFY_URL=http://127.0.0.1:8077/<your-site-key>/siteverify`
   - `CAPTCHA_SECRET=<your-secret>`
4. Configure CAP standalone runtime env (required):
   - `ENABLE_ASSETS_SERVER=true`
   - `WIDGET_VERSION=<pinned-version>` (avoid `latest` in production)
   - `WASM_VERSION=<pinned-version>` (avoid `latest` in production)
   - `CACHE_HOST` optional (set only if outbound fetch is restricted)

The native Launchpad install operation can auto-provision CAP runtime:
- optional Docker deployment to `/opt/cap/docker-compose.yml`
- canonical volume `cap-data` (with migration from legacy `cap_cap-data` when safe)
- forced `ENABLE_ASSETS_SERVER=true`
- pinned default versions if unset/invalid
- container name conflict repair (`/cap`) and compose fallback to direct `docker run`
- smoke checks for upstream + proxied CAP assets and `siteverify`

CAP runtime health checks (required):
- Do **not** use `GET /<site_key>/` as primary health signal.
- Use these checks instead:
  - `GET /assets/widget.js` -> `200`
  - `GET /assets/cap_wasm.js` -> `200`
  - `GET /assets/cap_wasm_bg.wasm` -> `200`
  - `POST /<site_key>/siteverify` -> JSON response containing `success`

Example checks:
```bash
curl -i http://127.0.0.1:8077/assets/widget.js
curl -i http://127.0.0.1:8077/assets/cap_wasm.js
curl -i http://127.0.0.1:8077/assets/cap_wasm_bg.wasm
curl -i -H 'Content-Type: application/json' \
  -d '{"secret":"<site-secret>","response":"probe-invalid-token"}' \
  http://127.0.0.1:8077/<site_key>/siteverify
```

Volume drift / key mismatch runbook (`cap-data` vs `cap_cap-data`):
```bash
sudo docker volume ls | grep cap
sudo docker inspect cap --format '{{json .Mounts}}' | jq
sudo docker run --rm -v cap-data:/a -v cap_cap-data:/b alpine sh -lc 'ls -la /a; ls -la /b'
```
- Keep one canonical CAP data volume (recommended: `cap-data`).
- If keys exist in a different volume than the running container mount, migrate once and remove stale volume references.
- Ensure `CAPTCHA_SITE_KEY` in `/opt/despatch/.env` matches the key in CAP dashboard on the mounted volume.
- Ensure `CAPTCHA_SECRET` is the per-site verification secret (not CAP admin key).

Failure policy is fail-closed for registration:
- Missing/invalid challenge: `captcha_required` (`400`)
- Verification backend unavailable: `captcha_unavailable` (`503`)

## SMTP sender diagnostics (Ubuntu)
If sending fails with `smtp_sender_rejected` or `smtp_error` related to sender identity, run:

```bash
sudo grep -E '^(DOVECOT_AUTH_MODE|APP_DB_PATH)=' /opt/despatch/.env
DB_PATH="$(sudo awk -F= '/^APP_DB_PATH=/{print $2}' /opt/despatch/.env)"
if [[ "$DB_PATH" != /* ]]; then DB_PATH="/opt/despatch/${DB_PATH#./}"; fi
sudo sqlite3 "$DB_PATH" "SELECT email, COALESCE(mail_login,'') AS mail_login FROM users ORDER BY created_at DESC LIMIT 20;"
```

Quick local send smoke test through API:

```bash
curl -sS -c /tmp/despatch.cookies -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"YOUR_PASSWORD"}' \
  http://127.0.0.1:8080/api/v1/login >/dev/null
CSRF="$(awk '$6=="despatch_csrf"{print $7}' /tmp/despatch.cookies)"
curl -i -b /tmp/despatch.cookies -H "X-CSRF-Token: $CSRF" -H 'Content-Type: application/json' \
  -d '{"to":["postmaster@example.com"],"subject":"probe","body":"probe"}' \
  http://127.0.0.1:8080/api/v1/messages/send
sudo journalctl -u despatch -n 200 --no-pager | grep -E 'messages/send|smtp_sender_rejected|smtp_error' || true
```

## Dovecot provisioning notes
Set `DOVECOT_AUTH_DB_DRIVER` and `DOVECOT_AUTH_DB_DSN` to enable automatic writes to your Dovecot auth DB.

Set `DOVECOT_AUTH_MODE`:
- `sql` (default): app provisions Dovecot auth rows when SQL DSN is configured.
- `pam`: app skips provisioning and uses IMAP/PAM credentials for login.

Supported drivers:
- `mysql`
- `pgx` (PostgreSQL)

Adjust table/column env vars if your auth schema differs.

## API
Base: `/api/v1`

- Auth/registration:
  - `GET /public/captcha/config`
  - `GET /public/auth/capabilities`
  - `GET /public/password-reset/capabilities` (includes sender diagnostics: `sender_address`, `sender_status`, `sender_reason`)
  - `GET /setup/status`
  - `POST /setup/complete`
  - `POST /register`
  - `POST /login`
  - `POST /login/passkey/begin`
  - `POST /login/passkey/finish`
  - `POST /logout`
  - `POST /password/reset/request`
  - `POST /password/reset/confirm`
- User mail:
  - `GET /me` (includes `session_id`, MFA stage fields, and `mail_secret_required`)
  - `POST /session/mail-secret/unlock`
  - `GET /mailboxes`
  - `GET /messages`
  - `GET /messages/{id}`
  - `GET /compose/identities` (returns `auth_email`, `manual_fallback_required`, identity list from accounts/identities)
  - `POST /messages/send` (supports legacy payload and extended fields: `cc`, `bcc`, `body_html`, `from_mode`, `identity_id`, `account_id`, `from_manual`, multipart `inline_images` + `inline_image_cids`)
  - `POST /messages/{id}/reply`
  - `POST /messages/{id}/forward`
  - `POST /messages/{id}/flags`
  - `POST /messages/{id}/move`
  - `GET /search`
  - `GET /attachments/{id}`
- Admin:
  - `GET /admin/registrations` (supports `q,status,sort,order,page,page_size`; includes `total`)
  - `POST /admin/registrations/{id}/approve`
  - `POST /admin/registrations/{id}/reject`
  - `POST /admin/registrations/bulk/decision`
  - `GET /admin/users` (supports `q,status,role,provision_state,sort,order,page,page_size`; includes `total`)
  - `POST /admin/users/{id}/suspend`
  - `POST /admin/users/{id}/unsuspend`
  - `POST /admin/users/bulk/action`
  - `POST /admin/users/{id}/reset-password`
  - `GET /admin/audit-log` (supports `q,action,actor,target,from,to,sort,order,page,page_size`; includes `total` + server-generated summary fields)
  - `GET /admin/system/mail-health`
  - `POST /admin/users/{id}/retry-provision`
  - `GET /admin/system/version`
  - `GET /admin/system/update/status`
  - `POST /admin/system/update/check`
  - `POST /admin/system/update/apply`
  - `GET /admin/system/feature-flags`
  - `POST /admin/system/feature-flags/{id}` with body `{ "enabled": true|false }`
  - `POST /admin/system/feature-flags/{id}/reset`
  - Feature flag items include: `id,name,description,category,enabled,default_enabled,source,editable,requires_restart,note`

### API v2 foundation
Base: `/api/v2`

This release adds a new v2 surface while keeping `/api/v1` unchanged.

- Auth/MFA:
  - `POST /login`
  - `POST /login/passkey/begin`
  - `POST /login/passkey/finish`
  - `POST /session/mail-secret/unlock`
  - `POST /mfa/totp/verify`
  - `POST /mfa/webauthn/begin`
  - `POST /mfa/webauthn/finish`
  - `POST /mfa/recovery-code/verify`
  - `WebAuthn` and `TOTP` flows require `mailsec_service` when `MAILSEC_ENABLED=true`.
- Accounts and identities:
  - `GET /accounts`
  - `POST /accounts`
  - `PATCH /accounts/{id}`
  - `DELETE /accounts/{id}`
  - `POST /accounts/{id}/activate`
  - `GET /accounts/{id}/identities`
  - `POST /accounts/{id}/identities`
  - `PATCH /identities/{id}`
  - `DELETE /identities/{id}`
- Mail index and actions:
  - `GET /mailboxes`
  - `POST /mailboxes`
  - `PATCH /mailboxes/{id}`
  - `DELETE /mailboxes/{id}`
  - `GET /threads`
  - `GET /threads/{id}`
  - `GET /messages/{id}`
  - `GET /messages/{id}/raw`
  - `POST /messages/bulk`
  - `POST /messages/{id}/remote-images/allow`
  - `POST /messages/{id}/crypto/decrypt`
  - `POST /messages/{id}/crypto/verify`
- Search/saved queries:
  - `GET /search`
  - `GET /saved-searches`
  - `POST /saved-searches`
  - `PATCH /saved-searches/{id}`
  - `DELETE /saved-searches/{id}`
- Drafts/send:
  - `GET /drafts`
  - `POST /drafts`
  - `PATCH /drafts/{id}`
  - `GET /drafts/{id}/versions`
  - `POST /drafts/{id}/send`
  - `POST /messages/send`
- Rules/preferences/security:
  - `GET /rules/scripts`
  - `GET /rules/scripts/{name}`
  - `PUT /rules/scripts/{name}`
  - `POST /rules/scripts/{name}/activate`
  - `DELETE /rules/scripts/{name}`
  - `POST /rules/validate`
  - `GET /preferences`
  - `PUT /preferences`
  - `GET /security/mfa/status`
  - `GET /security/mfa/webauthn`
  - `POST /security/mfa/totp/enroll`
  - `POST /security/mfa/totp/confirm`
  - `POST /security/mfa/webauthn/register/begin`
  - `POST /security/mfa/webauthn/register/finish`
  - `PATCH /security/mfa/webauthn/{id}`
  - `DELETE /security/mfa/webauthn/{id}`
  - `GET /security/mfa/trusted-devices` (includes `is_current`, `display_label`, `browser`, `os`, `device_type`)
  - `POST /security/mfa/trusted-devices/{id}/revoke`
  - `POST /security/mfa/trusted-devices/revoke-all`
  - `GET /security/sessions`
  - `POST /security/sessions/{id}/revoke` (`GET /security/sessions` includes `is_current`)
  - `GET /security/crypto/keyrings`
  - `POST /security/crypto/keyrings`
  - `PATCH /security/crypto/keyrings/{id}`
  - `DELETE /security/crypto/keyrings/{id}`
  - `GET /security/crypto/trust-policies`
  - `POST /security/crypto/trust-policies`
  - `PATCH /security/crypto/trust-policies/{id}`
  - `DELETE /security/crypto/trust-policies/{id}`
  - `GET /quota`

## Test
- `go test ./...`
- `./scripts/check_warm_palette.sh`

## Security behavior updates
- Password reset request endpoint no longer returns raw reset tokens.
- CAPTCHA (when enabled) is server-verified.
- CORS now uses explicit allowlist via `CORS_ALLOWED_ORIGINS`.
- Cookie policy now supports `COOKIE_SECURE_MODE` (preferred) with legacy `COOKIE_SECURE` compatibility.

## Mail Security Service
- Rust `mailsec_service` is now part of the workspace (`rust/mailsec_service`).
- IPC contract is frozen in [`docs/security/rust-mailsec-contracts.md`](docs/security/rust-mailsec-contracts.md).
- Contract wire format remains length-prefixed JSON frames over UNIX sockets.
- `mailsec_service` implements PGP and S/MIME operations used by `/api/v2` compose and reader crypto flows:
  - sign / encrypt / decrypt / verify
  - strict JSON decoding (`deny_unknown_fields`) and bounded IPC frame size.
