#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_INSTALL_ENV="/opt/despatch/.env"
ENV_FILE="${DESPATCH_ENV_FILE:-}"

have_cmd() {
  command -v "$1" >/dev/null 2>&1
}

trim() {
  local s="${1:-}"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  printf '%s' "$s"
}

lower() {
  printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]'
}

truthy() {
  case "$(lower "$(trim "${1:-}")")" in
    1|y|yes|true|on) return 0 ;;
  esac
  return 1
}

section() {
  printf '\n=== %s ===\n' "$1"
}

command_note() {
  printf '$'
  printf ' %q' "$@"
  printf '\n'
}

run_or_note() {
  command_note "$@"
  local rc=0
  if "$@"; then
    rc=0
  else
    rc=$?
  fi
  if [[ $rc -eq 0 ]]; then
    return 0
  fi
  printf '[command failed] exit=%d\n' "$rc"
  return 0
}

env_file_dir() {
  dirname "$ENV_FILE"
}

get_env() {
  local key="$1" default="${2:-}" line=""
  if [[ -f "$ENV_FILE" ]]; then
    line="$(grep -E "^${key}=" "$ENV_FILE" | tail -n1 || true)"
    if [[ -n "$line" ]]; then
      printf '%s' "${line#*=}"
      return
    fi
  fi
  printf '%s' "$default"
}

resolve_path() {
  local raw="$1" base="$2"
  if [[ -z "$raw" ]]; then
    return
  fi
  if [[ "$raw" == /* ]]; then
    printf '%s' "$raw"
    return
  fi
  printf '%s/%s' "$base" "${raw#./}"
}

health_url_from_listen() {
  local listen_addr="$1"
  local host="127.0.0.1" port="8080"

  if [[ "$listen_addr" == :* ]]; then
    port="${listen_addr#:}"
  elif [[ "$listen_addr" == *:* ]]; then
    host="${listen_addr%:*}"
    port="${listen_addr##*:}"
    host="${host#[}"
    host="${host%]}"
  fi

  if [[ -z "$port" || ! "$port" =~ ^[0-9]+$ ]]; then
    port="8080"
  fi
  if [[ -z "$host" || "$host" == "0.0.0.0" || "$host" == "::" ]]; then
    host="127.0.0.1"
  fi
  if [[ "$host" == *:* ]]; then
    printf 'http://[%s]:%s/health/live' "$host" "$port"
    return
  fi
  printf 'http://%s:%s/health/live' "$host" "$port"
}

port_from_listen() {
  local listen_addr="$1"
  local port="8080"
  if [[ "$listen_addr" == :* ]]; then
    port="${listen_addr#:}"
  elif [[ "$listen_addr" == *:* ]]; then
    port="${listen_addr##*:}"
  fi
  if [[ "$port" =~ ^[0-9]+$ ]]; then
    printf '%s' "$port"
    return
  fi
  printf '8080'
}

build_listener_pattern() {
  local ports=()
  local seen=" "
  local candidate=""
  for candidate in "$@"; do
    candidate="$(trim "$candidate")"
    if [[ ! "$candidate" =~ ^[0-9]+$ ]]; then
      continue
    fi
    if [[ "$seen" == *" ${candidate} "* ]]; then
      continue
    fi
    seen+="$(printf '%s ' "$candidate")"
    ports+=("$candidate")
  done
  if [[ "${#ports[@]}" -eq 0 ]]; then
    printf ':[0-9]+([[:space:]]|$)'
    return
  fi
  local joined=""
  joined="$(IFS='|'; printf '%s' "${ports[*]}")"
  printf ':(%s)([[:space:]]|$)' "$joined"
}

mask_email_sql() {
  local expr="$1"
  printf "CASE
    WHEN instr(%s, '@') > 0 THEN substr(%s, 1, 2) || '***@' || substr(%s, instr(%s, '@') + 1)
    WHEN trim(coalesce(%s, '')) <> '' THEN substr(%s, 1, 2) || '***'
    ELSE ''
  END" "$expr" "$expr" "$expr" "$expr" "$expr" "$expr"
}

sqlite_query() {
  local sql="$1"
  if ! have_cmd sqlite3; then
    printf '[sqlite3 not installed]\n'
    return 0
  fi
  if [[ -z "${DB_PATH:-}" || ! -f "$DB_PATH" ]]; then
    printf '[database file not found]\n'
    return 0
  fi
  local rc=0
  if sqlite3 -cmd '.headers on' -cmd '.mode column' "$DB_PATH" "$sql"; then
    rc=0
  else
    rc=$?
  fi
  if [[ $rc -eq 0 ]]; then
    return 0
  fi
  printf '[sqlite query failed] exit=%d\n' "$rc"
  return 0
}

table_exists() {
  local table="$1"
  [[ -n "$(sqlite3 "$DB_PATH" "SELECT name FROM sqlite_master WHERE type='table' AND name='${table}' LIMIT 1;" 2>/dev/null || true)" ]]
}

column_exists() {
  local table="$1" column="$2"
  [[ -n "$(sqlite3 "$DB_PATH" "SELECT 1 FROM pragma_table_info('${table}') WHERE name='${column}' LIMIT 1;" 2>/dev/null || true)" ]]
}

report_missing_columns() {
  local table="$1"
  shift
  local missing=()
  local col=""
  if ! table_exists "$table"; then
    printf '%s: missing table\n' "$table"
    return 0
  fi
  for col in "$@"; do
    if ! column_exists "$table" "$col"; then
      missing+=("$col")
    fi
  done
  if [[ "${#missing[@]}" -eq 0 ]]; then
    printf '%s: all expected columns present\n' "$table"
  else
    printf '%s: missing columns: %s\n' "$table" "${missing[*]}"
  fi
}

print_relevant_logs() {
  local unit="$1"
  local lines="$2"
  if ! have_cmd journalctl; then
    printf '[journalctl not available]\n'
    return 0
  fi
  local log_lines=""
  log_lines="$(journalctl -u "$unit" -n "$lines" --no-pager 2>/dev/null || true)"
  if [[ -z "$log_lines" ]]; then
    printf '[no journal entries available for %s]\n' "$unit"
    return 0
  fi
  printf '%s\n' "$log_lines" | grep -E \
    'migration|request method=.*(/api/v1/messages/send|/api/v2/messages/send|/api/v2/drafts|/api/v2/drafts/.*/send|/api/v1/compose/identities|/api/v2/session/mail-secret/unlock)|auth_failed|create_draft_failed|update_draft_failed|draft_.*failed|draft_not_found|send_failed|smtp_|mail_auth_missing|session_invalid|cannot decrypt account secret|missing mail credentials|compose_identities_failed|password_reset_sender_|mail_sync .*error|scheduled_send ' \
    || printf '[no matching mail/draft log lines in last %s entries]\n' "$lines"
}

if [[ -z "$ENV_FILE" ]]; then
  if [[ -f "$DEFAULT_INSTALL_ENV" ]]; then
    ENV_FILE="$DEFAULT_INSTALL_ENV"
  else
    ENV_FILE="$ROOT_DIR/.env"
  fi
fi

LISTEN_ADDR="$(trim "$(get_env LISTEN_ADDR ":8080")")"
APP_DB_PATH_RAW="$(trim "$(get_env APP_DB_PATH "./data/app.db")")"
DB_PATH="$(resolve_path "$APP_DB_PATH_RAW" "$(env_file_dir)")"
READY_URL="$(health_url_from_listen "$LISTEN_ADDR")"
READY_URL="${READY_URL%/health/live}/health/ready"
APP_PORT="$(port_from_listen "$LISTEN_ADDR")"
IMAP_PORT_RAW="$(trim "$(get_env IMAP_PORT "")")"
SMTP_PORT_RAW="$(trim "$(get_env SMTP_PORT "")")"
LISTENER_PATTERN="$(build_listener_pattern "$APP_PORT" "$IMAP_PORT_RAW" "$SMTP_PORT_RAW" 25 143 465 587 993)"
MAILSEC_ENABLED_RAW="$(trim "$(get_env MAILSEC_ENABLED "false")")"
PAM_RESET_HELPER_ENABLED_RAW="$(trim "$(get_env PAM_RESET_HELPER_ENABLED "false")")"

section "Runtime"
printf 'timestamp_utc=%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
printf 'hostname=%s\n' "$(hostname 2>/dev/null || printf 'unknown')"
printf 'uname=%s\n' "$(uname -a 2>/dev/null || printf 'unknown')"
printf 'cwd=%s\n' "$(pwd)"
printf 'repo_root=%s\n' "$ROOT_DIR"
printf 'env_file=%s\n' "$ENV_FILE"
printf 'db_path=%s\n' "${DB_PATH:-}"
printf 'euid=%s\n' "${EUID:-unknown}"
if have_cmd git; then
  printf 'git_head=%s\n' "$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || printf 'unknown')"
  printf 'git_branch=%s\n' "$(git -C "$ROOT_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null || printf 'unknown')"
fi

section "Config"
printf 'listen_addr=%s\n' "$LISTEN_ADDR"
printf 'base_domain=%s\n' "$(trim "$(get_env BASE_DOMAIN "")")"
printf 'app_db_path_raw=%s\n' "$APP_DB_PATH_RAW"
printf 'dovecot_auth_mode=%s\n' "$(trim "$(get_env DOVECOT_AUTH_MODE "")")"
printf 'imap_host=%s\n' "$(trim "$(get_env IMAP_HOST "")")"
printf 'imap_port=%s\n' "$(trim "$(get_env IMAP_PORT "")")"
printf 'imap_tls=%s\n' "$(trim "$(get_env IMAP_TLS "")")"
printf 'imap_starttls=%s\n' "$(trim "$(get_env IMAP_STARTTLS "")")"
printf 'imap_insecure_skip_verify=%s\n' "$(trim "$(get_env IMAP_INSECURE_SKIP_VERIFY "")")"
printf 'smtp_host=%s\n' "$(trim "$(get_env SMTP_HOST "")")"
printf 'smtp_port=%s\n' "$(trim "$(get_env SMTP_PORT "")")"
printf 'smtp_tls=%s\n' "$(trim "$(get_env SMTP_TLS "")")"
printf 'smtp_starttls=%s\n' "$(trim "$(get_env SMTP_STARTTLS "")")"
printf 'smtp_insecure_skip_verify=%s\n' "$(trim "$(get_env SMTP_INSECURE_SKIP_VERIFY "")")"
printf 'mailsec_enabled=%s\n' "$MAILSEC_ENABLED_RAW"
printf 'mailsec_socket=%s\n' "$(trim "$(get_env MAILSEC_SOCKET "")")"
printf 'pam_reset_helper_enabled=%s\n' "$PAM_RESET_HELPER_ENABLED_RAW"
printf 'pam_reset_helper_socket=%s\n' "$(trim "$(get_env PAM_RESET_HELPER_SOCKET "")")"
printf 'session_encrypt_key_set=%s\n' "$([[ -n "$(trim "$(get_env SESSION_ENCRYPT_KEY "")")" ]] && printf 'yes' || printf 'no')"
printf 'session_encrypt_key_len=%s\n' "$(printf '%s' "$(get_env SESSION_ENCRYPT_KEY "")" | wc -c | awk '{print $1}')"

section "Health"
printf 'live_url=%s\n' "${READY_URL%/health/ready}/health/live"
printf 'ready_url=%s\n' "$READY_URL"
if have_cmd curl; then
  run_or_note curl -fsS --max-time 8 "${READY_URL%/health/ready}/health/live"
  run_or_note curl -fsS --max-time 8 "$READY_URL"
else
  printf '[curl not installed]\n'
fi

section "Services"
if have_cmd systemctl; then
  run_or_note systemctl is-active despatch
  run_or_note systemctl is-enabled despatch
  run_or_note systemctl status despatch --no-pager
  if truthy "$MAILSEC_ENABLED_RAW"; then
    run_or_note systemctl is-active despatch-mailsec
    run_or_note systemctl status despatch-mailsec --no-pager
  fi
  if truthy "$PAM_RESET_HELPER_ENABLED_RAW"; then
    run_or_note systemctl is-active despatch-pam-reset-helper.socket
    run_or_note systemctl status despatch-pam-reset-helper.socket --no-pager
  fi
else
  printf '[systemctl not available]\n'
fi

section "Listeners"
if have_cmd ss; then
  command_note ss -ltnp
  ss_out="$(ss -ltnp 2>/dev/null || true)"
  if [[ -n "$ss_out" ]]; then
    printf '%s\n' "$ss_out" | awk 'NR==1'
    printf '%s\n' "$ss_out" | grep -E "$LISTENER_PATTERN" || true
  else
    printf '[no listener output]\n'
  fi
elif have_cmd lsof; then
  command_note lsof -nP -iTCP -sTCP:LISTEN
  lsof_out="$(lsof -nP -iTCP -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$lsof_out" ]]; then
    printf '%s\n' "$lsof_out" | awk 'NR==1'
    printf '%s\n' "$lsof_out" | grep -E "$LISTENER_PATTERN" || true
  else
    printf '[no listener output]\n'
  fi
elif have_cmd netstat; then
  command_note netstat -an
  netstat_out="$(netstat -an 2>/dev/null || true)"
  if [[ -n "$netstat_out" ]]; then
    printf 'Proto Local-Address State\n'
    printf '%s\n' "$netstat_out" | grep -E 'LISTEN|tcp' | grep -E "$LISTENER_PATTERN" || true
  else
    printf '[no listener output]\n'
  fi
else
  printf '[ss/netstat not available]\n'
fi

section "Relevant Logs"
print_relevant_logs despatch 500
if truthy "$MAILSEC_ENABLED_RAW"; then
  section "MailSec Logs"
  print_relevant_logs despatch-mailsec 200
fi

section "Database File"
if [[ -n "$DB_PATH" && -f "$DB_PATH" ]]; then
  run_or_note ls -lh "$DB_PATH"
else
  printf '[database file not found at %s]\n' "$DB_PATH"
fi

section "Schema Checks"
if [[ -n "$DB_PATH" && -f "$DB_PATH" ]] && have_cmd sqlite3; then
  report_missing_columns drafts \
    id user_id account_id identity_id to_value cc_value bcc_value subject body_text body_html \
    attachments_json crypto_options_json send_mode scheduled_for status created_at updated_at \
    compose_mode context_message_id from_mode from_manual client_state_json last_send_error context_account_id
  report_missing_columns sessions \
    id user_id token_hash mail_secret expires_at idle_expires_at mfa_verified_at auth_method active_account_id
  report_missing_columns users \
    id email role status provision_state provision_error mail_login mail_secret_enc mail_secret_updated_at recovery_email
  report_missing_columns mail_accounts \
    id user_id login secret_enc imap_host imap_port imap_tls imap_starttls smtp_host smtp_port smtp_tls smtp_starttls status last_sync_at last_error
  report_missing_columns mail_identities \
    id account_id display_name from_email reply_to is_default
  report_missing_columns session_mail_profiles \
    id user_id from_email display_name reply_to signature_text signature_html updated_at
else
  printf '[schema checks skipped]\n'
fi

section "SQLite Integrity"
sqlite_query "PRAGMA quick_check;"
if table_exists sessions; then
  sqlite_query "PRAGMA foreign_key_check;"
fi

section "Table Info: drafts"
sqlite_query "PRAGMA table_info(drafts);"

section "Table Info: sessions"
sqlite_query "PRAGMA table_info(sessions);"

section "Table Info: users"
sqlite_query "PRAGMA table_info(users);"

section "Table Info: mail_accounts"
sqlite_query "PRAGMA table_info(mail_accounts);"

section "Table Info: session_mail_profiles"
if table_exists session_mail_profiles; then
  sqlite_query "PRAGMA table_info(session_mail_profiles);"
else
  printf '[table missing]\n'
fi

section "User Summary"
sqlite_query "
SELECT
  COUNT(1) AS total_users,
  SUM(CASE WHEN status='active' THEN 1 ELSE 0 END) AS active_users,
  SUM(CASE WHEN lower(trim(coalesce(provision_state, '')))='error' THEN 1 ELSE 0 END) AS provision_errors,
  SUM(CASE WHEN length(trim(coalesce(mail_login, ''))) = 0 THEN 1 ELSE 0 END) AS users_missing_mail_login,
  SUM(CASE WHEN length(trim(coalesce(mail_secret_enc, ''))) = 0 THEN 1 ELSE 0 END) AS users_missing_mail_secret_enc
FROM users;
"

section "Recent Users"
sqlite_query "
SELECT
  substr(id, 1, 8) AS user_id,
  $(mask_email_sql "email") AS email,
  $(mask_email_sql "mail_login") AS mail_login,
  status,
  coalesce(provision_state, '') AS provision_state,
  CASE WHEN length(trim(coalesce(mail_secret_enc, ''))) > 0 THEN 'yes' ELSE 'no' END AS has_mail_secret_enc,
  created_at,
  coalesce(last_login_at, '') AS last_login_at
FROM users
ORDER BY datetime(created_at) DESC
LIMIT 20;
"

section "Session Summary"
sqlite_query "
SELECT
  COUNT(1) AS total_sessions,
  SUM(CASE WHEN revoked_at IS NULL THEN 1 ELSE 0 END) AS not_revoked,
  SUM(CASE WHEN revoked_at IS NULL AND datetime(expires_at) > datetime('now') THEN 1 ELSE 0 END) AS unexpired_not_revoked,
  SUM(CASE WHEN revoked_at IS NULL AND datetime(expires_at) > datetime('now') AND length(trim(coalesce(mail_secret, ''))) = 0 THEN 1 ELSE 0 END) AS active_sessions_missing_mail_secret
FROM sessions;
"

section "Recent Sessions"
sqlite_query "
SELECT
  substr(s.id, 1, 8) AS session_id,
  $(mask_email_sql "u.email") AS user_email,
  coalesce(s.auth_method, '') AS auth_method,
  CASE WHEN length(trim(coalesce(s.mail_secret, ''))) > 0 THEN 'yes' ELSE 'no' END AS has_mail_secret,
  substr(coalesce(s.active_account_id, ''), 1, 8) AS active_account_id,
  s.last_seen_at,
  s.expires_at,
  coalesce(s.revoked_at, '') AS revoked_at
FROM sessions s
JOIN users u ON u.id = s.user_id
ORDER BY datetime(s.last_seen_at) DESC
LIMIT 20;
"

section "Mail Accounts"
sqlite_query "
SELECT
  substr(ma.id, 1, 8) AS account_id,
  $(mask_email_sql "u.email") AS user_email,
  trim(coalesce(ma.display_name, '')) AS display_name,
  $(mask_email_sql "ma.login") AS login,
  ma.imap_host || ':' || ma.imap_port AS imap_endpoint,
  CASE
    WHEN ma.imap_tls = 1 THEN 'tls'
    WHEN ma.imap_starttls = 1 THEN 'starttls'
    ELSE 'plain'
  END AS imap_mode,
  ma.smtp_host || ':' || ma.smtp_port AS smtp_endpoint,
  CASE
    WHEN ma.smtp_tls = 1 THEN 'tls'
    WHEN ma.smtp_starttls = 1 THEN 'starttls'
    ELSE 'plain'
  END AS smtp_mode,
  ma.status,
  ma.is_default,
  CASE WHEN length(trim(coalesce(ma.secret_enc, ''))) > 0 THEN 'yes' ELSE 'no' END AS has_secret_enc,
  coalesce(ma.last_sync_at, '') AS last_sync_at,
  substr(coalesce(ma.last_error, ''), 1, 160) AS last_error
FROM mail_accounts ma
JOIN users u ON u.id = ma.user_id
ORDER BY datetime(ma.updated_at) DESC
LIMIT 30;
"

section "Mail Identities"
sqlite_query "
SELECT
  substr(mi.id, 1, 8) AS identity_id,
  substr(ma.id, 1, 8) AS account_id,
  $(mask_email_sql "u.email") AS user_email,
  trim(coalesce(mi.display_name, '')) AS display_name,
  $(mask_email_sql "mi.from_email") AS from_email,
  $(mask_email_sql "mi.reply_to") AS reply_to,
  mi.is_default,
  mi.updated_at
FROM mail_identities mi
JOIN mail_accounts ma ON ma.id = mi.account_id
JOIN users u ON u.id = ma.user_id
ORDER BY datetime(mi.updated_at) DESC
LIMIT 40;
"

section "Session Mail Profiles"
if table_exists session_mail_profiles; then
  sqlite_query "
SELECT
  substr(smp.id, 1, 8) AS profile_id,
  $(mask_email_sql "u.email") AS user_email,
  $(mask_email_sql "smp.from_email") AS from_email,
  trim(coalesce(smp.display_name, '')) AS display_name,
  $(mask_email_sql "smp.reply_to") AS reply_to,
  smp.updated_at
FROM session_mail_profiles smp
JOIN users u ON u.id = smp.user_id
ORDER BY datetime(smp.updated_at) DESC
LIMIT 30;
"
else
  printf '[table missing]\n'
fi

if table_exists drafts; then
  DRAFT_FIELDS=(
    "substr(d.id, 1, 8) AS draft_id"
    "$(mask_email_sql "u.email") AS user_email"
    "substr(coalesce(d.account_id, ''), 1, 8) AS account_id"
    "substr(coalesce(d.identity_id, ''), 1, 8) AS identity_id"
    "coalesce(d.status, '') AS status"
    "coalesce(d.send_mode, '') AS send_mode"
  )
  if column_exists drafts compose_mode; then
    DRAFT_FIELDS+=("coalesce(d.compose_mode, '') AS compose_mode")
  fi
  if column_exists drafts context_message_id; then
    DRAFT_FIELDS+=("substr(coalesce(d.context_message_id, ''), 1, 16) AS context_message_id")
  fi
  if column_exists drafts context_account_id; then
    DRAFT_FIELDS+=("substr(coalesce(d.context_account_id, ''), 1, 8) AS context_account_id")
  fi
  DRAFT_FIELDS+=(
    "length(coalesce(d.to_value, '')) AS to_len"
    "length(coalesce(d.body_text, '')) AS body_text_len"
    "length(coalesce(d.body_html, '')) AS body_html_len"
    "length(coalesce(d.attachments_json, '')) AS attachments_json_len"
  )
  if column_exists drafts last_send_error; then
    DRAFT_FIELDS+=("substr(coalesce(d.last_send_error, ''), 1, 160) AS last_send_error")
  fi
  DRAFT_FIELDS+=("d.updated_at")
  DRAFT_SELECT="$(printf ', %s' "${DRAFT_FIELDS[@]}")"
  DRAFT_SELECT="${DRAFT_SELECT#, }"

  section "Recent Drafts"
  sqlite_query "
SELECT
  ${DRAFT_SELECT}
FROM drafts d
JOIN users u ON u.id = d.user_id
ORDER BY datetime(d.updated_at) DESC
LIMIT 30;
"

  if column_exists drafts last_send_error; then
    section "Drafts With Send Errors"
    sqlite_query "
SELECT
  substr(d.id, 1, 8) AS draft_id,
  $(mask_email_sql "u.email") AS user_email,
  coalesce(d.status, '') AS status,
  substr(coalesce(d.last_send_error, ''), 1, 200) AS last_send_error,
  d.updated_at
FROM drafts d
JOIN users u ON u.id = d.user_id
WHERE trim(coalesce(d.last_send_error, '')) <> ''
ORDER BY datetime(d.updated_at) DESC
LIMIT 20;
"
  fi
fi

section "Done"
printf 'If you ran this with tee, send the saved report file back along with the exact UI action that failed and the approximate UTC time.\n'
