#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_EXAMPLE="$ROOT_DIR/.env.example"
OUT_ENV="$ROOT_DIR/.env"
DEFAULT_REPO_URL="${DESPATCH_REPO_URL:-https://github.com/2high4schooltoday/despatch.git}"
DEFAULT_REPO_REF="${DESPATCH_REPO_REF:-main}"
DEFAULT_CAP_IMAGE="${DESPATCH_CAPTCHA_CAP_IMAGE_DEFAULT:-tiagozip/cap:latest}"
DEFAULT_CAP_WIDGET_VERSION="${DESPATCH_CAPTCHA_DEFAULT_WIDGET_VERSION:-0.0.6}"
DEFAULT_CAP_WASM_VERSION="${DESPATCH_CAPTCHA_DEFAULT_WASM_VERSION:-0.0.6}"
APT_UPDATED=0
DESPATCH_NONINTERACTIVE="${DESPATCH_NONINTERACTIVE:-0}"
DESPATCH_TUI_MODE="${DESPATCH_TUI_MODE:-0}"
DESPATCH_RUN_ID="${DESPATCH_RUN_ID:-run-$(date +%s)}"
DESPATCH_BOOTSTRAPPED="${DESPATCH_BOOTSTRAPPED:-0}"
CURRENT_STAGE_ID=""
CURRENT_STAGE_TITLE=""
CURRENT_STAGE_WEIGHT=0
RUN_RESULT_EMITTED=0
# Prefer rustup-managed toolchain when present (avoids old distro cargo/rustc).
export PATH="$HOME/.cargo/bin:$PATH"

log() { printf '[INFO] %s\n' "$*"; }
warn() { printf '[WARN] %s\n' "$*" >&2; }
err() { printf '[ERR ] %s\n' "$*" >&2; }
ni_log() { printf '[INFO] %s\n' "$*" >&2; }
INSTALL_ERROR_REPORTED=0
CURRENT_STEP="bootstrap"
LAST_COMMAND=""
step() {
  CURRENT_STEP="$*"
  log "==> $*"
}

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

emit_event_raw() {
  [[ "$DESPATCH_TUI_MODE" == "1" ]] || return 0
  printf '::despatch-event::%s\n' "$1"
}

emit_event() {
  local type="$1"
  shift
  local body=""
  body="\"type\":\"$(json_escape "$type")\""
  while [[ $# -gt 1 ]]; do
    local k="$1"
    local v="$2"
    shift 2
    body="${body},\"$(json_escape "$k")\":\"$(json_escape "$v")\""
  done
  emit_event_raw "{${body}}"
}

begin_stage() {
  local stage_id="$1" title="$2" weight="$3"
  if [[ -n "$CURRENT_STAGE_ID" ]]; then
    emit_event "stage_progress" "stage_id" "$CURRENT_STAGE_ID" "current" "1" "total" "1" "message" "done"
    emit_event "stage_result" "stage_id" "$CURRENT_STAGE_ID" "status" "ok" "error_code" ""
  fi
  CURRENT_STAGE_ID="$stage_id"
  CURRENT_STAGE_TITLE="$title"
  CURRENT_STAGE_WEIGHT="$weight"
  step "$title"
  emit_event "stage_start" "stage_id" "$stage_id" "title" "$title" "weight" "$weight"
  emit_event "stage_progress" "stage_id" "$stage_id" "current" "0" "total" "1" "message" "started"
}

finish_stage_ok() {
  if [[ -z "$CURRENT_STAGE_ID" ]]; then
    return
  fi
  emit_event "stage_progress" "stage_id" "$CURRENT_STAGE_ID" "current" "1" "total" "1" "message" "done"
  emit_event "stage_result" "stage_id" "$CURRENT_STAGE_ID" "status" "ok" "error_code" ""
  CURRENT_STAGE_ID=""
  CURRENT_STAGE_TITLE=""
  CURRENT_STAGE_WEIGHT=0
}

finish_stage_failed() {
  local code="$1"
  if [[ -z "$CURRENT_STAGE_ID" ]]; then
    return
  fi
  emit_event "stage_result" "stage_id" "$CURRENT_STAGE_ID" "status" "failed" "error_code" "$code"
}

emit_run_result_once() {
  local status="$1" failed_stage="$2" exit_code="$3"
  if [[ "$RUN_RESULT_EMITTED" == "1" ]]; then
    return
  fi
  RUN_RESULT_EMITTED=1
  emit_event "run_result" "status" "$status" "failed_stage" "$failed_stage" "exit_code" "$exit_code"
}

on_install_error() {
  local code="$1" line="$2" cmd="$3"
  INSTALL_ERROR_REPORTED=1
  finish_stage_failed "E_INSTALL"
  err "Installer failed at line ${line}: ${cmd}"
  err "Current step: ${CURRENT_STEP}"
  err "Run manually for diagnostics: bash -x \"$0\""
  emit_run_result_once "failed" "${CURRENT_STAGE_ID:-unknown}" "$code"
  exit "$code"
}
trap 'on_install_error "$?" "$LINENO" "$BASH_COMMAND"' ERR
trap 'LAST_COMMAND=$BASH_COMMAND' DEBUG

on_install_exit() {
  local code="$1"
  if [[ "$code" -ne 0 && "$INSTALL_ERROR_REPORTED" -eq 0 ]]; then
    finish_stage_failed "E_INSTALL_EXIT"
    err "Installer exited with code ${code}."
    err "Current step: ${CURRENT_STEP}"
    err "Last command: ${LAST_COMMAND}"
    err "Run manually for diagnostics: bash -x \"$0\""
    emit_run_result_once "failed" "${CURRENT_STAGE_ID:-unknown}" "$code"
  fi
}
trap 'on_install_exit "$?"' EXIT

run_nonfatal_step() {
  local label="$1"
  shift
  local err_trap rc
  err_trap="$(trap -p ERR || true)"
  trap - ERR
  set +e
  "$@"
  rc=$?
  set -e
  if [[ -n "$err_trap" ]]; then
    eval "$err_trap"
  else
    trap - ERR
  fi
  if [[ "$rc" -ne 0 ]]; then
    warn "${label} returned non-fatal code ${rc}. Continuing."
  fi
  return 0
}

configure_selected_proxy() {
  local server="$1" server_name="$2" upstream="$3" tls_enabled="$4" cert_file="$5" key_file="$6" cap_upstream="${7:-}"
  case "$server" in
    nginx)
      setup_nginx_proxy "$server_name" "$upstream" "$tls_enabled" "$cert_file" "$key_file" "$cap_upstream"
      ;;
    apache2)
      setup_apache_proxy "$server_name" "$upstream" "$tls_enabled" "$cert_file" "$key_file" "$cap_upstream"
      ;;
    *)
      err "Unknown proxy server: $server"
      return 1
      ;;
  esac
}

if [[ $# -ne 0 ]]; then
  err "This installer does not accept CLI arguments. Use environment variables for non-interactive mode."
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

run_as_despatch() {
  if [[ "${EUID:-1}" -ne 0 ]]; then
    if ! have_cmd sudo; then
      err "sudo is required for despatch user verification"
      exit 1
    fi
    sudo -u despatch "$@"
    return
  fi
  if have_cmd runuser; then
    runuser -u despatch -- "$@"
    return
  fi
  if have_cmd sudo; then
    sudo -u despatch "$@"
    return
  fi
  if have_cmd su; then
    local quoted
    quoted="$(printf '%q ' "$@")"
    su -s /bin/sh -c "$quoted" despatch
    return
  fi
  err "cannot execute updater verification as despatch user (missing runuser/sudo/su)"
  exit 1
}

trim() {
  local s="$1"
  # shellcheck disable=SC2001
  s="$(echo "$s" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  printf '%s' "$s"
}

lower() {
  echo "$1" | tr '[:upper:]' '[:lower:]'
}

is_noninteractive() {
  [[ "$DESPATCH_NONINTERACTIVE" == "1" ]]
}

env_key_for_input_prompt() {
  local prompt="$1"
  case "$prompt" in
    "GitHub repository URL") echo "DESPATCH_REPO_URL" ;;
    "Git ref (branch/tag/commit)") echo "DESPATCH_REPO_REF" ;;
    "Installer workspace directory") echo "DESPATCH_CHECKOUT_DIR" ;;
    "Primary mail domain (used by first-run web setup)") echo "DESPATCH_BASE_DOMAIN" ;;
    "HTTP listen address") echo "DESPATCH_LISTEN_ADDR" ;;
    "Choose proxy server: nginx or apache2") echo "DESPATCH_PROXY_SERVER" ;;
    "Public server name for reverse proxy") echo "DESPATCH_PROXY_SERVER_NAME" ;;
    "TLS certificate file") echo "DESPATCH_PROXY_CERT" ;;
    "TLS private key file") echo "DESPATCH_PROXY_KEY" ;;
    "CAP site key") echo "DESPATCH_CAPTCHA_SITE_KEY" ;;
    "CAP verification secret") echo "DESPATCH_CAPTCHA_SECRET" ;;
    "CAP upstream host:port for reverse proxy /cap/ route") echo "DESPATCH_CAPTCHA_CAP_UPSTREAM" ;;
    "CAP ENABLE_ASSETS_SERVER value (true/false)") echo "DESPATCH_CAPTCHA_CAP_ENABLE_ASSETS_SERVER" ;;
    "CAP WIDGET_VERSION (pin, avoid latest)") echo "DESPATCH_CAPTCHA_CAP_WIDGET_VERSION" ;;
    "CAP WASM_VERSION (pin, avoid latest)") echo "DESPATCH_CAPTCHA_CAP_WASM_VERSION" ;;
    "CAP image (Docker)") echo "DESPATCH_CAPTCHA_CAP_IMAGE" ;;
    "CAP admin key (for CAP dashboard access)") echo "DESPATCH_CAPTCHA_CAP_ADMIN_KEY" ;;
    "Dovecot auth backend mode (pam or sql)") echo "DESPATCH_DOVECOT_AUTH_MODE" ;;
    "Dovecot auth DB driver (mysql or pgx)") echo "DESPATCH_DOVECOT_AUTH_DB_DRIVER" ;;
    "Dovecot auth DB DSN") echo "DESPATCH_DOVECOT_AUTH_DB_DSN" ;;
    "Dovecot auth table") echo "DESPATCH_DOVECOT_AUTH_TABLE" ;;
    "Email/login column") echo "DESPATCH_DOVECOT_AUTH_EMAIL_COL" ;;
    "Password hash column") echo "DESPATCH_DOVECOT_AUTH_PASS_COL" ;;
    "Active/enabled column (blank if none)") echo "DESPATCH_DOVECOT_AUTH_ACTIVE_COL" ;;
    "Maildir column (blank if none)") echo "DESPATCH_DOVECOT_AUTH_MAILDIR_COL" ;;
    *) echo "" ;;
  esac
}

env_key_for_yes_no_prompt() {
  local prompt="$1"
  case "$prompt" in
    "Continue with installation?") echo "DESPATCH_CONFIRM_CONTINUE" ;;
    "git is missing. Install git automatically?") echo "DESPATCH_INSTALL_GIT" ;;
    "Install and start systemd service automatically?") echo "DESPATCH_INSTALL_SERVICE" ;;
    "Detected installed reverse proxy"*) echo "DESPATCH_PROXY_SETUP" ;;
    "Install nginx automatically now?") echo "DESPATCH_INSTALL_NGINX" ;;
    "Install apache2 automatically now?") echo "DESPATCH_INSTALL_APACHE2" ;;
    "Install Docker automatically now?") echo "DESPATCH_INSTALL_DOCKER" ;;
    "Continue without reverse proxy (direct mode on :8080)?") echo "DESPATCH_PROXY_FALLBACK_DIRECT" ;;
    "Enable TLS in reverse proxy config now (requires existing cert files)?") echo "DESPATCH_PROXY_TLS" ;;
    "Enable self-hosted CAP captcha for registration now?") echo "DESPATCH_CAPTCHA_ENABLE_CAP" ;;
    "Auto-provision CAP standalone runtime with Docker?") echo "DESPATCH_CAPTCHA_AUTO_PROVISION" ;;
    "Re-enter TLS paths?") echo "DESPATCH_RETRY_TLS_PATHS" ;;
    "Enter Dovecot SQL settings manually now?") echo "DESPATCH_SQL_MANUAL" ;;
    "Install missing dependencies automatically with apt?") echo "DESPATCH_AUTO_INSTALL_DEPS" ;;
    "Enable ufw now? (be careful on remote SSH hosts)") echo "DESPATCH_UFW_ENABLE" ;;
    "Open firewall ports 80/tcp and 443/tcp via ufw?") echo "DESPATCH_UFW_OPEN_PROXY_PORTS" ;;
    "Open firewall port 8080/tcp via ufw for direct access?") echo "DESPATCH_UFW_OPEN_DIRECT_PORT" ;;
    "Run Internet accessibility diagnostics now?") echo "DESPATCH_RUN_DIAG" ;;
    *) echo "" ;;
  esac
}

truthy() {
  local v
  v="$(lower "$(trim "${1:-}")")"
  case "$v" in
    1|y|yes|true|on) return 0 ;;
  esac
  return 1
}

prompt_input() {
  local prompt="$1" default="${2:-}" val
  if is_noninteractive; then
    local key env_val
    key="$(env_key_for_input_prompt "$prompt")"
    if [[ -n "$key" ]]; then
      env_val="$(trim "${!key:-}")"
      if [[ -n "$env_val" ]]; then
        ni_log "[non-interactive] ${prompt}: ${env_val}"
        printf '%s' "$env_val"
        return
      fi
    fi
    if [[ -n "$default" ]]; then
      ni_log "[non-interactive] ${prompt}: ${default} (default)"
      printf '%s' "$default"
      return
    fi
    err "Missing non-interactive value for prompt: ${prompt}"
    exit 1
  fi
  if [[ -n "$default" ]]; then
    if ! read -r -p "$prompt [$default]: " val; then
      warn "Input stream closed. Using default: $default"
      printf '%s' "$default"
      return
    fi
    if [[ -z "$(trim "$val")" ]]; then
      printf '%s' "$default"
      return
    fi
    printf '%s' "$(trim "$val")"
    return
  fi
  while true; do
    if ! read -r -p "$prompt: " val; then
      err "Input stream closed while waiting for: $prompt"
      exit 1
    fi
    val="$(trim "$val")"
    if [[ -n "$val" ]]; then
      printf '%s' "$val"
      return
    fi
  done
}

prompt_yes_no() {
  local prompt="$1" default_yes="$2" ans
  local hint="[y/N]"
  if is_noninteractive; then
    local key env_val
    key="$(env_key_for_yes_no_prompt "$prompt")"
    env_val=""
    if [[ -n "$key" ]]; then
      env_val="${!key:-}"
    fi
    if [[ -n "$env_val" ]]; then
      if truthy "$env_val"; then
        ni_log "[non-interactive] ${prompt}: yes"
        return 0
      fi
      ni_log "[non-interactive] ${prompt}: no"
      return 1
    fi
    if [[ "$default_yes" == "1" ]]; then
      ni_log "[non-interactive] ${prompt}: yes (default)"
      return 0
    fi
    ni_log "[non-interactive] ${prompt}: no (default)"
    return 1
  fi
  if [[ "$default_yes" == "1" ]]; then
    hint="[Y/n]"
  fi
  while true; do
    if ! read -r -p "$prompt $hint " ans; then
      warn "Input stream closed for prompt '$prompt'."
      [[ "$default_yes" == "1" ]] && return 0
      return 1
    fi
    ans="$(lower "$(trim "$ans")")"
    if [[ -z "$ans" ]]; then
      [[ "$default_yes" == "1" ]] && return 0
      return 1
    fi
    case "$ans" in
      y|yes) return 0 ;;
      n|no) return 1 ;;
      *) warn "Please answer y or n." ;;
    esac
  done
}

install_apt_packages() {
  if ! have_cmd apt-get; then
    return 1
  fi
  if [[ "$APT_UPDATED" -eq 0 ]]; then
    run_as_root apt-get update
    APT_UPDATED=1
  fi
  run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y "$@"
}

version_ge() {
  local actual expected
  actual="$(trim "${1:-}")"
  expected="$(trim "${2:-}")"
  if [[ -z "$actual" || -z "$expected" ]]; then
    return 1
  fi
  [[ "$(printf '%s\n%s\n' "$expected" "$actual" | sort -V | head -n1)" == "$expected" ]]
}

ensure_rust_toolchain() {
  local target="$1"
  local min_version="1.74.0"
  local rustc_ver=""
  local need_bootstrap=0

  if have_cmd rustc; then
    rustc_ver="$(rustc --version 2>/dev/null | awk '{print $2}')"
  fi

  if ! have_cmd rustup; then
    need_bootstrap=1
  fi
  if [[ -z "$rustc_ver" ]] || ! version_ge "$rustc_ver" "$min_version"; then
    need_bootstrap=1
  fi

  if [[ "$need_bootstrap" -eq 1 ]]; then
    warn "Bootstrapping Rust toolchain (stable >= ${min_version}) for privileged binaries"
    if [[ "$OS_ID" == "ubuntu" || "$OS_ID" == "debian" ]]; then
      # openssl-sys vendored path and Rust bootstrap dependencies
      install_apt_packages ca-certificates curl build-essential pkg-config perl cmake || true
    fi
    if ! have_cmd curl; then
      err "curl is required to bootstrap rustup toolchain"
      return 1
    fi
    curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
      | sh -s -- -y --profile minimal --default-toolchain stable
    export PATH="$HOME/.cargo/bin:$PATH"
  fi

  if ! have_cmd rustup; then
    err "rustup is required to install Rust targets for privileged binaries"
    return 1
  fi
  rustup default stable >/dev/null 2>&1 || true
  rustup target add "$target" >/dev/null 2>&1 || true

  case "$target" in
    aarch64-unknown-linux-gnu)
      if [[ "$UNAME_ARCH" == "x86_64" || "$UNAME_ARCH" == "amd64" ]]; then
        if [[ "$OS_ID" == "ubuntu" || "$OS_ID" == "debian" ]]; then
          install_apt_packages gcc-aarch64-linux-gnu || true
        fi
        if have_cmd aarch64-linux-gnu-gcc; then
          export CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER="aarch64-linux-gnu-gcc"
        fi
      fi
      ;;
    x86_64-unknown-linux-gnu)
      if [[ "$UNAME_ARCH" == "aarch64" || "$UNAME_ARCH" == "arm64" ]]; then
        if [[ "$OS_ID" == "ubuntu" || "$OS_ID" == "debian" ]]; then
          install_apt_packages gcc-x86-64-linux-gnu || true
        fi
        if have_cmd x86_64-linux-gnu-gcc; then
          export CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_LINKER="x86_64-linux-gnu-gcc"
        fi
      fi
      ;;
  esac
}

ensure_repo_checkout_or_bootstrap() {
  if [[ -f "$ENV_EXAMPLE" && -f "$ROOT_DIR/go.mod" && -f "$ROOT_DIR/cmd/server/main.go" ]]; then
    return
  fi
  if [[ "$DESPATCH_BOOTSTRAPPED" == "1" ]]; then
    err "Bootstrap relaunch already attempted, but source files are still missing."
    err "Check repository URL/ref and permissions for installer workspace."
    exit 1
  fi

  warn "App source files were not found next to this script."
  warn "Switching to standalone bootstrap mode (fetch from GitHub)."

  local uname_sys repo_url repo_ref checkout_base checkout_dir target
  uname_sys="$(lower "$(uname -s)")"
  if [[ "$uname_sys" != "linux" ]]; then
    err "Standalone bootstrap currently supports Linux servers only."
    exit 1
  fi

  repo_url="$(prompt_input "GitHub repository URL" "$DEFAULT_REPO_URL")"
  repo_ref="$(prompt_input "Git ref (branch/tag/commit)" "$DEFAULT_REPO_REF")"
  checkout_base="$(prompt_input "Installer workspace directory" "/opt/despatch-installer")"
  checkout_dir="${checkout_base%/}/src"

  if ! have_cmd git; then
    if prompt_yes_no "git is missing. Install git automatically?" 1; then
      install_apt_packages git ca-certificates curl
    else
      err "git is required to download source files."
      exit 1
    fi
  fi

  run_as_root mkdir -p "$checkout_base"
  if [[ -d "$checkout_dir/.git" ]]; then
    run_as_root git -C "$checkout_dir" fetch --tags --prune origin
  else
    run_as_root rm -rf "$checkout_dir"
    run_as_root git clone "$repo_url" "$checkout_dir"
  fi
  run_as_root git -C "$checkout_dir" checkout "$repo_ref"
  run_as_root git -C "$checkout_dir" pull --ff-only origin "$repo_ref" || true

  target="$checkout_dir/scripts/auto_install.sh"
  if [[ ! -f "$target" ]]; then
    err "Downloaded repository does not contain scripts/auto_install.sh"
    exit 1
  fi

  log "Re-launching installer from: $checkout_dir"
  local -a relaunch_env=()
  local env_line env_key env_val
  while IFS= read -r env_line; do
    env_key="${env_line%%=*}"
    env_val="${env_line#*=}"
    relaunch_env+=("${env_key}=${env_val}")
  done < <(env | grep -E '^DESPATCH_[A-Za-z0-9_]*=' || true)
  relaunch_env+=("DESPATCH_BOOTSTRAPPED=1")
  relaunch_env+=("DESPATCH_REPO_URL=${repo_url}")
  relaunch_env+=("DESPATCH_REPO_REF=${repo_ref}")
  run_as_root env "${relaunch_env[@]}" bash "$target"
  exit 0
}

port_open() {
  local host="$1" port="$2"
  if have_cmd nc; then
    nc -z -w 1 "$host" "$port" >/dev/null 2>&1
    return $?
  fi
  (echo >"/dev/tcp/$host/$port") >/dev/null 2>&1
  return $?
}

set_env_var() {
  local file="$1" key="$2" value="$3"
  local tmp
  tmp="${file}.tmp.$$"

  if grep -q "^${key}=" "$file" 2>/dev/null; then
    awk -v k="$key" -v v="$value" '
      BEGIN { done = 0 }
      {
        if ($0 ~ "^" k "=") {
          print k "=" v
          done = 1
        } else {
          print $0
        }
      }
      END {
        if (!done) print k "=" v
      }
    ' "$file" >"$tmp"
  else
    cat "$file" >"$tmp"
    printf '%s=%s\n' "$key" "$value" >>"$tmp"
  fi
  mv "$tmp" "$file"
}

extract_kv_line() {
  local file="$1" key="$2"
  local line
  line="$(grep -E "^[[:space:]]*${key}[[:space:]]*=" "$file" 2>/dev/null | tail -n 1 || true)"
  line="${line#*=}"
  trim "$line"
}

extract_table_from_query() {
  local q="$1"
  awk '
    {
      for (i = 1; i <= NF; i++) {
        t = tolower($i)
        gsub(/[;,]/, "", t)
        if (t == "from" && i + 1 <= NF) {
          v = $(i+1)
          gsub(/[;,]/, "", v)
          print v
          exit
        }
      }
    }
  ' <<<"$q"
}

extract_email_col_from_query() {
  local q="$1"
  local m
  m="$(echo "$q" | grep -Eo "[A-Za-z0-9_.\"\`]+[[:space:]]*=[[:space:]]*'?%[A-Za-z]+'?" | head -n 1 || true)"
  if [[ -n "$m" ]]; then
    echo "$m" | sed -E "s/[[:space:]]*=[[:space:]]*.*$//"
  fi
}

extract_password_col_from_query() {
  local q="$1"
  local select_part alias_hit token
  select_part="$(echo "$q" | sed -E 's/^[[:space:]]*[Ss][Ee][Ll][Ee][Cc][Tt][[:space:]]+//; s/[[:space:]]+[Ff][Rr][Oo][Mm][[:space:]].*$//')"

  alias_hit="$(echo "$select_part" | sed -nE "s/.*([A-Za-z0-9_.\"\`]+)[[:space:]]+[Aa][Ss][[:space:]]+password.*/\1/p" | head -n 1 || true)"
  if [[ -n "$alias_hit" ]]; then
    echo "$alias_hit"
    return
  fi

  while IFS= read -r token; do
    token="$(trim "$token")"
    if [[ -z "$token" ]]; then
      continue
    fi
    if echo "$(lower "$token")" | grep -q "password"; then
      echo "$token" | awk '{print $1}'
      return
    fi
    if echo "$(lower "$token")" | grep -q "pass"; then
      echo "$token" | awk '{print $1}'
      return
    fi
  done < <(echo "$select_part" | tr ',' '\n')
}

extract_maildir_col_from_query() {
  local q="$1"
  local select_part alias_hit token l
  select_part="$(echo "$q" | sed -E 's/^[[:space:]]*[Ss][Ee][Ll][Ee][Cc][Tt][[:space:]]+//; s/[[:space:]]+[Ff][Rr][Oo][Mm][[:space:]].*$//')"

  alias_hit="$(echo "$select_part" | sed -nE "s/.*([A-Za-z0-9_.\"\`]+)[[:space:]]+[Aa][Ss][[:space:]]+(maildir|mail|home).*/\1/p" | head -n 1 || true)"
  if [[ -n "$alias_hit" ]]; then
    echo "$alias_hit"
    return
  fi

  while IFS= read -r token; do
    token="$(trim "$token")"
    l="$(lower "$token")"
    if echo "$l" | grep -Eq "maildir|\bhome\b|\bmail\b"; then
      echo "$token" | awk '{print $1}'
      return
    fi
  done < <(echo "$select_part" | tr ',' '\n')
}

extract_active_col_from_query() {
  local q="$1"
  local maybe
  maybe="$(echo "$q" | grep -Eo "[A-Za-z0-9_.\"\`]+[[:space:]]*=[[:space:]]*(1|true|'[YyTt]'|'active'|[Aa][Cc][Tt][Ii][Vv][Ee])" | head -n 1 || true)"
  if [[ -n "$maybe" ]]; then
    echo "$maybe" | sed -E "s/[[:space:]]*=[[:space:]]*.*$//"
  fi
}

parse_connect_kv() {
  local connect="$1" key="$2"
  local tok k v
  local IFS=' '
  for tok in $connect; do
    k="${tok%%=*}"
    v="${tok#*=}"
    if [[ "$k" == "$key" ]]; then
      printf '%s' "$v"
      return
    fi
  done
}

build_dsn() {
  local driver="$1" connect="$2"
  local host port user pass db ssl
  host="$(parse_connect_kv "$connect" host)"
  port="$(parse_connect_kv "$connect" port)"
  user="$(parse_connect_kv "$connect" user)"
  pass="$(parse_connect_kv "$connect" password)"
  db="$(parse_connect_kv "$connect" dbname)"
  ssl="$(parse_connect_kv "$connect" sslmode)"

  if [[ "$driver" == "mysql" ]]; then
    [[ -z "$host" ]] && host="127.0.0.1"
    [[ -z "$port" ]] && port="3306"
    if [[ -z "$db" || -z "$user" ]]; then
      printf ''
      return
    fi
    if [[ "$host" == /* ]]; then
      printf '%s' "${user}:${pass}@unix(${host})/${db}?parseTime=true"
    else
      printf '%s' "${user}:${pass}@tcp(${host}:${port})/${db}?parseTime=true"
    fi
    return
  fi

  if [[ "$driver" == "pgx" ]]; then
    [[ -z "$host" ]] && host="127.0.0.1"
    [[ -z "$port" ]] && port="5432"
    [[ -z "$ssl" ]] && ssl="disable"
    if [[ -z "$db" || -z "$user" ]]; then
      printf ''
      return
    fi
    printf '%s' "postgres://${user}:${pass}@${host}:${port}/${db}?sslmode=${ssl}"
    return
  fi

  printf ''
}

detect_sql_conf_file() {
  if [[ -n "${DOVECOT_SQL_CONF:-}" && -f "${DOVECOT_SQL_CONF}" ]]; then
    printf '%s' "$DOVECOT_SQL_CONF"
    return
  fi

  local candidates out p
  candidates=""

  if have_cmd doveconf; then
    out="$(doveconf -n 2>/dev/null || true)"
    if [[ -z "$out" && ${EUID:-1} -ne 0 ]] && have_cmd sudo; then
      out="$(sudo -n doveconf -n 2>/dev/null || true)"
    fi
    if [[ -n "$out" ]]; then
      while IFS= read -r p; do
        p="$(trim "$p")"
        [[ -z "$p" ]] && continue
        candidates+="$p\n"
      done < <(printf '%s\n' "$out" | sed -nE 's/^[[:space:]]*args[[:space:]]*=[[:space:]]*(.*dovecot-sql[^[:space:]]*).*/\1/p')
    fi
  fi

  candidates+="/etc/dovecot/dovecot-sql.conf.ext\n"
  candidates+="/usr/local/etc/dovecot/dovecot-sql.conf.ext\n"

  while IFS= read -r p; do
    [[ -z "$p" ]] && continue
    if [[ -f "$p" ]]; then
      printf '%s' "$p"
      return
    fi
  done <<<"$(printf "%b" "$candidates")"

  printf ''
}

detect_dovecot_auth_mode() {
  local out=""
  if have_cmd doveconf; then
    out="$(doveconf -n 2>/dev/null || true)"
    if [[ -z "$out" && ${EUID:-1} -ne 0 ]] && have_cmd sudo; then
      out="$(sudo -n doveconf -n 2>/dev/null || true)"
    fi
  fi

  if [[ -n "$out" ]] && echo "$out" | grep -Eiq 'driver[[:space:]]*=[[:space:]]*pam'; then
    printf 'pam'
    return
  fi

  if grep -RqsE 'auth-system\.conf\.ext|driver[[:space:]]*=[[:space:]]*pam' /etc/dovecot /usr/local/etc/dovecot 2>/dev/null; then
    printf 'pam'
    return
  fi

  printf 'sql'
}

detect_imap_port() {
  if port_open 127.0.0.1 993; then
    echo 993
    return
  fi
  if port_open 127.0.0.1 143; then
    echo 143
    return
  fi
  echo 993
}

detect_smtp_port() {
  if port_open 127.0.0.1 25; then
    echo 25
    return
  fi
  if port_open 127.0.0.1 587; then
    echo 587
    return
  fi
  if port_open 127.0.0.1 465; then
    echo 465
    return
  fi
  echo 25
}

derive_password_reset_sender_domain() {
  local domain
  domain="$(lower "$(trim "${1:-}")")"
  if [[ -z "$domain" ]]; then
    printf 'example.com'
    return
  fi
  case "$domain" in
    mail.*)
      printf '%s' "${domain#mail.}"
      return
      ;;
  esac
  printf '%s' "$domain"
}

derive_password_reset_base_url() {
  local deploy_mode="$1" proxy_server_name="$2" proxy_tls="$3" base_domain="$4" listen_addr="$5"
  local host="" port=""

  if [[ "$deploy_mode" == "proxy" && -n "$(trim "$proxy_server_name")" ]]; then
    if [[ "$proxy_tls" == "1" ]]; then
      printf 'https://%s' "$(trim "$proxy_server_name")"
      return
    fi
    printf 'http://%s' "$(trim "$proxy_server_name")"
    return
  fi

  if [[ "$listen_addr" == :* ]]; then
    host="$base_domain"
    port="${listen_addr#:}"
  elif [[ "$listen_addr" == *:* ]]; then
    host="${listen_addr%:*}"
    port="${listen_addr##*:}"
    host="${host#[}"
    host="${host%]}"
  else
    host="$base_domain"
    port="8080"
  fi

  host="$(trim "$host")"
  if [[ -z "$host" || "$host" == "127.0.0.1" || "$host" == "::1" || "$host" == "0.0.0.0" || "$host" == "::" ]]; then
    host="$(trim "$base_domain")"
  fi
  if [[ -z "$host" ]]; then
    host="127.0.0.1"
  fi
  if [[ -z "$port" || ! "$port" =~ ^[0-9]+$ ]]; then
    port="8080"
  fi

  http_base_url_from_host_port "$host" "$port"
}

detect_web_servers() {
  local found=()
  if have_cmd nginx; then
    found+=("nginx")
  fi
  if have_cmd apache2ctl || have_cmd apache2; then
    found+=("apache2")
  fi
  printf '%s\n' "${found[@]}"
}

is_loopback_host() {
  local host
  host="$(lower "$(trim "${1:-}")")"
  case "$host" in
    127.0.0.1|localhost|::1|"[::1]") return 0 ;;
  esac
  return 1
}

detect_letsencrypt_tls_pair() {
  local server_name="$1"
  local live_dir="/etc/letsencrypt/live"
  local cert key candidate walk
  declare -A seen=()
  local candidates=()

  server_name="$(lower "$(trim "${server_name:-}")")"
  [[ -d "$live_dir" ]] || return 1
  [[ -n "$server_name" ]] || return 1
  if [[ "$server_name" == \*.* ]]; then
    server_name="${server_name#*.}"
  fi

  candidates+=("$server_name")
  walk="$server_name"
  while [[ "$walk" == *.* ]]; do
    walk="${walk#*.}"
    candidates+=("$walk")
  done

  for candidate in "${candidates[@]}"; do
    [[ -n "$candidate" ]] || continue
    if [[ -n "${seen[$candidate]:-}" ]]; then
      continue
    fi
    seen[$candidate]=1
    cert="${live_dir}/${candidate}/fullchain.pem"
    key="${live_dir}/${candidate}/privkey.pem"
    if [[ -f "$cert" && -f "$key" ]]; then
      printf '%s\n%s\n' "$cert" "$key"
      return 0
    fi
  done

  for candidate in "$live_dir"/*; do
    [[ -d "$candidate" ]] || continue
    cert="${candidate}/fullchain.pem"
    key="${candidate}/privkey.pem"
    if [[ -f "$cert" && -f "$key" ]]; then
      printf '%s\n%s\n' "$cert" "$key"
      return 0
    fi
  done
  return 1
}

derive_imap_insecure_skip_verify() {
  local host="$1" imap_tls="$2" imap_starttls="$3"
  if is_loopback_host "$host" && { [[ "$imap_tls" == "true" ]] || [[ "$imap_starttls" == "true" ]]; }; then
    printf 'true'
    return
  fi
  printf 'false'
}

derive_smtp_insecure_skip_verify() {
  local host="$1" smtp_tls="$2" smtp_starttls="$3"
  if is_loopback_host "$host" && { [[ "$smtp_tls" == "true" ]] || [[ "$smtp_starttls" == "true" ]]; }; then
    printf 'true'
    return
  fi
  printf 'false'
}

ensure_proxy_tooling() {
  local server="$1" os_id="$2"
  case "$server" in
    nginx)
      if have_cmd nginx; then
        return 0
      fi
      if [[ "$os_id" == "ubuntu" || "$os_id" == "debian" ]]; then
        warn "nginx was selected but command is missing."
        if prompt_yes_no "Install nginx automatically now?" 1; then
          install_apt_packages nginx
          return 0
        fi
      fi
      err "nginx is required for selected proxy mode."
      return 1
      ;;
    apache2)
      if have_cmd apache2ctl && have_cmd a2enmod && have_cmd a2ensite; then
        return 0
      fi
      if [[ "$os_id" == "ubuntu" || "$os_id" == "debian" ]]; then
        warn "apache2 was selected but required commands are missing."
        if prompt_yes_no "Install apache2 automatically now?" 1; then
          install_apt_packages apache2
          return 0
        fi
      fi
      err "apache2 + a2enmod + a2ensite are required for selected proxy mode."
      return 1
      ;;
    *)
      err "unknown proxy server: $server"
      return 1
      ;;
  esac
}

wait_for_condition() {
  local label="$1" tries="$2" sleep_sec="$3"
  shift 3
  local i
  for ((i = 1; i <= tries; i++)); do
    if "$@"; then
      log "${label}: ok"
      return 0
    fi
    sleep "$sleep_sec"
  done
  warn "${label}: still failing after $((tries * sleep_sec))s"
  return 1
}

http_base_url_from_host_port() {
  local host="$1" port="$2"
  if [[ "$host" == *:* && "$host" != \[*\] ]]; then
    printf 'http://[%s]:%s' "$host" "$port"
    return
  fi
  printf 'http://%s:%s' "$host" "$port"
}

detect_docker_compose_mode() {
  if have_cmd docker && run_as_root docker compose version >/dev/null 2>&1; then
    printf 'plugin'
    return 0
  fi
  if have_cmd docker-compose; then
    printf 'legacy'
    return 0
  fi
  return 1
}

cap_compose_exec() {
  local compose_file="$1"
  shift
  local mode
  mode="$(detect_docker_compose_mode || true)"
  case "$mode" in
    plugin)
      run_as_root docker compose -f "$compose_file" "$@"
      ;;
    legacy)
      run_as_root docker-compose -f "$compose_file" "$@"
      ;;
    *)
      return 1
      ;;
  esac
}

cap_volume_has_data() {
  local volume="$1"
  local first
  first="$(run_as_root docker run --rm -v "${volume}:/data" alpine sh -lc 'ls -A /data 2>/dev/null | head -n1' 2>/dev/null || true)"
  [[ -n "$first" ]]
}

render_cap_compose() {
  local upstream="$1" image="$2" admin_key="$3" widget_version="$4" wasm_version="$5"
  local host="127.0.0.1" port="8077" bind_entry=""
  if [[ "$upstream" == *:* ]]; then
    host="$(trim "${upstream%:*}")"
    port="$(trim "${upstream##*:}")"
  else
    port="$(trim "$upstream")"
  fi
  [[ "$host" == "localhost" ]] && host="127.0.0.1"
  if [[ -z "$host" ]]; then
    host="127.0.0.1"
  fi
  if [[ -z "$port" || ! "$port" =~ ^[0-9]+$ ]]; then
    return 1
  fi
  case "$host" in
    127.0.0.1|0.0.0.0|::1)
      bind_entry="127.0.0.1:${port}:80"
      ;;
    *)
      bind_entry="${port}:80"
      warn "CAP upstream host '${host}' is not loopback; binding CAP on 0.0.0.0:${port}."
      ;;
  esac

  cat <<EOF
services:
  cap:
    image: ${image}
    container_name: cap
    restart: unless-stopped
    ports:
      - "${bind_entry}"
    environment:
      ADMIN_KEY: "${admin_key}"
      ENABLE_ASSETS_SERVER: "true"
      WIDGET_VERSION: "${widget_version}"
      WASM_VERSION: "${wasm_version}"
    volumes:
      - cap-data:/usr/src/app/.data

volumes:
  cap-data:
    name: cap-data
EOF
}

start_cap_runtime_direct() {
  local upstream="$1" image="$2" admin_key="$3" widget_version="$4" wasm_version="$5"
  local host="127.0.0.1" port="8077" publish=""
  if [[ "$upstream" == *:* ]]; then
    host="$(trim "${upstream%:*}")"
    port="$(trim "${upstream##*:}")"
  else
    port="$(trim "$upstream")"
  fi
  [[ "$host" == "localhost" ]] && host="127.0.0.1"
  if [[ -z "$host" ]]; then
    host="127.0.0.1"
  fi
  if [[ -z "$port" || ! "$port" =~ ^[0-9]+$ ]]; then
    return 1
  fi
  if [[ "$host" == "127.0.0.1" || "$host" == "::1" || "$host" == "0.0.0.0" ]]; then
    publish="127.0.0.1:${port}:80"
  else
    publish="${port}:80"
  fi
  run_as_root docker rm -f cap >/dev/null 2>&1 || true
  run_as_root docker volume create cap-data >/dev/null
  run_as_root docker run -d --name cap --restart unless-stopped \
    -p "$publish" \
    -e "ADMIN_KEY=${admin_key}" \
    -e "ENABLE_ASSETS_SERVER=true" \
    -e "WIDGET_VERSION=${widget_version}" \
    -e "WASM_VERSION=${wasm_version}" \
    -v cap-data:/usr/src/app/.data \
    "$image" >/dev/null
}

provision_cap_runtime() {
  local upstream="$1" image="$2" admin_key="$3" widget_version="$4" wasm_version="$5"
  local cap_dir="/opt/cap" compose_file="${cap_dir}/docker-compose.yml" env_file="${cap_dir}/.env" tmp=""

  if ! have_cmd docker; then
    err "Docker is required for CAP auto-provisioning but not found."
    return 1
  fi
  run_as_root mkdir -p "$cap_dir"

  if ! tmp="$(mktemp)"; then
    err "Failed to create temporary file for CAP compose."
    return 1
  fi
  if ! render_cap_compose "$upstream" "$image" "$admin_key" "$widget_version" "$wasm_version" >"$tmp"; then
    rm -f "$tmp"
    err "Invalid CAP upstream '${upstream}'. Expected host:port."
    return 1
  fi
  run_as_root install -m 0644 "$tmp" "$compose_file"
  rm -f "$tmp"

  if ! tmp="$(mktemp)"; then
    err "Failed to create temporary file for CAP env."
    return 1
  fi
  cat >"$tmp" <<EOF
CAP_IMAGE=$image
CAP_ADMIN_KEY=$admin_key
CAP_UPSTREAM=$upstream
CAP_WIDGET_VERSION=$widget_version
CAP_WASM_VERSION=$wasm_version
EOF
  run_as_root install -m 0600 "$tmp" "$env_file"
  rm -f "$tmp"

  run_as_root docker volume create cap-data >/dev/null
  if run_as_root docker volume inspect cap_cap-data >/dev/null 2>&1; then
    if cap_volume_has_data cap_cap-data && ! cap_volume_has_data cap-data; then
      warn "Detected legacy cap_cap-data volume with data and empty cap-data. Migrating to cap-data."
      run_as_root docker run --rm -v cap_cap-data:/from -v cap-data:/to alpine sh -lc 'cp -a /from/. /to/'
    elif cap_volume_has_data cap_cap-data && cap_volume_has_data cap-data; then
      warn "Detected both cap-data and cap_cap-data with data. Keeping cap-data as canonical volume."
    fi
  fi

  run_as_root docker rm -f cap >/dev/null 2>&1 || true
  local compose_started=0
  if detect_docker_compose_mode >/dev/null 2>&1; then
    cap_compose_exec "$compose_file" down --remove-orphans >/dev/null 2>&1 || true
    if cap_compose_exec "$compose_file" up -d --remove-orphans >/dev/null 2>&1; then
      compose_started=1
    fi
  fi
  if [[ "$compose_started" -ne 1 ]]; then
    warn "Docker compose path failed or unavailable; falling back to direct docker run for CAP."
    if ! start_cap_runtime_direct "$upstream" "$image" "$admin_key" "$widget_version" "$wasm_version"; then
      return 1
    fi
  fi

  local running
  running="$(run_as_root docker inspect -f '{{.State.Running}}' cap 2>/dev/null || true)"
  [[ "$running" == "true" ]]
}

detect_primary_ip() {
  local ip
  if have_cmd hostname; then
    ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
    ip="$(trim "${ip:-}")"
    if [[ -n "$ip" ]]; then
      printf '%s' "$ip"
      return
    fi
  fi
  if have_cmd ip; then
    ip="$(ip route get 1.1.1.1 2>/dev/null | sed -nE 's/.* src ([^ ]+).*/\1/p' | head -n1 || true)"
    ip="$(trim "${ip:-}")"
    if [[ -n "$ip" ]]; then
      printf '%s' "$ip"
      return
    fi
  fi
  printf '<server-ip>'
}

verify_direct_access() {
  local listen_addr="$1"
  local host="" port="" check_url=""
  if [[ "$listen_addr" == :* ]]; then
    host="127.0.0.1"
    port="${listen_addr#:}"
  elif [[ "$listen_addr" == *:* ]]; then
    host="${listen_addr%:*}"
    port="${listen_addr##*:}"
    host="${host#[}"
    host="${host%]}"
  else
    host="127.0.0.1"
    port="8080"
  fi

  if [[ -z "$port" || ! "$port" =~ ^[0-9]+$ ]]; then
    port="8080"
  fi
  if [[ -z "$host" || "$host" == "0.0.0.0" || "$host" == "::" ]]; then
    host="127.0.0.1"
  fi

  check_url="$(http_base_url_from_host_port "$host" "$port")/health/live"
  if curl -fsS --max-time 5 "$check_url" >/dev/null; then
    return 0
  fi
  if [[ "$host" != "127.0.0.1" ]]; then
    curl -fsS --max-time 5 "$(http_base_url_from_host_port "127.0.0.1" "$port")/health/live" >/dev/null
    return
  fi
  return 1
}

local_base_url_from_listen() {
  local listen_addr="$1"
  local host="" port=""
  if [[ "$listen_addr" == :* ]]; then
    host="127.0.0.1"
    port="${listen_addr#:}"
  elif [[ "$listen_addr" == *:* ]]; then
    host="${listen_addr%:*}"
    port="${listen_addr##*:}"
    host="${host#[}"
    host="${host%]}"
  else
    host="127.0.0.1"
    port="8080"
  fi
  if [[ -z "$port" || ! "$port" =~ ^[0-9]+$ ]]; then
    port="8080"
  fi
  if [[ -z "$host" || "$host" == "0.0.0.0" || "$host" == "::" ]]; then
    host="127.0.0.1"
  fi
  http_base_url_from_host_port "$host" "$port"
}

verify_setup_status() {
  local base_url="$1"
  curl -fsS --max-time 8 "${base_url}/api/v1/setup/status" >/dev/null
}

verify_proxy_access() {
  local server="$1" tls_enabled="$2"
  if [[ "$tls_enabled" == "1" ]]; then
    curl -kfsS --max-time 8 --resolve "${server}:443:127.0.0.1" "https://${server}/health/live" >/dev/null
    return
  fi
  curl -fsS --max-time 8 -H "Host: ${server}" "http://127.0.0.1/health/live" >/dev/null
}

verify_cap_siteverify_success_key() {
  local response="$1"
  [[ -n "$response" ]] || return 1
  [[ "$response" == *"\"success\":"* ]] || return 1
  [[ "$response" != *"<!doctype html"* ]] || return 1
}

verify_cap_text_asset_body() {
  local body="$1"
  [[ -n "$body" ]] || return 1
  [[ "$body" != *"Failed to resolve the requested file"* ]] || return 1
  [[ "$body" != *"Couldn't find the requested release version"* ]] || return 1
  [[ "$body" != *"<!doctype html"* ]] || return 1
}

verify_cap_wasm_binary_url() {
  local url="$1"
  shift
  local tmp size
  if ! tmp="$(mktemp)"; then
    return 1
  fi
  if ! curl "$@" -o "$tmp" "$url" >/dev/null 2>&1; then
    rm -f "$tmp"
    return 1
  fi
  size="$(wc -c <"$tmp" | tr -d '[:space:]')"
  rm -f "$tmp"
  [[ -n "$size" && "$size" =~ ^[0-9]+$ ]] || return 1
  [[ "$size" -gt 512 ]] || return 1
}

verify_cap_upstream_access() {
  local upstream="$1" site_key="$2" secret="$3"
  local payload response body
  body="$(curl -fsS --max-time 8 "http://${upstream}/assets/widget.js" || true)"
  verify_cap_text_asset_body "$body" || return 1
  body="$(curl -fsS --max-time 8 "http://${upstream}/assets/cap_wasm.js" || true)"
  verify_cap_text_asset_body "$body" || return 1
  verify_cap_wasm_binary_url "http://${upstream}/assets/cap_wasm_bg.wasm" -fsS --max-time 8 || return 1
  payload="$(printf '{"secret":"%s","response":"installer-smoke-invalid-token"}' "$secret")"
  response="$(curl -sS --max-time 8 -H "Content-Type: application/json" -d "$payload" "http://${upstream}/${site_key}/siteverify" || true)"
  verify_cap_siteverify_success_key "$response"
}

verify_cap_proxy_access() {
  local server="$1" tls_enabled="$2" site_key="$3" secret="$4"
  local payload response body
  payload="$(printf '{"secret":"%s","response":"installer-smoke-invalid-token"}' "$secret")"
  if [[ "$tls_enabled" == "1" ]]; then
    body="$(curl -kfsS --max-time 8 --resolve "${server}:443:127.0.0.1" "https://${server}/cap/assets/widget.js" || true)"
    verify_cap_text_asset_body "$body" || return 1
    body="$(curl -kfsS --max-time 8 --resolve "${server}:443:127.0.0.1" "https://${server}/cap/assets/cap_wasm.js" || true)"
    verify_cap_text_asset_body "$body" || return 1
    verify_cap_wasm_binary_url "https://${server}/cap/assets/cap_wasm_bg.wasm" -kfsS --max-time 8 --resolve "${server}:443:127.0.0.1" || return 1
    response="$(curl -ksS --max-time 8 --resolve "${server}:443:127.0.0.1" -H "Content-Type: application/json" -d "$payload" "https://${server}/cap/${site_key}/siteverify" || true)"
    verify_cap_siteverify_success_key "$response"
    return
  fi
  body="$(curl -fsS --max-time 8 -H "Host: ${server}" "http://127.0.0.1/cap/assets/widget.js" || true)"
  verify_cap_text_asset_body "$body" || return 1
  body="$(curl -fsS --max-time 8 -H "Host: ${server}" "http://127.0.0.1/cap/assets/cap_wasm.js" || true)"
  verify_cap_text_asset_body "$body" || return 1
  verify_cap_wasm_binary_url "http://127.0.0.1/cap/assets/cap_wasm_bg.wasm" -fsS --max-time 8 -H "Host: ${server}" || return 1
  response="$(curl -sS --max-time 8 -H "Host: ${server}" -H "Content-Type: application/json" -d "$payload" "http://127.0.0.1/cap/${site_key}/siteverify" || true)"
  verify_cap_siteverify_success_key "$response"
}

verify_cap_public_config() {
  local base_url="$1" site_key="$2"
  local body
  body="$(curl -fsS --max-time 8 "${base_url}/api/v1/public/captcha/config" || true)"
  [[ "$body" == *"\"enabled\":true"* ]] || return 1
  [[ "$body" == *"\"provider\":\"cap\""* ]] || return 1
  [[ "$body" == *"\"site_key\":\"${site_key}\""* ]] || return 1
  [[ "$body" == *"\"widget_api_url\":"* ]] || return 1
  [[ "$body" == *"/cap/${site_key}/"* ]] || return 1
}

derive_cookie_secure_mode() {
  local mode="$1" proxy_tls="$2"
  if [[ "$mode" == "proxy" && "$proxy_tls" == "1" ]]; then
    printf 'always'
    return
  fi
  printf 'never'
}

cookie_secure_legacy_value() {
  local mode="$1"
  if [[ "$mode" == "always" ]]; then
    printf 'true'
    return
  fi
  printf 'false'
}

validate_cookie_policy_tuple() {
  local mode="$1" proxy_tls="$2" cookie_mode="$3" listen_addr="$4"
  if [[ "$mode" == "direct" && "$cookie_mode" == "always" ]]; then
    err "Invalid cookie/deploy config: direct mode on ${listen_addr} cannot use secure-only cookies."
    return 1
  fi
  if [[ "$mode" == "proxy" && "$proxy_tls" == "1" && "$cookie_mode" != "always" ]]; then
    err "Invalid cookie/deploy config: HTTPS proxy mode requires COOKIE_SECURE_MODE=always."
    return 1
  fi
  if [[ "$mode" == "proxy" && "$proxy_tls" != "1" && "$cookie_mode" == "always" ]]; then
    err "Invalid cookie/deploy config: HTTP-only proxy mode cannot use secure-only cookies."
    return 1
  fi
  return 0
}

apply_ufw_rules() {
  local mode="$1"
  local rc=0
  if ! have_cmd ufw; then
    warn "ufw is not installed. Skipping local firewall automation."
    return
  fi
  if ! run_as_root ufw status >/dev/null 2>&1; then
    warn "ufw exists but could not be queried; skipping firewall automation."
    return
  fi
  local ufw_state
  ufw_state="$(run_as_root ufw status 2>/dev/null | head -n1 || true)"
  if echo "$ufw_state" | grep -qi "inactive"; then
    warn "ufw is currently inactive. Rules can still be staged, but firewall is not enforcing until enabled."
    if prompt_yes_no "Enable ufw now? (be careful on remote SSH hosts)" 0; then
      if ! run_as_root ufw --force enable >/dev/null 2>&1; then
        warn "Failed to enable ufw. Continuing without enabling."
      else
        log "ufw enabled."
      fi
    fi
  fi
  if [[ "$mode" == "proxy" ]]; then
    if prompt_yes_no "Open firewall ports 80/tcp and 443/tcp via ufw?" 1; then
      if ! run_as_root ufw allow 80/tcp >/dev/null 2>&1; then
        warn "Failed to apply ufw rule 80/tcp. Continue manually: sudo ufw allow 80/tcp"
        rc=1
      fi
      if ! run_as_root ufw allow 443/tcp >/dev/null 2>&1; then
        warn "Failed to apply ufw rule 443/tcp. Continue manually: sudo ufw allow 443/tcp"
        rc=1
      fi
      if [[ "$rc" -eq 0 ]]; then
        log "Applied ufw rules: 80/tcp, 443/tcp"
      else
        warn "ufw automation incomplete; installer will continue."
        run_as_root ufw status verbose || true
      fi
    fi
    return
  fi
  if prompt_yes_no "Open firewall port 8080/tcp via ufw for direct access?" 1; then
    if run_as_root ufw allow 8080/tcp >/dev/null 2>&1; then
      log "Applied ufw rule: 8080/tcp"
    else
      warn "Failed to apply ufw rule 8080/tcp. Continue manually: sudo ufw allow 8080/tcp"
      warn "ufw automation incomplete; installer will continue."
      run_as_root ufw status verbose || true
    fi
  fi
}

fallback_to_direct_mode() {
  warn "Falling back to direct mode (:8080)."
  DEPLOY_MODE="direct"
  PROXY_SETUP=0
  PROXY_SERVER=""
  PROXY_SERVER_NAME=""
  PROXY_TLS=0
  PROXY_CERT=""
  PROXY_KEY=""
  LISTEN_ADDR=":8080"
  COOKIE_SECURE_MODE="never"
  COOKIE_SECURE_LEGACY="false"

  set_env_var "$OUT_ENV" "DEPLOY_MODE" "$DEPLOY_MODE"
  set_env_var "$OUT_ENV" "LISTEN_ADDR" "$LISTEN_ADDR"
  set_env_var "$OUT_ENV" "TRUST_PROXY" "false"
  set_env_var "$OUT_ENV" "COOKIE_SECURE_MODE" "never"
  set_env_var "$OUT_ENV" "COOKIE_SECURE" "false"
  set_env_var "$OUT_ENV" "PROXY_SERVER" ""
  set_env_var "$OUT_ENV" "PROXY_SERVER_NAME" ""
  set_env_var "$OUT_ENV" "PROXY_TLS" "0"

  "${PREFIX[@]}" install -m 0640 "$OUT_ENV" /opt/despatch/.env || return 1
  "${PREFIX[@]}" chown root:despatch /opt/despatch/.env || return 1
  "${PREFIX[@]}" systemctl restart despatch || return 1

  run_nonfatal_step "firewall reconfiguration (direct mode)" apply_ufw_rules "direct"
  return 0
}

print_cloud_firewall_checklist() {
  local mode="$1"
  cat <<EOF

Cloud firewall / security-group checklist:
  - Ensure DNS points to this server public IP.
  - Ensure inbound rules allow required ports.
EOF
  if [[ "$mode" == "proxy" ]]; then
    cat <<EOF
  - Required inbound: TCP 80 and TCP 443.
EOF
  else
    cat <<EOF
  - Required inbound: TCP 8080.
EOF
  fi
  cat <<EOF
  - Verify no provider-level ACL/NACL blocks these ports.

EOF
}

ensure_service_dependencies() {
  local os_id="$1"
  local missing=()
  local install_pkgs=()

  if ! have_cmd go; then
    missing+=("go")
    install_pkgs+=("golang-go")
  fi
  if ! have_cmd cargo; then
    missing+=("cargo")
    install_pkgs+=("cargo" "rustc")
  fi
  if ! have_cmd gcc; then
    missing+=("gcc")
    install_pkgs+=("build-essential")
  fi
  if ! have_cmd git; then
    missing+=("git")
    install_pkgs+=("git")
  fi
  if ! have_cmd pkg-config; then
    missing+=("pkg-config")
    install_pkgs+=("pkg-config")
  fi
  if ! have_cmd curl; then
    missing+=("curl")
    install_pkgs+=("curl")
  fi

  if [[ "${#missing[@]}" -eq 0 ]]; then
    return
  fi

  warn "Missing build/runtime dependencies: ${missing[*]}"
  if [[ "$os_id" == "ubuntu" || "$os_id" == "debian" ]]; then
    if prompt_yes_no "Install missing dependencies automatically with apt?" 1; then
      install_apt_packages "${install_pkgs[@]}"
      return
    fi
  fi

  err "Required dependencies are missing: ${missing[*]}"
  exit 1
}

render_nginx_conf() {
  local server_name="$1" upstream="$2" tls_enabled="$3" cert_file="$4" key_file="$5" cap_upstream="${6:-}"
  local cap_location_block=""
  if [[ -n "$cap_upstream" ]]; then
    cap_location_block=$(cat <<EOF
    location /cap/ {
        proxy_pass http://${cap_upstream}/;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
EOF
)
  fi
  if [[ "$tls_enabled" == "1" ]]; then
    cat <<EOF
server {
    listen 80;
    server_name ${server_name};
    return 301 https://\$host\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name ${server_name};
    client_max_body_size 30m;

    ssl_certificate ${cert_file};
    ssl_certificate_key ${key_file};

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;

${cap_location_block}
    location / {
        proxy_pass http://${upstream};
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF
    return
  fi

  cat <<EOF
server {
    listen 80;
    server_name ${server_name};
    client_max_body_size 30m;

    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;

${cap_location_block}
    location / {
        proxy_pass http://${upstream};
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF
}

render_apache_conf() {
  local server_name="$1" upstream="$2" tls_enabled="$3" cert_file="$4" key_file="$5" cap_upstream="${6:-}"
  local cap_proxy_block=""
  if [[ -n "$cap_upstream" ]]; then
    cap_proxy_block=$(cat <<EOF
    ProxyPass /cap/ http://${cap_upstream}/ nocanon
    ProxyPassReverse /cap/ http://${cap_upstream}/
EOF
)
  fi
  if [[ "$tls_enabled" == "1" ]]; then
    cat <<EOF
<VirtualHost *:80>
    ServerName ${server_name}
    RewriteEngine On
    RewriteRule ^ https://%{HTTP_HOST}%{REQUEST_URI} [R=301,L]
</VirtualHost>

<VirtualHost *:443>
    ServerName ${server_name}

    SSLEngine on
    SSLCertificateFile ${cert_file}
    SSLCertificateKeyFile ${key_file}

    ProxyPreserveHost On
    ProxyRequests Off
    AllowEncodedSlashes NoDecode

    RequestHeader set X-Forwarded-Proto "https"
${cap_proxy_block}
    ProxyPass / http://${upstream}/ nocanon
    ProxyPassReverse / http://${upstream}/

    LimitRequestBody 31457280
    Header always set Strict-Transport-Security "max-age=31536000; includeSubDomains"
    Header always set X-Frame-Options "DENY"
    Header always set X-Content-Type-Options "nosniff"
</VirtualHost>
EOF
    return
  fi

  cat <<EOF
<VirtualHost *:80>
    ServerName ${server_name}

    ProxyPreserveHost On
    ProxyRequests Off
    AllowEncodedSlashes NoDecode

    RequestHeader set X-Forwarded-Proto expr=%{REQUEST_SCHEME}
${cap_proxy_block}
    ProxyPass / http://${upstream}/ nocanon
    ProxyPassReverse / http://${upstream}/

    LimitRequestBody 31457280
    Header always set X-Frame-Options "DENY"
    Header always set X-Content-Type-Options "nosniff"
</VirtualHost>
EOF
}

setup_nginx_proxy() {
  local server_name="$1" upstream="$2" tls_enabled="$3" cert_file="$4" key_file="$5" cap_upstream="${6:-}"
  local conf="/etc/nginx/sites-available/despatch.conf"
  local enabled="/etc/nginx/sites-enabled/despatch.conf"
  local tmp
  if ! have_cmd nginx; then
    err "nginx command not found but nginx proxy setup was requested."
    return 1
  fi
  if ! tmp="$(mktemp)"; then
    err "Failed to create temporary file for nginx config."
    return 1
  fi
  render_nginx_conf "$server_name" "$upstream" "$tls_enabled" "$cert_file" "$key_file" "$cap_upstream" >"$tmp"

  if ! "${PREFIX[@]}" mkdir -p /etc/nginx/sites-available /etc/nginx/sites-enabled; then
    err "Failed to create nginx site directories."
    rm -f "$tmp"
    return 1
  fi
  if ! "${PREFIX[@]}" install -m 0644 "$tmp" "$conf"; then
    err "Failed to install nginx config: $conf"
    rm -f "$tmp"
    return 1
  fi
  if ! "${PREFIX[@]}" ln -sfn "$conf" "$enabled"; then
    err "Failed to enable nginx site symlink: $enabled"
    rm -f "$tmp"
    return 1
  fi
  rm -f "$tmp"

  if [[ ! -L "$enabled" ]]; then
    err "Nginx site enablement failed: ${enabled} symlink missing"
    return 1
  fi

  if ! "${PREFIX[@]}" nginx -t; then
    err "nginx configuration test failed."
    return 1
  fi
  if ! "${PREFIX[@]}" systemctl enable --now nginx; then
    err "Failed to enable/start nginx."
    return 1
  fi
  if ! "${PREFIX[@]}" systemctl reload nginx; then
    err "Failed to reload nginx."
    return 1
  fi
  return 0
}

setup_apache_proxy() {
  local server_name="$1" upstream="$2" tls_enabled="$3" cert_file="$4" key_file="$5" cap_upstream="${6:-}"
  local conf="/etc/apache2/sites-available/despatch.conf"
  local tmp
  if ! have_cmd apache2ctl || ! have_cmd a2enmod || ! have_cmd a2ensite; then
    err "apache2 tooling missing (apache2ctl/a2enmod/a2ensite) but apache2 proxy setup was requested."
    return 1
  fi
  if ! tmp="$(mktemp)"; then
    err "Failed to create temporary file for apache2 config."
    return 1
  fi
  render_apache_conf "$server_name" "$upstream" "$tls_enabled" "$cert_file" "$key_file" "$cap_upstream" >"$tmp"

  if ! "${PREFIX[@]}" install -m 0644 "$tmp" "$conf"; then
    err "Failed to install apache2 config: $conf"
    rm -f "$tmp"
    return 1
  fi
  rm -f "$tmp"

  if ! "${PREFIX[@]}" a2enmod proxy proxy_http headers rewrite >/dev/null; then
    err "Failed enabling apache2 modules: proxy proxy_http headers rewrite"
    return 1
  fi
  if [[ "$tls_enabled" == "1" ]]; then
    if ! "${PREFIX[@]}" a2enmod ssl >/dev/null; then
      err "Failed enabling apache2 ssl module."
      return 1
    fi
  fi
  if ! "${PREFIX[@]}" a2ensite despatch.conf >/dev/null; then
    err "Failed enabling apache2 site: despatch.conf"
    return 1
  fi
  if [[ ! -L /etc/apache2/sites-enabled/despatch.conf && ! -f /etc/apache2/sites-enabled/despatch.conf ]]; then
    err "Apache2 site enablement failed: /etc/apache2/sites-enabled/despatch.conf missing"
    return 1
  fi
  if ! "${PREFIX[@]}" apache2ctl configtest; then
    err "apache2 configuration test failed."
    return 1
  fi
  if ! "${PREFIX[@]}" systemctl enable --now apache2; then
    err "Failed to enable/start apache2."
    return 1
  fi
  if ! "${PREFIX[@]}" systemctl reload apache2; then
    err "Failed to reload apache2."
    return 1
  fi
  return 0
}

detect_default_domain() {
  local out
  if [[ -f /etc/mailname ]]; then
    out="$(trim "$(cat /etc/mailname 2>/dev/null || true)")"
    if [[ "$out" == *.* && "$out" != "localhost" ]]; then
      printf '%s' "$out"
      return
    fi
  fi

  if have_cmd hostname; then
    out="$(hostname -f 2>/dev/null || true)"
    out="$(trim "$out")"
    if [[ "$out" == *.* && "$out" != "localhost" ]]; then
      printf '%s' "$out"
      return
    fi
  fi

  printf 'example.com'
}

generate_secret() {
  if have_cmd openssl; then
    openssl rand -hex 32
    return
  fi
  if [[ -r /dev/urandom ]]; then
    head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'
    return
  fi
  date +%s | sha256sum | awk '{print $1}'
}

emit_event "run_start" "run_id" "$DESPATCH_RUN_ID" "operation" "install"

begin_stage "fetch_source" "Source Fetch / Bootstrap" "10"
ensure_repo_checkout_or_bootstrap

if [[ ! -f "$ENV_EXAMPLE" ]]; then
  err ".env.example not found at $ENV_EXAMPLE"
  exit 1
fi
finish_stage_ok

begin_stage "preflight" "Preflight and Configuration" "10"
OS_ID="unknown"
OS_VER="unknown"
UNAME_ARCH="$(uname -m)"
UNAME_SYS="$(lower "$(uname -s)")"
if [[ -f /etc/os-release ]]; then
  # shellcheck source=/etc/os-release
  . /etc/os-release
  OS_ID="$(lower "${ID:-unknown}")"
  OS_VER="${VERSION_ID:-unknown}"
else
  OS_ID="$UNAME_SYS"
fi

GOARCH=""
case "$UNAME_ARCH" in
  x86_64|amd64) GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *)
    err "Unsupported CPU architecture: $UNAME_ARCH (supported: x86_64, aarch64/arm64)"
    exit 1
    ;;
esac

if [[ "$UNAME_SYS" != "linux" ]]; then
  warn "Detected non-Linux host: $UNAME_SYS. Service installation is Ubuntu/Linux only; .env generation still works."
fi
if [[ "$OS_ID" != "ubuntu" ]]; then
  warn "Detected OS: $OS_ID $OS_VER. This script is validated for Ubuntu Server; continuing in best-effort mode."
fi

log "Despatch interactive installer"
log "Detected platform: ${OS_ID} ${OS_VER}, arch ${UNAME_ARCH} (${GOARCH})"

if ! prompt_yes_no "Continue with installation?" 1; then
  err "Aborted"
  exit 1
fi

BASE_DOMAIN_DEFAULT="$(detect_default_domain)"
LISTEN_ADDR_DEFAULT=":8080"
INSTALL_SERVICE=1
PROXY_SETUP=0
PROXY_SERVER=""
PROXY_SERVER_NAME=""
PROXY_TLS=0
PROXY_CERT=""
PROXY_KEY=""
DEPLOY_MODE="direct"
CAPTCHA_ENABLED="false"
CAPTCHA_PROVIDER="turnstile"
CAPTCHA_SITE_KEY=""
CAPTCHA_WIDGET_API_URL=""
CAPTCHA_VERIFY_URL=""
CAPTCHA_SECRET=""
CAPTCHA_CAP_PROXY_UPSTREAM=""
CAPTCHA_CAP_ENABLE_ASSETS_SERVER="true"
CAPTCHA_CAP_WIDGET_VERSION="$DEFAULT_CAP_WIDGET_VERSION"
CAPTCHA_CAP_WASM_VERSION="$DEFAULT_CAP_WASM_VERSION"
CAPTCHA_CAP_AUTO_PROVISION=0
CAPTCHA_CAP_IMAGE="$DEFAULT_CAP_IMAGE"
CAPTCHA_CAP_ADMIN_KEY=""

BASE_DOMAIN="$(prompt_input "Primary mail domain (used by first-run web setup)" "$BASE_DOMAIN_DEFAULT")"
LISTEN_ADDR="$(prompt_input "HTTP listen address" "$LISTEN_ADDR_DEFAULT")"
if [[ "$UNAME_SYS" != "linux" ]]; then
  INSTALL_SERVICE=0
  warn "Service installation disabled on non-Linux hosts."
elif ! prompt_yes_no "Install and start systemd service automatically?" 1; then
  INSTALL_SERVICE=0
fi

if [[ "$INSTALL_SERVICE" -eq 1 ]]; then
  mapfile -t WEB_SERVERS < <(detect_web_servers)
  if [[ "${#WEB_SERVERS[@]}" -gt 0 ]]; then
    if prompt_yes_no "Detected installed reverse proxy ($(IFS=,; echo "${WEB_SERVERS[*]}")). Configure it automatically?" 1; then
      PROXY_SETUP=1
      DEPLOY_MODE="proxy"
      if [[ "${#WEB_SERVERS[@]}" -eq 1 ]]; then
        PROXY_SERVER="${WEB_SERVERS[0]}"
      else
        while true; do
          choice="$(lower "$(prompt_input "Choose proxy server: nginx or apache2" "${WEB_SERVERS[0]}")")"
          if [[ "$choice" == "nginx" || "$choice" == "apache2" ]]; then
            PROXY_SERVER="$choice"
            break
          fi
          warn "Choose either nginx or apache2."
        done
      fi
      if ! ensure_proxy_tooling "$PROXY_SERVER" "$OS_ID"; then
        warn "Selected reverse proxy prerequisites are not available."
        if prompt_yes_no "Continue without reverse proxy (direct mode on :8080)?" 1; then
          PROXY_SETUP=0
          DEPLOY_MODE="direct"
          PROXY_SERVER=""
          PROXY_SERVER_NAME=""
          PROXY_TLS=0
          PROXY_CERT=""
          PROXY_KEY=""
        else
          err "Cannot continue with proxy mode without required tooling."
          exit 1
        fi
      fi
      if [[ "$PROXY_SETUP" -eq 1 ]]; then
        PROXY_SERVER_NAME="$(prompt_input "Public server name for reverse proxy" "$BASE_DOMAIN")"
        TLS_CERT_DEFAULT="/etc/letsencrypt/live/${PROXY_SERVER_NAME}/fullchain.pem"
        TLS_KEY_DEFAULT="/etc/letsencrypt/live/${PROXY_SERVER_NAME}/privkey.pem"
        TLS_PROMPT_DEFAULT=0
        if mapfile -t TLS_PAIR < <(detect_letsencrypt_tls_pair "$PROXY_SERVER_NAME"); then
          if [[ "${#TLS_PAIR[@]}" -ge 2 ]]; then
            TLS_CERT_DEFAULT="${TLS_PAIR[0]}"
            TLS_KEY_DEFAULT="${TLS_PAIR[1]}"
            TLS_PROMPT_DEFAULT=1
            log "Auto-detected TLS certificate and key from /etc/letsencrypt/live for ${PROXY_SERVER_NAME}."
          fi
        fi
        if prompt_yes_no "Enable TLS in reverse proxy config now (requires existing cert files)?" "$TLS_PROMPT_DEFAULT"; then
          PROXY_TLS=1
          while true; do
            PROXY_CERT="$(prompt_input "TLS certificate file" "$TLS_CERT_DEFAULT")"
            PROXY_KEY="$(prompt_input "TLS private key file" "$TLS_KEY_DEFAULT")"
            if [[ -f "$PROXY_CERT" && -f "$PROXY_KEY" ]]; then
              break
            fi
            if mapfile -t TLS_PAIR < <(detect_letsencrypt_tls_pair "$PROXY_SERVER_NAME"); then
              if [[ "${#TLS_PAIR[@]}" -ge 2 ]]; then
                PROXY_CERT="${TLS_PAIR[0]}"
                PROXY_KEY="${TLS_PAIR[1]}"
                TLS_CERT_DEFAULT="$PROXY_CERT"
                TLS_KEY_DEFAULT="$PROXY_KEY"
                if [[ -f "$PROXY_CERT" && -f "$PROXY_KEY" ]]; then
                  log "Auto-recovered TLS paths from /etc/letsencrypt/live."
                  break
                fi
              fi
            fi
            warn "TLS files missing. cert_exists=$( [[ -f "$PROXY_CERT" ]] && echo yes || echo no ), key_exists=$( [[ -f "$PROXY_KEY" ]] && echo yes || echo no )"
            if ! prompt_yes_no "Re-enter TLS paths?" 1; then
              warn "Disabling TLS proxy setup for now. You can add TLS later."
              PROXY_TLS=0
              PROXY_CERT=""
              PROXY_KEY=""
              break
            fi
          done
        fi
        if [[ "$LISTEN_ADDR" == ":8080" ]]; then
          LISTEN_ADDR="127.0.0.1:8080"
          log "Adjusted app listen address to ${LISTEN_ADDR} because reverse proxy is enabled."
        fi
      fi
    fi
  fi
fi

if [[ "$PROXY_SETUP" -eq 1 ]]; then
  if prompt_yes_no "Enable self-hosted CAP captcha for registration now?" 0; then
    CAPTCHA_ENABLED="true"
    CAPTCHA_PROVIDER="cap"
    CAPTCHA_SITE_KEY="$(prompt_input "CAP site key" "cap-site-key")"
    CAPTCHA_SECRET="$(prompt_input "CAP verification secret" "$(generate_secret)")"
    CAPTCHA_CAP_PROXY_UPSTREAM="$(prompt_input "CAP upstream host:port for reverse proxy /cap/ route" "127.0.0.1:8077")"
    CAPTCHA_CAP_ENABLE_ASSETS_SERVER="$(lower "$(prompt_input "CAP ENABLE_ASSETS_SERVER value (true/false)" "true")")"
    CAPTCHA_CAP_WIDGET_VERSION="$(trim "$(prompt_input "CAP WIDGET_VERSION (pin, avoid latest)" "$DEFAULT_CAP_WIDGET_VERSION")")"
    CAPTCHA_CAP_WASM_VERSION="$(trim "$(prompt_input "CAP WASM_VERSION (pin, avoid latest)" "$DEFAULT_CAP_WASM_VERSION")")"
    if [[ -z "$CAPTCHA_CAP_WIDGET_VERSION" || "$CAPTCHA_CAP_WIDGET_VERSION" == "latest" ]]; then
      warn "Invalid CAP WIDGET_VERSION; defaulting to pinned ${DEFAULT_CAP_WIDGET_VERSION}."
      CAPTCHA_CAP_WIDGET_VERSION="$DEFAULT_CAP_WIDGET_VERSION"
    fi
    if [[ -z "$CAPTCHA_CAP_WASM_VERSION" || "$CAPTCHA_CAP_WASM_VERSION" == "latest" ]]; then
      warn "Invalid CAP WASM_VERSION; defaulting to pinned ${DEFAULT_CAP_WASM_VERSION}."
      CAPTCHA_CAP_WASM_VERSION="$DEFAULT_CAP_WASM_VERSION"
    fi
    CAPTCHA_WIDGET_API_URL="/cap/${CAPTCHA_SITE_KEY}/"
    CAPTCHA_VERIFY_URL="http://${CAPTCHA_CAP_PROXY_UPSTREAM}/${CAPTCHA_SITE_KEY}/siteverify"
    CAPTCHA_CAP_ENABLE_ASSETS_SERVER="true"
    if prompt_yes_no "Auto-provision CAP standalone runtime with Docker?" 1; then
      CAPTCHA_CAP_AUTO_PROVISION=1
      CAPTCHA_CAP_IMAGE="$(trim "$(prompt_input "CAP image (Docker)" "$DEFAULT_CAP_IMAGE")")"
      CAPTCHA_CAP_ADMIN_KEY="$(prompt_input "CAP admin key (for CAP dashboard access)" "$(generate_secret)")"
      if ! have_cmd docker && [[ "$OS_ID" == "ubuntu" || "$OS_ID" == "debian" ]]; then
        warn "Docker is required for CAP auto-provisioning."
        if prompt_yes_no "Install Docker automatically now?" 1; then
          install_apt_packages docker.io docker-compose-plugin
        fi
      fi
    fi
  fi
fi

if [[ "$CAPTCHA_CAP_AUTO_PROVISION" == "1" && -z "$CAPTCHA_CAP_IMAGE" ]]; then
  CAPTCHA_CAP_IMAGE="$DEFAULT_CAP_IMAGE"
fi

SQL_CONF="$(detect_sql_conf_file)"
DOVECOT_AUTH_MODE_DETECTED="$(detect_dovecot_auth_mode)"
DOVECOT_AUTH_MODE="$(lower "$(prompt_input "Dovecot auth backend mode (pam or sql)" "$DOVECOT_AUTH_MODE_DETECTED")")"
if [[ "$DOVECOT_AUTH_MODE" != "pam" && "$DOVECOT_AUTH_MODE" != "sql" ]]; then
  warn "Invalid auth backend mode: $DOVECOT_AUTH_MODE. Falling back to detected mode: $DOVECOT_AUTH_MODE_DETECTED"
  DOVECOT_AUTH_MODE="$DOVECOT_AUTH_MODE_DETECTED"
fi
DOVECOT_DRIVER_RAW=""
DOVECOT_DRIVER=""
DOVECOT_CONNECT=""
DOVECOT_PASSWORD_QUERY=""
DOVECOT_USER_QUERY=""
DOVECOT_TABLE="users"
DOVECOT_EMAIL_COL="email"
DOVECOT_PASS_COL="password_hash"
DOVECOT_ACTIVE_COL=""
DOVECOT_MAILDIR_COL=""
DOVECOT_DSN=""

if [[ "$DOVECOT_AUTH_MODE" == "sql" && -n "$SQL_CONF" ]]; then
  DOVECOT_DRIVER_RAW="$(extract_kv_line "$SQL_CONF" driver)"
  DOVECOT_CONNECT="$(extract_kv_line "$SQL_CONF" connect)"
  DOVECOT_PASSWORD_QUERY="$(extract_kv_line "$SQL_CONF" password_query)"
  DOVECOT_USER_QUERY="$(extract_kv_line "$SQL_CONF" user_query)"

  case "$(lower "$DOVECOT_DRIVER_RAW")" in
    mysql) DOVECOT_DRIVER="mysql" ;;
    pgsql|postgres|postgresql|pgx) DOVECOT_DRIVER="pgx" ;;
    *) DOVECOT_DRIVER="" ;;
  esac

  if [[ -n "$DOVECOT_PASSWORD_QUERY" ]]; then
    t="$(extract_table_from_query "$DOVECOT_PASSWORD_QUERY")"
    [[ -n "$t" ]] && DOVECOT_TABLE="$t"
    c="$(extract_email_col_from_query "$DOVECOT_PASSWORD_QUERY")"
    [[ -n "$c" ]] && DOVECOT_EMAIL_COL="$c"
    c="$(extract_password_col_from_query "$DOVECOT_PASSWORD_QUERY")"
    [[ -n "$c" ]] && DOVECOT_PASS_COL="$c"
    c="$(extract_active_col_from_query "$DOVECOT_PASSWORD_QUERY")"
    [[ -n "$c" ]] && DOVECOT_ACTIVE_COL="$c"
  fi

  if [[ -n "$DOVECOT_USER_QUERY" ]]; then
    c="$(extract_maildir_col_from_query "$DOVECOT_USER_QUERY")"
    [[ -n "$c" ]] && DOVECOT_MAILDIR_COL="$c"
    t="$(extract_table_from_query "$DOVECOT_USER_QUERY")"
    [[ -n "$t" ]] && DOVECOT_TABLE="$t"
    c="$(extract_email_col_from_query "$DOVECOT_USER_QUERY")"
    [[ -n "$c" ]] && DOVECOT_EMAIL_COL="$c"
  fi

  if [[ -n "$DOVECOT_DRIVER" && -n "$DOVECOT_CONNECT" ]]; then
    DOVECOT_DSN="$(build_dsn "$DOVECOT_DRIVER" "$DOVECOT_CONNECT")"
  fi
fi

if [[ "$DOVECOT_AUTH_MODE" == "sql" ]]; then
  if [[ -z "$DOVECOT_DRIVER" || -z "$DOVECOT_DSN" ]]; then
    warn "Could not fully auto-detect Dovecot SQL writable credentials."
    if prompt_yes_no "Enter Dovecot SQL settings manually now?" 0; then
      DOVECOT_DRIVER="$(prompt_input "Dovecot auth DB driver (mysql or pgx)" "mysql")"
      DOVECOT_DRIVER="$(lower "$DOVECOT_DRIVER")"
      DOVECOT_DSN="$(prompt_input "Dovecot auth DB DSN")"
      DOVECOT_TABLE="$(prompt_input "Dovecot auth table" "$DOVECOT_TABLE")"
      DOVECOT_EMAIL_COL="$(prompt_input "Email/login column" "$DOVECOT_EMAIL_COL")"
      DOVECOT_PASS_COL="$(prompt_input "Password hash column" "$DOVECOT_PASS_COL")"
      DOVECOT_ACTIVE_COL="$(prompt_input "Active/enabled column (blank if none)" "$DOVECOT_ACTIVE_COL")"
      DOVECOT_MAILDIR_COL="$(prompt_input "Maildir column (blank if none)" "$DOVECOT_MAILDIR_COL")"
    else
      warn "Dovecot SQL provisioning will remain disabled until configured in .env"
    fi
  fi
else
  DOVECOT_DRIVER=""
  DOVECOT_DSN=""
  DOVECOT_ACTIVE_COL=""
  DOVECOT_MAILDIR_COL=""
  log "PAM auth mode selected. Dovecot SQL provisioning is disabled."
fi

IMAP_PORT="$(detect_imap_port)"
SMTP_PORT="$(detect_smtp_port)"
IMAP_HOST="127.0.0.1"
SMTP_HOST="127.0.0.1"
IMAP_TLS="true"
IMAP_STARTTLS="false"
if [[ "$IMAP_PORT" == "143" ]]; then
  IMAP_TLS="false"
  IMAP_STARTTLS="true"
fi
IMAP_INSECURE_SKIP_VERIFY="$(derive_imap_insecure_skip_verify "$IMAP_HOST" "$IMAP_TLS" "$IMAP_STARTTLS")"
SMTP_TLS="false"
SMTP_STARTTLS="true"
if [[ "$SMTP_PORT" == "465" ]]; then
  SMTP_TLS="true"
  SMTP_STARTTLS="false"
fi
if [[ "$SMTP_PORT" == "25" ]]; then
  SMTP_STARTTLS="false"
fi
SMTP_INSECURE_SKIP_VERIFY="$(derive_smtp_insecure_skip_verify "$SMTP_HOST" "$SMTP_TLS" "$SMTP_STARTTLS")"
PASSWORD_RESET_FROM_DOMAIN="$(derive_password_reset_sender_domain "$BASE_DOMAIN")"
PASSWORD_RESET_FROM_ADDR="no-reply@${PASSWORD_RESET_FROM_DOMAIN}"
PASSWORD_RESET_BASE_URL="$(derive_password_reset_base_url "$DEPLOY_MODE" "$PROXY_SERVER_NAME" "$PROXY_TLS" "$BASE_DOMAIN" "$LISTEN_ADDR")"
PASSWORD_RESET_EXTERNAL_SENDER_READY="false"
if [[ "$DOVECOT_AUTH_MODE" == "pam" && "$INSTALL_SERVICE" -eq 1 && "$SMTP_HOST" == "127.0.0.1" && "$SMTP_PORT" == "25" ]]; then
  PASSWORD_RESET_EXTERNAL_SENDER_READY="true"
fi

finish_stage_ok
begin_stage "env_generation" "Environment Generation" "10"
SESSION_KEY="$(generate_secret)"
APP_DB_PATH="./data/app.db"
if [[ "$INSTALL_SERVICE" -eq 1 ]]; then
  APP_DB_PATH="/var/lib/despatch/app.db"
fi

cp "$ENV_EXAMPLE" "$OUT_ENV"
COOKIE_SECURE_MODE="$(derive_cookie_secure_mode "$DEPLOY_MODE" "$PROXY_TLS")"
COOKIE_SECURE_LEGACY="$(cookie_secure_legacy_value "$COOKIE_SECURE_MODE")"
set_env_var "$OUT_ENV" "BASE_DOMAIN" "$BASE_DOMAIN"
set_env_var "$OUT_ENV" "LISTEN_ADDR" "$LISTEN_ADDR"
set_env_var "$OUT_ENV" "APP_DB_PATH" "$APP_DB_PATH"
set_env_var "$OUT_ENV" "SESSION_ENCRYPT_KEY" "$SESSION_KEY"
set_env_var "$OUT_ENV" "COOKIE_SECURE_MODE" "$COOKIE_SECURE_MODE"
set_env_var "$OUT_ENV" "COOKIE_SECURE" "$COOKIE_SECURE_LEGACY"
set_env_var "$OUT_ENV" "DEPLOY_MODE" "$DEPLOY_MODE"
set_env_var "$OUT_ENV" "DOVECOT_AUTH_MODE" "$DOVECOT_AUTH_MODE"
set_env_var "$OUT_ENV" "PROXY_SERVER" "$PROXY_SERVER"
set_env_var "$OUT_ENV" "PROXY_SERVER_NAME" "$PROXY_SERVER_NAME"
set_env_var "$OUT_ENV" "PROXY_TLS" "$PROXY_TLS"
if [[ "$PROXY_SETUP" -eq 1 ]]; then
  set_env_var "$OUT_ENV" "TRUST_PROXY" "true"
fi

set_env_var "$OUT_ENV" "IMAP_HOST" "$IMAP_HOST"
set_env_var "$OUT_ENV" "IMAP_PORT" "$IMAP_PORT"
set_env_var "$OUT_ENV" "IMAP_TLS" "$IMAP_TLS"
set_env_var "$OUT_ENV" "IMAP_STARTTLS" "$IMAP_STARTTLS"
set_env_var "$OUT_ENV" "IMAP_INSECURE_SKIP_VERIFY" "$IMAP_INSECURE_SKIP_VERIFY"
set_env_var "$OUT_ENV" "MAIL_ALLOW_INSECURE_SKIP_VERIFY_NON_LOOPBACK" "false"
set_env_var "$OUT_ENV" "SMTP_HOST" "$SMTP_HOST"
set_env_var "$OUT_ENV" "SMTP_PORT" "$SMTP_PORT"
set_env_var "$OUT_ENV" "SMTP_TLS" "$SMTP_TLS"
set_env_var "$OUT_ENV" "SMTP_STARTTLS" "$SMTP_STARTTLS"
set_env_var "$OUT_ENV" "SMTP_INSECURE_SKIP_VERIFY" "$SMTP_INSECURE_SKIP_VERIFY"

set_env_var "$OUT_ENV" "BOOTSTRAP_ADMIN_EMAIL" ""
set_env_var "$OUT_ENV" "BOOTSTRAP_ADMIN_PASSWORD" ""
set_env_var "$OUT_ENV" "UPDATE_ENABLED" "true"
set_env_var "$OUT_ENV" "UPDATE_REPO_OWNER" "2high4schooltoday"
set_env_var "$OUT_ENV" "UPDATE_REPO_NAME" "despatch"
set_env_var "$OUT_ENV" "UPDATE_CHECK_INTERVAL_MIN" "60"
set_env_var "$OUT_ENV" "UPDATE_HTTP_TIMEOUT_SEC" "10"
set_env_var "$OUT_ENV" "UPDATE_GITHUB_TOKEN" ""
set_env_var "$OUT_ENV" "UPDATE_BACKUP_KEEP" "3"
set_env_var "$OUT_ENV" "UPDATE_BASE_DIR" "/var/lib/despatch/update"
set_env_var "$OUT_ENV" "UPDATE_INSTALL_DIR" "/opt/despatch"
set_env_var "$OUT_ENV" "UPDATE_SERVICE_NAME" "despatch"
set_env_var "$OUT_ENV" "UPDATE_SYSTEMD_UNIT_DIR" "/etc/systemd/system"
set_env_var "$OUT_ENV" "UPDATE_REQUIRE_SIGNATURE" "true"
set_env_var "$OUT_ENV" "UPDATE_SIGNATURE_ASSET" "checksums.txt.sig"
set_env_var "$OUT_ENV" "UPDATE_SIGNING_PUBLIC_KEYS" "NgWFCGvluSgGiR4KpQhrNGqK4UVDRcEb76RNBEproBw="
set_env_var "$OUT_ENV" "CAPTCHA_ENABLED" "$CAPTCHA_ENABLED"
set_env_var "$OUT_ENV" "CAPTCHA_PROVIDER" "$CAPTCHA_PROVIDER"
set_env_var "$OUT_ENV" "CAPTCHA_SITE_KEY" "$CAPTCHA_SITE_KEY"
set_env_var "$OUT_ENV" "CAPTCHA_WIDGET_API_URL" "$CAPTCHA_WIDGET_API_URL"
set_env_var "$OUT_ENV" "CAPTCHA_VERIFY_URL" "$CAPTCHA_VERIFY_URL"
set_env_var "$OUT_ENV" "CAPTCHA_SECRET" "$CAPTCHA_SECRET"

if [[ -n "$DOVECOT_DRIVER" ]]; then
  set_env_var "$OUT_ENV" "DOVECOT_AUTH_DB_DRIVER" "$DOVECOT_DRIVER"
fi
if [[ -n "$DOVECOT_DSN" ]]; then
  set_env_var "$OUT_ENV" "DOVECOT_AUTH_DB_DSN" "$DOVECOT_DSN"
fi
set_env_var "$OUT_ENV" "DOVECOT_AUTH_TABLE" "$DOVECOT_TABLE"
set_env_var "$OUT_ENV" "DOVECOT_AUTH_EMAIL_COL" "$DOVECOT_EMAIL_COL"
set_env_var "$OUT_ENV" "DOVECOT_AUTH_PASS_COL" "$DOVECOT_PASS_COL"
set_env_var "$OUT_ENV" "DOVECOT_AUTH_ACTIVE_COL" "$DOVECOT_ACTIVE_COL"
set_env_var "$OUT_ENV" "DOVECOT_AUTH_MAILDIR_COL" "$DOVECOT_MAILDIR_COL"
set_env_var "$OUT_ENV" "PASSWORD_RESET_TOKEN_TTL_MINUTES" "30"
set_env_var "$OUT_ENV" "PASSWORD_RESET_PUBLIC_ENABLED" "true"
set_env_var "$OUT_ENV" "PASSWORD_RESET_REQUIRE_MAPPED_LOGIN" "true"
set_env_var "$OUT_ENV" "PASSWORD_RESET_SENDER" "smtp"
set_env_var "$OUT_ENV" "PASSWORD_RESET_FROM" "$PASSWORD_RESET_FROM_ADDR"
set_env_var "$OUT_ENV" "PASSWORD_RESET_BASE_URL" "$PASSWORD_RESET_BASE_URL"
set_env_var "$OUT_ENV" "PASSWORD_RESET_EXTERNAL_SENDER_READY" "$PASSWORD_RESET_EXTERNAL_SENDER_READY"
if [[ "$DOVECOT_AUTH_MODE" == "pam" && "$INSTALL_SERVICE" -eq 1 ]]; then
 set_env_var "$OUT_ENV" "PAM_RESET_HELPER_ENABLED" "true"
else
 set_env_var "$OUT_ENV" "PAM_RESET_HELPER_ENABLED" "false"
fi
set_env_var "$OUT_ENV" "PAM_RESET_HELPER_SOCKET" "/run/despatch/pam-reset-helper.sock"
set_env_var "$OUT_ENV" "PAM_RESET_HELPER_TIMEOUT_SEC" "5"
set_env_var "$OUT_ENV" "PAM_RESET_ALLOWED_UID" "-1"
set_env_var "$OUT_ENV" "PAM_RESET_ALLOWED_GID" "-1"
set_env_var "$OUT_ENV" "MAILSEC_ENABLED" "true"
set_env_var "$OUT_ENV" "MAILSEC_SOCKET" "/run/despatch/mailsec.sock"
set_env_var "$OUT_ENV" "MAILSEC_TIMEOUT_MS" "5000"

log "Generated $OUT_ENV"
log "Detected IMAP ${IMAP_HOST}:${IMAP_PORT} and SMTP ${SMTP_HOST}:${SMTP_PORT}"
log "Password reset sender: ${PASSWORD_RESET_FROM_ADDR}"
log "Password reset base URL: ${PASSWORD_RESET_BASE_URL}"
if [[ "$IMAP_INSECURE_SKIP_VERIFY" == "true" ]]; then
  warn "IMAP host is loopback with TLS/STARTTLS enabled; auto-setting IMAP_INSECURE_SKIP_VERIFY=true to avoid local certificate SAN mismatches."
fi
if [[ "$SMTP_INSECURE_SKIP_VERIFY" == "true" ]]; then
  warn "SMTP host is loopback with TLS/STARTTLS enabled; auto-setting SMTP_INSECURE_SKIP_VERIFY=true to avoid local certificate SAN mismatches."
fi
if [[ "$IMAP_INSECURE_SKIP_VERIFY" != "true" && "$SMTP_INSECURE_SKIP_VERIFY" != "true" ]]; then
  log "TLS certificate verification for IMAP/SMTP is enabled by default."
fi
log "Deployment mode: $DEPLOY_MODE"
log "Cookie secure mode: $COOKIE_SECURE_MODE"
if [[ "$DEPLOY_MODE" == "proxy" && "$PROXY_TLS" != "1" ]]; then
  warn "Reverse proxy TLS is disabled; cookies are configured for HTTP transport (COOKIE_SECURE_MODE=never)."
fi
if [[ "$CAPTCHA_ENABLED" == "true" && "$CAPTCHA_PROVIDER" == "cap" ]]; then
  log "CAPTCHA provider: cap (widget=${CAPTCHA_WIDGET_API_URL}, verify=${CAPTCHA_VERIFY_URL})"
  log "Expected CAP standalone env: ENABLE_ASSETS_SERVER=${CAPTCHA_CAP_ENABLE_ASSETS_SERVER}, WIDGET_VERSION=${CAPTCHA_CAP_WIDGET_VERSION}, WASM_VERSION=${CAPTCHA_CAP_WASM_VERSION}"
  if [[ "$CAPTCHA_CAP_AUTO_PROVISION" == "1" ]]; then
    log "CAP runtime provisioning: enabled (image=${CAPTCHA_CAP_IMAGE}, upstream=${CAPTCHA_CAP_PROXY_UPSTREAM})"
  else
    warn "CAP runtime provisioning: disabled; ensure CAP service is already running on ${CAPTCHA_CAP_PROXY_UPSTREAM}."
  fi
fi
log "Dovecot auth mode: $DOVECOT_AUTH_MODE"
if [[ -n "$SQL_CONF" ]]; then
  log "Detected Dovecot SQL file: $SQL_CONF"
fi

if [[ "$INSTALL_SERVICE" -eq 0 ]]; then
  finish_stage_ok
  begin_stage "final_summary" "Final Summary" "0"
  cat <<NEXT

Run locally:
  cd "$ROOT_DIR"
  go run ./cmd/server

Open:
  http://localhost:8080

The first web visit will launch OOBE where you set admin email/password.
Default admin email in OOBE is webmaster@${BASE_DOMAIN}.
NEXT
  finish_stage_ok
  emit_run_result_once "ok" "" "0"
  exit 0
fi

finish_stage_ok
begin_stage "deps" "Dependency Checks" "15"
ensure_service_dependencies "$OS_ID"
if ! have_cmd go; then
  err "Go toolchain is still unavailable after dependency checks."
  exit 1
fi
if ! have_cmd cargo; then
  err "Rust toolchain (cargo) is required to build privileged helper binaries."
  err "Install rustup/cargo and rerun installer, or deploy from prebuilt release archives."
  exit 1
fi
finish_stage_ok

if [[ "$OS_ID" != "ubuntu" ]]; then
  warn "Proceeding with best-effort service install on non-Ubuntu Linux"
fi

if ! have_cmd systemctl; then
  err "systemd is required for automatic service install."
  exit 1
fi

PREFIX=()
if [[ "${EUID:-1}" -ne 0 ]]; then
  if ! have_cmd sudo; then
    err "sudo is required when running installer as non-root"
    exit 1
  fi
  PREFIX=(sudo)
fi

begin_stage "build" "Build Binary" "15"
log "Building despatch binary for linux/${GOARCH}"
BUILD_VERSION="$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || echo dev)"
BUILD_COMMIT="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
BUILD_REPO="${DESPATCH_REPO_URL:-https://github.com/2high4schooltoday/despatch}"
BUILD_LDFLAGS="-X 'despatch/internal/version.Version=${BUILD_VERSION}' -X 'despatch/internal/version.Commit=${BUILD_COMMIT}' -X 'despatch/internal/version.BuildTime=${BUILD_TIME}' -X 'despatch/internal/version.SourceRepo=${BUILD_REPO}'"
(
  cd "$ROOT_DIR"
  GOOS=linux GOARCH="$GOARCH" go build -ldflags "$BUILD_LDFLAGS" -o "$ROOT_DIR/despatch" ./cmd/server
)
case "$GOARCH" in
  amd64|x86_64)
    RUST_TARGET="x86_64-unknown-linux-gnu"
    ;;
  arm64|aarch64)
    RUST_TARGET="aarch64-unknown-linux-gnu"
    ;;
  *)
    err "Unsupported GOARCH for Rust privileged binaries: $GOARCH"
    exit 1
    ;;
esac
log "Building privileged Rust binaries for ${RUST_TARGET}"
ensure_rust_toolchain "$RUST_TARGET"
(
  cd "$ROOT_DIR/rust"
  DESPATCH_BUILD_VERSION="$BUILD_VERSION" \
  DESPATCH_BUILD_COMMIT="$BUILD_COMMIT" \
  cargo build --release --locked --target "$RUST_TARGET" -p pam_reset_helper -p update_worker -p mailsec_service
) || (
  warn "Initial Rust privileged binary build failed; refreshing stable toolchain and retrying once."
  ensure_rust_toolchain "$RUST_TARGET"
  cd "$ROOT_DIR/rust"
  DESPATCH_BUILD_VERSION="$BUILD_VERSION" \
  DESPATCH_BUILD_COMMIT="$BUILD_COMMIT" \
  cargo build --release --locked --target "$RUST_TARGET" -p pam_reset_helper -p update_worker -p mailsec_service
)
RUST_BIN_DIR="$ROOT_DIR/rust/target/$RUST_TARGET/release"
if [[ ! -x "$RUST_BIN_DIR/pam_reset_helper" || ! -x "$RUST_BIN_DIR/update_worker" || ! -x "$RUST_BIN_DIR/mailsec_service" ]]; then
  err "Rust privileged binaries were not produced in $RUST_BIN_DIR"
  exit 1
fi
finish_stage_ok

begin_stage "filesystem_and_user" "Filesystem and User Setup" "10"
"${PREFIX[@]}" mkdir -p /opt/despatch /var/lib/despatch
if ! id -u despatch >/dev/null 2>&1; then
  "${PREFIX[@]}" useradd --system --home /var/lib/despatch --shell /usr/sbin/nologin despatch
fi
DESPATCH_UID="$(id -u despatch)"
DESPATCH_GID="$(id -g despatch)"
set_env_var "$OUT_ENV" "PAM_RESET_ALLOWED_UID" "$DESPATCH_UID"
set_env_var "$OUT_ENV" "PAM_RESET_ALLOWED_GID" "$DESPATCH_GID"
"${PREFIX[@]}" mkdir -p /var/lib/despatch/update/request /var/lib/despatch/update/status /var/lib/despatch/update/lock /var/lib/despatch/update/backups /var/lib/despatch/update/work

"${PREFIX[@]}" install -m 0755 "$ROOT_DIR/despatch" /opt/despatch/despatch
"${PREFIX[@]}" install -m 0755 "$RUST_BIN_DIR/pam_reset_helper" /opt/despatch/despatch-pam-reset-helper
"${PREFIX[@]}" install -m 0755 "$RUST_BIN_DIR/update_worker" /opt/despatch/despatch-update-worker
"${PREFIX[@]}" install -m 0755 "$RUST_BIN_DIR/mailsec_service" /opt/despatch/despatch-mailsec-service
"${PREFIX[@]}" install -m 0640 "$OUT_ENV" /opt/despatch/.env
"${PREFIX[@]}" rm -rf /opt/despatch/web /opt/despatch/migrations
"${PREFIX[@]}" cp -R "$ROOT_DIR/web" /opt/despatch/web
"${PREFIX[@]}" cp -R "$ROOT_DIR/migrations" /opt/despatch/migrations
"${PREFIX[@]}" mkdir -p /opt/despatch/deploy/fallback
"${PREFIX[@]}" install -m 0644 "$ROOT_DIR/deploy/fallback/despatch-pam-reset-helper-go.override.conf" /opt/despatch/deploy/fallback/despatch-pam-reset-helper-go.override.conf
"${PREFIX[@]}" install -m 0644 "$ROOT_DIR/deploy/fallback/despatch-updater-go.override.conf" /opt/despatch/deploy/fallback/despatch-updater-go.override.conf
"${PREFIX[@]}" install -m 0644 "$ROOT_DIR/deploy/despatch.service" /etc/systemd/system/despatch.service
"${PREFIX[@]}" install -m 0644 "$ROOT_DIR/deploy/despatch-updater.service" /etc/systemd/system/despatch-updater.service
"${PREFIX[@]}" install -m 0644 "$ROOT_DIR/deploy/despatch-updater.path" /etc/systemd/system/despatch-updater.path
"${PREFIX[@]}" install -m 0644 "$ROOT_DIR/deploy/despatch-pam-reset-helper.service" /etc/systemd/system/despatch-pam-reset-helper.service
"${PREFIX[@]}" install -m 0644 "$ROOT_DIR/deploy/despatch-pam-reset-helper.socket" /etc/systemd/system/despatch-pam-reset-helper.socket
"${PREFIX[@]}" install -m 0644 "$ROOT_DIR/deploy/despatch-mailsec.service" /etc/systemd/system/despatch-mailsec.service
"${PREFIX[@]}" chown root:root /opt/despatch
"${PREFIX[@]}" chmod 0755 /opt/despatch
"${PREFIX[@]}" chown root:root /opt/despatch/despatch /opt/despatch/despatch-pam-reset-helper /opt/despatch/despatch-update-worker /opt/despatch/despatch-mailsec-service /opt/despatch/web /opt/despatch/migrations /opt/despatch/deploy
"${PREFIX[@]}" chmod 0755 /opt/despatch/despatch /opt/despatch/despatch-pam-reset-helper /opt/despatch/despatch-update-worker /opt/despatch/despatch-mailsec-service
"${PREFIX[@]}" chmod -R go-w /opt/despatch/web /opt/despatch/migrations /opt/despatch/deploy
"${PREFIX[@]}" chown root:despatch /opt/despatch/.env
"${PREFIX[@]}" chmod 0640 /opt/despatch/.env
"${PREFIX[@]}" chown despatch:despatch /var/lib/despatch
"${PREFIX[@]}" chmod 0750 /var/lib/despatch
"${PREFIX[@]}" chown root:despatch /var/lib/despatch/update
"${PREFIX[@]}" chmod 0750 /var/lib/despatch/update
"${PREFIX[@]}" chown root:root /var/lib/despatch/update/lock /var/lib/despatch/update/backups /var/lib/despatch/update/work
"${PREFIX[@]}" chmod 0750 /var/lib/despatch/update/lock /var/lib/despatch/update/backups /var/lib/despatch/update/work
"${PREFIX[@]}" chown root:despatch /var/lib/despatch/update/request /var/lib/despatch/update/status
"${PREFIX[@]}" chmod 0770 /var/lib/despatch/update/request /var/lib/despatch/update/status
"${PREFIX[@]}" find /var/lib/despatch/update/request /var/lib/despatch/update/status -type f -exec chown root:despatch {} \; -exec chmod 0660 {} \; || true
finish_stage_ok

begin_stage "service_install_start" "Service Install and Start" "10"
"${PREFIX[@]}" systemctl daemon-reload
"${PREFIX[@]}" rm -f /var/lib/despatch/update/request/update-request*.json /var/lib/despatch/update/lock/update.lock || true
"${PREFIX[@]}" systemctl reset-failed despatch-updater.service despatch-updater.path || true
"${PREFIX[@]}" systemctl enable --now despatch-updater.path
"${PREFIX[@]}" systemctl enable --now despatch
"${PREFIX[@]}" systemctl enable --now despatch-mailsec
"${PREFIX[@]}" systemctl enable --now despatch-pam-reset-helper.socket

log "Service installed and started: despatch"
finish_stage_ok

begin_stage "firewall" "Firewall Configuration" "5"
run_nonfatal_step "firewall configuration" apply_ufw_rules "$DEPLOY_MODE"
print_cloud_firewall_checklist "$DEPLOY_MODE"
finish_stage_ok

begin_stage "proxy" "Reverse Proxy Configuration" "10"
if [[ "$PROXY_SETUP" -eq 1 ]]; then
  step "Reverse proxy configuration (${PROXY_SERVER})"
  if ! ensure_proxy_tooling "$PROXY_SERVER" "$OS_ID"; then
    warn "Proxy tooling missing at configure time."
    fallback_to_direct_mode || true
  else
    APP_UPSTREAM="$LISTEN_ADDR"
    if configure_selected_proxy "$PROXY_SERVER" "$PROXY_SERVER_NAME" "$APP_UPSTREAM" "$PROXY_TLS" "$PROXY_CERT" "$PROXY_KEY" "$CAPTCHA_CAP_PROXY_UPSTREAM"; then
      log "Reverse proxy configured: ${PROXY_SERVER} (${PROXY_SERVER_NAME})"
      if [[ "$PROXY_SERVER" == "nginx" ]]; then
        log "Reverse proxy config path: /etc/nginx/sites-available/despatch.conf"
      else
        log "Reverse proxy config path: /etc/apache2/sites-available/despatch.conf"
      fi
    else
      warn "Reverse proxy configuration failed."
      if ! fallback_to_direct_mode; then
        err "Automatic fallback to direct mode failed."
        err "Run: systemctl status despatch --no-pager"
        exit 1
      fi
    fi
  fi
else
  log "Reverse proxy stage skipped (direct mode)."
fi
finish_stage_ok

begin_stage "post_checks" "Post-install Checks" "5"
step "Deployment and cookie policy consistency"
if ! validate_cookie_policy_tuple "$DEPLOY_MODE" "$PROXY_TLS" "$COOKIE_SECURE_MODE" "$LISTEN_ADDR"; then
  err "Generated deployment tuple is inconsistent."
  err "Run: grep -E '^(DEPLOY_MODE|LISTEN_ADDR|COOKIE_SECURE_MODE|COOKIE_SECURE)=' /opt/despatch/.env"
  exit 1
fi
if ! wait_for_condition "despatch service state" 20 1 "${PREFIX[@]}" systemctl is-active --quiet despatch; then
  err "despatch service is not active after install."
  err "Run: systemctl status despatch --no-pager"
  exit 1
fi
if ! wait_for_condition "despatch mailsec service state" 20 1 "${PREFIX[@]}" systemctl is-active --quiet despatch-mailsec; then
  err "despatch-mailsec service is not active after install."
  err "Run: systemctl status despatch-mailsec --no-pager"
  exit 1
fi

step "Updater runtime readiness verification"
if ! wait_for_condition "despatch updater path state" 20 1 "${PREFIX[@]}" systemctl is-active --quiet despatch-updater.path; then
  warn "despatch-updater.path is not active after initial start; attempting one-time recovery."
  "${PREFIX[@]}" rm -f /var/lib/despatch/update/request/update-request*.json /var/lib/despatch/update/lock/update.lock || true
  "${PREFIX[@]}" systemctl reset-failed despatch-updater.service despatch-updater.path || true
  "${PREFIX[@]}" systemctl restart despatch-updater.path || true
  if ! wait_for_condition "despatch updater path state" 20 1 "${PREFIX[@]}" systemctl is-active --quiet despatch-updater.path; then
    err "despatch-updater.path is not active after install."
    err "Run: systemctl status despatch-updater.path --no-pager"
    err "Run: systemctl status despatch-updater.service --no-pager"
    exit 1
  fi
fi
if ! run_as_despatch sh -c "test -w /var/lib/despatch/update/request && test -x /var/lib/despatch/update/request"; then
  err "despatch user cannot access /var/lib/despatch/update/request with write+search permissions."
  err "Run: chown despatch:despatch /var/lib/despatch && chmod 0750 /var/lib/despatch"
  err "Run: chown root:despatch /var/lib/despatch/update /var/lib/despatch/update/request"
  err "Run: chmod 0750 /var/lib/despatch/update && chmod 0770 /var/lib/despatch/update/request"
  exit 1
fi
if ! run_as_despatch sh -c "test -w /var/lib/despatch/update/status && test -x /var/lib/despatch/update/status"; then
  err "despatch user cannot access /var/lib/despatch/update/status with write+search permissions."
  err "Run: chown despatch:despatch /var/lib/despatch && chmod 0750 /var/lib/despatch"
  err "Run: chown root:despatch /var/lib/despatch/update /var/lib/despatch/update/status"
  err "Run: chmod 0750 /var/lib/despatch/update && chmod 0770 /var/lib/despatch/update/status"
  exit 1
fi

step "Local service health verification"
if ! wait_for_condition "local app health endpoint" 20 1 verify_direct_access "$LISTEN_ADDR"; then
  err "Local app health check failed on ${LISTEN_ADDR}."
  err "Run: /opt/despatch/despatch (or check /opt/despatch/.env and service logs)"
  err "Run: journalctl -u despatch -n 100 --no-pager"
  exit 1
fi

step "Auth path sanity verification"
LOCAL_BASE_URL="$(local_base_url_from_listen "$LISTEN_ADDR")"
if ! wait_for_condition "auth/setup API sanity check" 20 1 verify_setup_status "$LOCAL_BASE_URL"; then
  err "Auth/setup API sanity check failed on local endpoint."
  err "Run: journalctl -u despatch -n 100 --no-pager"
  exit 1
fi

step "Final updater watcher verification"
if ! wait_for_condition "despatch updater path state" 20 1 "${PREFIX[@]}" systemctl is-active --quiet despatch-updater.path; then
  warn "despatch-updater.path did not remain active through install checks; attempting final recovery."
  "${PREFIX[@]}" rm -f /var/lib/despatch/update/request/update-request*.json /var/lib/despatch/update/lock/update.lock || true
  "${PREFIX[@]}" systemctl reset-failed despatch-updater.service despatch-updater.path || true
  "${PREFIX[@]}" systemctl enable --now despatch-updater.path || true
  if ! wait_for_condition "despatch updater path state" 20 1 "${PREFIX[@]}" systemctl is-active --quiet despatch-updater.path; then
    err "despatch-updater.path is not active after final install verification."
    err "Run: systemctl status despatch-updater.path --no-pager"
    err "Run: systemctl status despatch-updater.service --no-pager"
    exit 1
  fi
fi

if [[ "$CAPTCHA_ENABLED" == "true" && "$CAPTCHA_PROVIDER" == "cap" ]]; then
  if [[ "$DEPLOY_MODE" != "proxy" ]]; then
    err "CAPTCHA_PROVIDER=cap requires reverse proxy mode with /cap/ routing."
    err "Current deploy mode is ${DEPLOY_MODE}. Reconfigure proxy or disable CAP provider."
    exit 1
  fi
  if [[ -z "$CAPTCHA_CAP_PROXY_UPSTREAM" ]]; then
    err "CAP upstream host:port is required for CAP smoke checks."
    err "Expected non-empty CAPTCHA_CAP_PROXY_UPSTREAM."
    exit 1
  fi

  if [[ "$CAPTCHA_CAP_AUTO_PROVISION" == "1" ]]; then
    step "CAP standalone runtime provisioning"
    if ! wait_for_condition "CAP runtime auto-provision" 3 1 provision_cap_runtime "$CAPTCHA_CAP_PROXY_UPSTREAM" "$CAPTCHA_CAP_IMAGE" "$CAPTCHA_CAP_ADMIN_KEY" "$CAPTCHA_CAP_WIDGET_VERSION" "$CAPTCHA_CAP_WASM_VERSION"; then
      err "CAP runtime auto-provisioning failed."
      err "Run: docker logs --tail 200 cap"
      err "Run: docker ps -a | grep cap"
      err "Run: cat /opt/cap/docker-compose.yml"
      exit 1
    fi
  fi

  step "CAP public captcha config verification"
  if ! wait_for_condition "CAP public config endpoint" 20 1 verify_cap_public_config "$LOCAL_BASE_URL" "$CAPTCHA_SITE_KEY"; then
    err "Public captcha config endpoint does not report expected CAP settings."
    err "Run: curl -s ${LOCAL_BASE_URL}/api/v1/public/captcha/config"
    err "Check /opt/despatch/.env CAPTCHA_* values and restart despatch."
    exit 1
  fi

  step "CAP upstream assets and siteverify verification"
  if ! wait_for_condition "CAP upstream assets + siteverify" 25 1 verify_cap_upstream_access "$CAPTCHA_CAP_PROXY_UPSTREAM" "$CAPTCHA_SITE_KEY" "$CAPTCHA_SECRET"; then
    err "CAP upstream smoke check failed."
    err "Ensure CAP standalone serves /assets/widget.js, /assets/cap_wasm.js, /assets/cap_wasm_bg.wasm."
    err "Ensure CAP standalone ENABLE_ASSETS_SERVER=true and pinned WIDGET_VERSION/WASM_VERSION are valid."
    err "Run: curl -i http://${CAPTCHA_CAP_PROXY_UPSTREAM}/assets/widget.js"
    err "Run: curl -i http://${CAPTCHA_CAP_PROXY_UPSTREAM}/${CAPTCHA_SITE_KEY}/siteverify -H 'Content-Type: application/json' -d '{\"secret\":\"***\",\"response\":\"probe\"}'"
    exit 1
  fi
fi

if [[ "$DEPLOY_MODE" == "proxy" ]]; then
  step "Reverse proxy health verification"
  if ! wait_for_condition "reverse proxy route health" 25 1 verify_proxy_access "$PROXY_SERVER_NAME" "$PROXY_TLS"; then
    warn "Reverse proxy health check failed for ${PROXY_SERVER_NAME}."
    warn "Trying automatic fallback to direct mode."
    if fallback_to_direct_mode; then
      if ! wait_for_condition "local app health endpoint after fallback" 20 1 verify_direct_access "$LISTEN_ADDR"; then
        err "Fallback succeeded but app health still failing."
        err "Run: journalctl -u despatch -n 100 --no-pager"
        exit 1
      fi
      warn "Proxy path failed; installer completed in direct mode."
    else
      err "Reverse proxy health check failed and fallback to direct mode failed."
      if [[ "$PROXY_SERVER" == "nginx" ]]; then
        err "Run: nginx -t && systemctl status nginx --no-pager"
      else
        err "Run: apache2ctl configtest && systemctl status apache2 --no-pager"
      fi
      err "Run: systemctl status despatch --no-pager"
      err "Run: journalctl -u despatch -n 100 --no-pager"
      err "Run: bash $ROOT_DIR/scripts/diagnose_access.sh"
      exit 1
    fi
  fi
  if [[ "$CAPTCHA_ENABLED" == "true" && "$CAPTCHA_PROVIDER" == "cap" ]]; then
    step "CAP reverse proxy route verification"
    if ! wait_for_condition "CAP proxied assets + siteverify" 25 1 verify_cap_proxy_access "$PROXY_SERVER_NAME" "$PROXY_TLS" "$CAPTCHA_SITE_KEY" "$CAPTCHA_SECRET"; then
      err "CAP proxy route smoke check failed."
      err "Verify reverse proxy keeps /cap/ mapped to ${CAPTCHA_CAP_PROXY_UPSTREAM} and does not rewrite CAP asset paths."
      err "Run: curl -i -H 'Host: ${PROXY_SERVER_NAME}' http://127.0.0.1/cap/assets/widget.js"
      err "Run: curl -i -H 'Host: ${PROXY_SERVER_NAME}' http://127.0.0.1/cap/${CAPTCHA_SITE_KEY}/siteverify -H 'Content-Type: application/json' -d '{\"secret\":\"***\",\"response\":\"probe\"}'"
      exit 1
    fi
  fi
fi
finish_stage_ok

begin_stage "final_summary" "Final Summary" "0"
SERVER_IP="$(detect_primary_ip)"
cat <<DONE

Open in browser:
DONE
if [[ "$DEPLOY_MODE" == "proxy" ]]; then
  if [[ "$PROXY_TLS" == "1" ]]; then
    cat <<DONE
  https://${PROXY_SERVER_NAME}
DONE
  else
    cat <<DONE
  http://${PROXY_SERVER_NAME}
DONE
  fi
else
  cat <<DONE
  http://${SERVER_IP}:8080
DONE
fi
cat <<DONE

First-run OOBE is web-only and will ask for:
  - domain (pre-filled: ${BASE_DOMAIN})
  - admin email (default: webmaster@${BASE_DOMAIN})
  - admin password
Cookie policy:
  - COOKIE_SECURE_MODE=${COOKIE_SECURE_MODE}
DONE

if [[ "$CAPTCHA_ENABLED" == "true" && "$CAPTCHA_PROVIDER" == "cap" ]]; then
  cat <<DONE
CAPTCHA:
  - CAP widget endpoint: ${CAPTCHA_WIDGET_API_URL}
  - CAP verify endpoint: ${CAPTCHA_VERIFY_URL}
  - CAP upstream: ${CAPTCHA_CAP_PROXY_UPSTREAM}
DONE
  if [[ "$CAPTCHA_CAP_AUTO_PROVISION" == "1" ]]; then
    cat <<DONE
  - CAP runtime auto-provisioned in /opt/cap (container: cap, volume: cap-data)
  - CAP runtime credentials: /opt/cap/.env
DONE
  fi
fi

if prompt_yes_no "Run Internet accessibility diagnostics now?" 1; then
  bash "$ROOT_DIR/scripts/diagnose_access.sh" || true
fi
finish_stage_ok
emit_run_result_once "ok" "" "0"
