#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'
DESPATCH_NONINTERACTIVE="${DESPATCH_NONINTERACTIVE:-0}"
DESPATCH_TUI_MODE="${DESPATCH_TUI_MODE:-0}"
DESPATCH_RUN_ID="${DESPATCH_RUN_ID:-run-$(date +%s)}"
CURRENT_STAGE_ID=""
RUN_RESULT_EMITTED=0

log() { printf '[INFO] %s\n' "$*"; }
warn() { printf '[WARN] %s\n' "$*" >&2; }
err() { printf '[ERR ] %s\n' "$*" >&2; }

if [[ $# -ne 0 ]]; then
  err "This uninstaller does not accept CLI arguments. Use environment variables for non-interactive mode."
  exit 1
fi

have_cmd() { command -v "$1" >/dev/null 2>&1; }

run_as_root() {
  if [[ "${EUID:-1}" -ne 0 ]]; then
    if ! have_cmd sudo; then
      err "sudo is required for this operation"
      exit 1
    fi
    sudo "$@"
    return
  fi
  "$@"
}

lower() {
  echo "$1" | tr '[:upper:]' '[:lower:]'
}

trim() {
  local s="$1"
  # shellcheck disable=SC2001
  s="$(echo "$s" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  printf '%s' "$s"
}

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

emit_event() {
  [[ "$DESPATCH_TUI_MODE" == "1" ]] || return 0
  local type="$1"
  shift
  local body="\"type\":\"$(json_escape "$type")\""
  while [[ $# -gt 1 ]]; do
    body="${body},\"$(json_escape "$1")\":\"$(json_escape "$2")\""
    shift 2
  done
  printf '::despatch-event::{%s}\n' "$body"
}

begin_stage() {
  local id="$1" title="$2" weight="$3"
  if [[ -n "$CURRENT_STAGE_ID" ]]; then
    emit_event "stage_result" "stage_id" "$CURRENT_STAGE_ID" "status" "ok" "error_code" ""
  fi
  CURRENT_STAGE_ID="$id"
  emit_event "stage_start" "stage_id" "$id" "title" "$title" "weight" "$weight"
}

finish_stage_ok() {
  if [[ -n "$CURRENT_STAGE_ID" ]]; then
    emit_event "stage_result" "stage_id" "$CURRENT_STAGE_ID" "status" "ok" "error_code" ""
  fi
  CURRENT_STAGE_ID=""
}

emit_run_result_once() {
  local status="$1" failed_stage="$2" exit_code="$3"
  if [[ "$RUN_RESULT_EMITTED" == "1" ]]; then
    return
  fi
  RUN_RESULT_EMITTED=1
  emit_event "run_result" "status" "$status" "failed_stage" "$failed_stage" "exit_code" "$exit_code"
}

on_uninstall_error() {
  local code="$1"
  if [[ -n "$CURRENT_STAGE_ID" ]]; then
    emit_event "stage_result" "stage_id" "$CURRENT_STAGE_ID" "status" "failed" "error_code" "E_UNINSTALL"
  fi
  emit_run_result_once "failed" "${CURRENT_STAGE_ID:-unknown}" "$code"
}
trap 'on_uninstall_error "$?"' ERR
trap 'rc=$?; if [[ "$rc" -ne 0 ]]; then on_uninstall_error "$rc"; fi' EXIT

truthy() {
  local v
  v="$(lower "$(trim "${1:-}")")"
  case "$v" in
    1|y|yes|true|on) return 0 ;;
  esac
  return 1
}

env_key_for_prompt() {
  local prompt="$1"
  case "$prompt" in
    "Continue with uninstall?") echo "DESPATCH_CONFIRM_UNINSTALL" ;;
    "Backup /opt/despatch/.env before removal?") echo "DESPATCH_BACKUP_ENV" ;;
    "Backup /var/lib/despatch before removal?") echo "DESPATCH_BACKUP_DATA" ;;
    "Remove installed app files from /opt/despatch ?") echo "DESPATCH_REMOVE_APP_FILES" ;;
    "Remove app data from /var/lib/despatch ?") echo "DESPATCH_REMOVE_APP_DATA" ;;
    "Remove system user 'despatch'?") echo "DESPATCH_REMOVE_SYSTEM_USER" ;;
    "Remove despatch reverse-proxy site config from Nginx (if present)?") echo "DESPATCH_REMOVE_NGINX_SITE" ;;
    "Remove despatch reverse-proxy site config from Apache2 (if present)?") echo "DESPATCH_REMOVE_APACHE_SITE" ;;
    "Remove standalone installer checkout /opt/despatch-installer ?") echo "DESPATCH_REMOVE_CHECKOUT" ;;
    *) echo "" ;;
  esac
}

prompt_yes_no() {
  local prompt="$1" default_yes="$2" ans
  local hint="[y/N]"
  if [[ "$DESPATCH_NONINTERACTIVE" == "1" ]]; then
    local key env_val
    key="$(env_key_for_prompt "$prompt")"
    env_val=""
    if [[ -n "$key" ]]; then
      env_val="${!key:-}"
    fi
    if [[ -n "$env_val" ]]; then
      if truthy "$env_val"; then
        log "[non-interactive] ${prompt}: yes"
        return 0
      fi
      log "[non-interactive] ${prompt}: no"
      return 1
    fi
    if [[ "$default_yes" == "1" ]]; then
      log "[non-interactive] ${prompt}: yes (default)"
      return 0
    fi
    log "[non-interactive] ${prompt}: no (default)"
    return 1
  fi
  if [[ "$default_yes" == "1" ]]; then
    hint="[Y/n]"
  fi
  while true; do
    read -r -p "$prompt $hint " ans
    ans="$(lower "$(trim "$ans")")"
    if [[ -z "$ans" ]]; then
      [[ "$default_yes" == "1" ]] && return 0
      return 1
    fi
    case "$ans" in
      y|yes) return 0 ;;
      n|no) return 1 ;;
    esac
  done
}

timestamp() {
  date +%Y%m%d-%H%M%S
}

backup_path() {
  local path="$1"
  local dst="/var/backups/despatch/$(basename "$path").$(timestamp)"
  run_as_root mkdir -p /var/backups/despatch
  if [[ -d "$path" ]]; then
    run_as_root cp -a "$path" "$dst"
  else
    run_as_root cp -a "$path" "$dst"
  fi
  log "Backup created: $dst"
}

safe_systemctl() {
  local action="$1" unit="$2"
  if have_cmd systemctl; then
    run_as_root systemctl "$action" "$unit" >/dev/null 2>&1 || true
  fi
}

teardown_systemd_prefix() {
  local prefix="$1"
  local units=(
    "${prefix}-pam-reset-helper.socket"
    "${prefix}-pam-reset-helper.service"
    "${prefix}-updater.path"
    "${prefix}-updater.service"
    "${prefix}-mailsec"
    "${prefix}"
  )
  local unit
  for unit in "${units[@]}"; do
    safe_systemctl stop "$unit"
    safe_systemctl disable "$unit"
  done

  local files=(
    "/etc/systemd/system/${prefix}-updater.path"
    "/etc/systemd/system/${prefix}-updater.service"
    "/etc/systemd/system/${prefix}-pam-reset-helper.socket"
    "/etc/systemd/system/${prefix}-pam-reset-helper.service"
    "/etc/systemd/system/${prefix}-mailsec.service"
    "/etc/systemd/system/${prefix}.service"
  )
  local f
  for f in "${files[@]}"; do
    if [[ -f "$f" ]]; then
      run_as_root rm -f "$f"
    fi
  done

  local dropins=(
    "/etc/systemd/system/${prefix}-pam-reset-helper.service.d"
    "/etc/systemd/system/${prefix}-mailsec.service.d"
    "/etc/systemd/system/${prefix}-updater.service.d"
    "/etc/systemd/system/${prefix}.service.d"
  )
  local d
  for d in "${dropins[@]}"; do
    if [[ -d "$d" ]]; then
      run_as_root rm -rf "$d"
    fi
  done
}

cleanup_nginx_proxy() {
  local conf="/etc/nginx/sites-available/despatch.conf"
  local enabled="/etc/nginx/sites-enabled/despatch.conf"

  if [[ ! -f "$conf" && ! -L "$enabled" ]]; then
    warn "Nginx despatch site config not found; skipping."
    return
  fi

  run_as_root rm -f "$enabled" "$conf"
  if have_cmd nginx; then
    run_as_root nginx -t
    safe_systemctl reload nginx
  fi
  log "Removed despatch nginx site config."
}

cleanup_apache_proxy() {
  local conf="/etc/apache2/sites-available/despatch.conf"
  if [[ ! -f "$conf" ]]; then
    warn "Apache2 despatch site config not found; skipping."
    return
  fi

  if have_cmd a2dissite; then
    run_as_root a2dissite despatch.conf >/dev/null 2>&1 || true
  fi
  run_as_root rm -f "$conf"
  if have_cmd apache2ctl; then
    run_as_root apache2ctl configtest
    safe_systemctl reload apache2
  fi
  log "Removed despatch apache2 site config."
}

remove_path_if_exists() {
  local p="$1"
  if [[ -e "$p" ]]; then
    run_as_root rm -rf "$p"
    log "Removed: $p"
  else
    warn "Not found: $p"
  fi
}

remove_despatch_user() {
  if id -u despatch >/dev/null 2>&1; then
    run_as_root userdel despatch >/dev/null 2>&1 || true
    log "Removed system user: despatch"
  else
    warn "System user despatch not found; skipping."
  fi
}

log "Despatch interactive uninstaller"
warn "This removes only Despatch-managed artifacts."
warn "It does NOT remove Postfix, Dovecot, Nginx, Apache2, databases, or TLS packages."

if ! prompt_yes_no "Continue with uninstall?" 0; then
  err "Aborted"
  emit_run_result_once "failed" "preflight" "1"
  exit 1
fi

emit_event "run_start" "run_id" "$DESPATCH_RUN_ID" "operation" "uninstall"
begin_stage "preflight" "Preflight" "10"
finish_stage_ok

begin_stage "backups" "Backup Selected Files" "20"
if [[ -f /opt/despatch/.env ]] && prompt_yes_no "Backup /opt/despatch/.env before removal?" 1; then
  backup_path /opt/despatch/.env
fi

if [[ -d /var/lib/despatch ]] && prompt_yes_no "Backup /var/lib/despatch before removal?" 1; then
  backup_path /var/lib/despatch
fi
finish_stage_ok

begin_stage "service" "Service Teardown" "25"
log "Removing service files..."
if have_cmd systemctl; then
  teardown_systemd_prefix "despatch"
  teardown_systemd_prefix "mailclient"
  run_as_root systemctl daemon-reload
  run_as_root systemctl reset-failed \
    despatch despatch-mailsec despatch-updater.service despatch-updater.path despatch-pam-reset-helper.service despatch-pam-reset-helper.socket \
    mailclient mailclient-mailsec mailclient-updater.service mailclient-updater.path mailclient-pam-reset-helper.service mailclient-pam-reset-helper.socket \
    >/dev/null 2>&1 || true
else
  warn "systemctl not found; skipping systemd removal."
fi
finish_stage_ok

begin_stage "cleanup" "Filesystem Cleanup" "35"
if prompt_yes_no "Remove installed app files from /opt/despatch ?" 1; then
  remove_path_if_exists /opt/despatch
fi

if prompt_yes_no "Remove app data from /var/lib/despatch ?" 1; then
  remove_path_if_exists /var/lib/despatch
fi

if prompt_yes_no "Remove system user 'despatch'?" 1; then
  remove_despatch_user
fi

if prompt_yes_no "Remove despatch reverse-proxy site config from Nginx (if present)?" 1; then
  cleanup_nginx_proxy
fi

if prompt_yes_no "Remove despatch reverse-proxy site config from Apache2 (if present)?" 1; then
  cleanup_apache_proxy
fi

if [[ -d /opt/despatch-installer ]] && prompt_yes_no "Remove standalone installer checkout /opt/despatch-installer ?" 0; then
  remove_path_if_exists /opt/despatch-installer
fi
finish_stage_ok

begin_stage "summary" "Final Summary" "10"
cat <<DONE

Uninstall complete.

Not touched:
  - Postfix services/config
  - Dovecot services/config
  - Nginx/Apache2 packages (only optional despatch site entries)
  - Dovecot auth database
  - TLS/certbot packages and certificates

DONE
finish_stage_ok
emit_run_result_once "ok" "" "0"
