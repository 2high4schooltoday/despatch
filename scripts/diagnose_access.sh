#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="/opt/despatch/.env"
if [[ ! -f "$ENV_FILE" ]]; then
  ENV_FILE="$ROOT_DIR/.env"
fi

have_cmd() { command -v "$1" >/dev/null 2>&1; }
trim() {
  local s="$1"
  s="$(echo "$s" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  printf '%s' "$s"
}

lower() {
  echo "$1" | tr '[:upper:]' '[:lower:]'
}

truthy() {
  local v
  v="$(lower "$(trim "${1:-}")")"
  case "$v" in
    1|y|yes|true|on) return 0 ;;
  esac
  return 1
}

get_env() {
  local key="$1" default="${2:-}" line
  if [[ -f "$ENV_FILE" ]]; then
    line="$(grep -E "^${key}=" "$ENV_FILE" | tail -n1 || true)"
    if [[ -n "$line" ]]; then
      printf '%s' "${line#*=}"
      return
    fi
  fi
  printf '%s' "$default"
}

first_error=""
record_fail() {
  local code="$1" reason="$2"
  if [[ -z "$first_error" ]]; then
    first_error="${code}:${reason}"
  fi
  printf '[FAIL] %s\n' "$reason"
}

record_ok() {
  printf '[ OK ] %s\n' "$1"
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

service_state() {
  local unit="$1" state=""
  state="$(systemctl is-active "$unit" 2>/dev/null || true)"
  state="$(trim "$state")"
  if [[ -z "$state" ]]; then
    state="unknown"
  fi
  printf '%s' "$state"
}

wait_for_active_or_terminal() {
  local unit="$1" tries="$2" sleep_sec="$3"
  local state="" i
  for ((i = 1; i <= tries; i++)); do
    state="$(service_state "$unit")"
    if [[ "$state" == "active" ]]; then
      printf '%s' "$state"
      return 0
    fi
    if [[ "$state" != "activating" ]]; then
      printf '%s' "$state"
      return 1
    fi
    sleep "$sleep_sec"
  done
  printf '%s' "activating"
  return 1
}

DEPLOY_MODE="$(trim "$(get_env DEPLOY_MODE "")")"
LISTEN_ADDR="$(trim "$(get_env LISTEN_ADDR ":8080")")"
BASE_DOMAIN="$(trim "$(get_env BASE_DOMAIN "")")"
PROXY_TLS="$(trim "$(get_env PROXY_TLS "")")"
COOKIE_SECURE_MODE="$(lower "$(trim "$(get_env COOKIE_SECURE_MODE "")")")"
COOKIE_SECURE_LEGACY="$(trim "$(get_env COOKIE_SECURE "")")"
if [[ -z "$COOKIE_SECURE_MODE" ]]; then
  if truthy "$COOKIE_SECURE_LEGACY"; then
    COOKIE_SECURE_MODE="always"
  else
    COOKIE_SECURE_MODE="never"
  fi
fi
if [[ -z "$DEPLOY_MODE" ]]; then
  if [[ "$LISTEN_ADDR" == 127.0.0.1:* ]]; then
    DEPLOY_MODE="proxy"
  else
    DEPLOY_MODE="direct"
  fi
fi

PROXY_SERVER=""
if [[ -f /etc/nginx/sites-available/despatch.conf || -L /etc/nginx/sites-enabled/despatch.conf ]]; then
  PROXY_SERVER="nginx"
elif [[ -f /etc/apache2/sites-available/despatch.conf || -L /etc/apache2/sites-enabled/despatch.conf ]]; then
  PROXY_SERVER="apache2"
fi
if [[ -z "$PROXY_TLS" ]]; then
  if [[ "$PROXY_SERVER" == "nginx" ]]; then
    if grep -Eq 'listen[[:space:]]+443' /etc/nginx/sites-available/despatch.conf 2>/dev/null; then
      PROXY_TLS="1"
    fi
  elif [[ "$PROXY_SERVER" == "apache2" ]]; then
    if grep -Eq 'SSLEngine[[:space:]]+on|VirtualHost[[:space:]]+\*:443' /etc/apache2/sites-available/despatch.conf 2>/dev/null; then
      PROXY_TLS="1"
    fi
  fi
fi
if [[ -z "$PROXY_TLS" ]]; then
  PROXY_TLS="0"
fi

printf '=== Despatch Internet Access Diagnosis ===\n'
printf 'env_file=%s\n' "$ENV_FILE"
printf 'deploy_mode=%s\n' "$DEPLOY_MODE"
printf 'listen_addr=%s\n' "$LISTEN_ADDR"
if [[ -n "$BASE_DOMAIN" ]]; then
  printf 'base_domain=%s\n' "$BASE_DOMAIN"
fi
printf 'cookie_secure_mode=%s\n' "$COOKIE_SECURE_MODE"
if [[ -n "$PROXY_SERVER" ]]; then
  printf 'proxy_server=%s\n' "$PROXY_SERVER"
  printf 'proxy_tls=%s\n' "$PROXY_TLS"
fi
printf '\n'

LOCAL_HEALTH="$(health_url_from_listen "$LISTEN_ADDR")"

if have_cmd systemctl; then
  service_state_val="$(wait_for_active_or_terminal despatch 30 1 || true)"
  if [[ "$service_state_val" == "active" ]]; then
    record_ok "despatch service is active"
  elif [[ "$service_state_val" == "activating" ]] && have_cmd curl && curl -fsS --max-time 6 "$LOCAL_HEALTH" >/dev/null; then
    record_ok "despatch service is activating but app health is already reachable (${LOCAL_HEALTH})"
  else
    record_fail 10 "APP_DOWN: despatch service is inactive (is-active=${service_state_val})"
  fi
else
  printf '[WARN] systemctl not found; service state cannot be verified.\n'
fi

if have_cmd curl && curl -fsS --max-time 6 "$LOCAL_HEALTH" >/dev/null; then
  record_ok "local app health check passed (${LOCAL_HEALTH})"
else
  record_fail 10 "APP_DOWN: local app health check failed (${LOCAL_HEALTH})"
fi

READY_URL="${LOCAL_HEALTH%/health/live}/health/ready"
if have_cmd curl; then
  ready_json="$(curl -fsS --max-time 8 "$READY_URL" || true)"
  if [[ -n "${ready_json:-}" ]]; then
    if echo "$ready_json" | grep -q '"imap"[[:space:]]*:[[:space:]]*{[^}]*"ok"[[:space:]]*:[[:space:]]*false'; then
      printf '[WARN] mail readiness: IMAP probe failed in /health/ready\n'
      printf '       Check IMAP_HOST/IMAP_PORT/IMAP_TLS/IMAP_STARTTLS and cert verification settings.\n'
    fi
    if echo "$ready_json" | grep -q '"smtp"[[:space:]]*:[[:space:]]*{[^}]*"ok"[[:space:]]*:[[:space:]]*false'; then
      printf '[WARN] mail readiness: SMTP probe failed in /health/ready\n'
      printf '       Check SMTP_HOST/SMTP_PORT/SMTP_TLS/SMTP_STARTTLS and cert verification settings.\n'
    fi
  fi
fi

if [[ "$DEPLOY_MODE" == "proxy" ]]; then
  if [[ -z "$PROXY_SERVER" ]]; then
    record_fail 20 "PROXY_DOWN: despatch proxy config not found for nginx/apache2"
  fi

  if [[ "$PROXY_SERVER" == "nginx" ]]; then
    if have_cmd systemctl && systemctl is-active --quiet nginx; then
      record_ok "nginx service is active"
    else
      record_fail 20 "PROXY_DOWN: nginx service inactive"
    fi
  elif [[ "$PROXY_SERVER" == "apache2" ]]; then
    if have_cmd systemctl && systemctl is-active --quiet apache2; then
      record_ok "apache2 service is active"
    else
      record_fail 20 "PROXY_DOWN: apache2 service inactive"
    fi
  fi

  if have_cmd curl && [[ -n "$BASE_DOMAIN" ]]; then
    if curl -kfsS --max-time 6 --resolve "${BASE_DOMAIN}:443:127.0.0.1" "https://${BASE_DOMAIN}/health/live" >/dev/null \
      || curl -fsS --max-time 6 -H "Host: ${BASE_DOMAIN}" "http://127.0.0.1/health/live" >/dev/null; then
      record_ok "proxy route health check passed for ${BASE_DOMAIN}"
    else
      record_fail 21 "PROXY_MISROUTE: proxy does not route ${BASE_DOMAIN} to app health endpoint"
    fi
  else
    printf '[WARN] curl or BASE_DOMAIN missing; proxy route check skipped.\n'
  fi

  if [[ "$PROXY_TLS" == "1" && -n "$BASE_DOMAIN" ]]; then
    if have_cmd curl; then
      redirect_location="$(curl -ksSI --max-time 6 -H "Host: ${BASE_DOMAIN}" "http://127.0.0.1/" | awk -F': ' '/^Location:/ {print $2}' | tr -d '\r' | head -n1 || true)"
      if [[ "$redirect_location" =~ ^https:// ]]; then
        record_ok "HTTP -> HTTPS redirect is active for ${BASE_DOMAIN}"
      else
        record_fail 22 "TLS_REDIRECT_MISSING: http://${BASE_DOMAIN}/ does not redirect to HTTPS"
      fi
    fi

    if [[ "$PROXY_SERVER" == "apache2" ]] && have_cmd apache2ctl; then
      apache_dump="$(apache2ctl -S 2>/dev/null || true)"
      if echo "$apache_dump" | grep -Eqi "port 443 namevhost[[:space:]]+${BASE_DOMAIN}([[:space:]]|$)"; then
        record_ok "apache2 has dedicated :443 vhost for ${BASE_DOMAIN}"
      else
        record_fail 22 "TLS_VHOST_MISSING: apache2 has no explicit :443 vhost for ${BASE_DOMAIN}"
      fi
    fi

    if have_cmd openssl; then
      cert_san="$(echo | openssl s_client -connect 127.0.0.1:443 -servername "${BASE_DOMAIN}" 2>/dev/null | openssl x509 -noout -ext subjectAltName 2>/dev/null || true)"
      if echo "$cert_san" | grep -Fq "DNS:${BASE_DOMAIN}"; then
        record_ok "TLS certificate SAN covers ${BASE_DOMAIN}"
      else
        record_fail 22 "TLS_CERT_MISMATCH: certificate on 443 does not include SAN for ${BASE_DOMAIN}"
      fi
    fi
  fi
fi

printf '\n--- Cookie policy ---\n'
if [[ "$DEPLOY_MODE" == "direct" ]]; then
  if [[ "$COOKIE_SECURE_MODE" == "always" ]]; then
    record_fail 50 "COOKIE_POLICY_MISMATCH: direct HTTP mode cannot use secure-only cookies"
    printf '       Fix: set COOKIE_SECURE_MODE=never in %s and restart despatch.\n' "$ENV_FILE"
    printf '       Command: sudo sed -i \"s/^COOKIE_SECURE_MODE=.*/COOKIE_SECURE_MODE=never/\" %s && sudo systemctl restart despatch\n' "$ENV_FILE"
  else
    record_ok "direct-mode cookie policy is compatible"
  fi
fi

if [[ "$DEPLOY_MODE" == "proxy" ]]; then
  if [[ "$PROXY_TLS" != "1" && "$COOKIE_SECURE_MODE" == "always" ]]; then
    record_fail 50 "COOKIE_POLICY_MISMATCH: HTTP-only proxy mode cannot use secure-only cookies"
    printf '       Fix: enable TLS proxy or set COOKIE_SECURE_MODE=never in %s, then restart despatch.\n' "$ENV_FILE"
    printf '       Command: sudo sed -i \"s/^COOKIE_SECURE_MODE=.*/COOKIE_SECURE_MODE=never/\" %s && sudo systemctl restart despatch\n' "$ENV_FILE"
  elif [[ "$PROXY_TLS" == "1" && "$COOKIE_SECURE_MODE" != "always" ]]; then
    record_fail 50 "COOKIE_POLICY_MISMATCH: HTTPS proxy mode should use COOKIE_SECURE_MODE=always"
    printf '       Fix: set COOKIE_SECURE_MODE=always in %s and restart despatch.\n' "$ENV_FILE"
    printf '       Command: sudo sed -i \"s/^COOKIE_SECURE_MODE=.*/COOKIE_SECURE_MODE=always/\" %s && sudo systemctl restart despatch\n' "$ENV_FILE"
  else
    record_ok "proxy-mode cookie policy is compatible"
  fi
fi

printf '\n--- Listening ports ---\n'
if have_cmd ss; then
  ss -ltnp | awk 'NR==1 || /:80 |:443 |:8080 |:25 |:143 |:465 |:587 |:993 /'
elif have_cmd netstat; then
  netstat -ltnp 2>/dev/null | awk 'NR==1 || /:80 |:443 |:8080 |:25 |:143 |:465 |:587 |:993 /'
else
  printf '[WARN] ss/netstat not available.\n'
fi

printf '\n--- Firewall (ufw) ---\n'
if have_cmd ufw; then
  ufw status || true
  if ufw status 2>/dev/null | grep -q "Status: active"; then
    if [[ "$DEPLOY_MODE" == "proxy" ]]; then
      if ufw status 2>/dev/null | grep -Eq '(^|[[:space:]])80(/tcp)?[[:space:]].*ALLOW' \
        && ufw status 2>/dev/null | grep -Eq '(^|[[:space:]])443(/tcp)?[[:space:]].*ALLOW'; then
        record_ok "ufw allows 80/tcp and 443/tcp"
      else
        record_fail 30 "PORT_BLOCKED: ufw active but missing ALLOW for 80/443"
      fi
    else
      if ufw status 2>/dev/null | grep -Eq '(^|[[:space:]])8080(/tcp)?[[:space:]].*ALLOW'; then
        record_ok "ufw allows 8080/tcp"
      else
        record_fail 30 "PORT_BLOCKED: ufw active but missing ALLOW for 8080"
      fi
    fi
  fi
else
  printf '[INFO] ufw not installed.\n'
fi

printf '\n--- DNS ---\n'
if [[ -n "$BASE_DOMAIN" ]]; then
  resolved=""
  if have_cmd dig; then
    resolved="$(dig +short A "$BASE_DOMAIN" | head -n1 || true)"
  elif have_cmd getent; then
    resolved="$(getent ahostsv4 "$BASE_DOMAIN" | awk '{print $1}' | head -n1 || true)"
  fi
  resolved="$(trim "$resolved")"
  if [[ -n "$resolved" ]]; then
    printf '[INFO] %s resolves to %s\n' "$BASE_DOMAIN" "$resolved"
    public_ip=""
    if have_cmd curl; then
      public_ip="$(curl -fsS --max-time 4 https://api.ipify.org || true)"
      public_ip="$(trim "$public_ip")"
    fi
    if [[ -n "$public_ip" ]]; then
      printf '[INFO] this host public IP appears to be %s\n' "$public_ip"
      if [[ "$resolved" != "$public_ip" ]]; then
        record_fail 40 "DNS_MISMATCH: ${BASE_DOMAIN} does not resolve to this host public IP"
      else
        record_ok "DNS appears aligned with this host public IP"
      fi
    fi
  else
    printf '[WARN] could not resolve %s (dig/getent unavailable or no DNS record)\n' "$BASE_DOMAIN"
  fi
else
  printf '[WARN] BASE_DOMAIN not set; DNS check skipped.\n'
fi

printf '\n=== Result ===\n'
if [[ -z "$first_error" ]]; then
  printf '[ OK ] Healthy: Internet access path looks good.\n'
  exit 0
fi

code="${first_error%%:*}"
reason="${first_error#*:}"
printf '[FAIL] first_error=%s (%s)\n' "$code" "$reason"
case "$code" in
  10) printf 'Next: check app logs -> journalctl -u despatch -n 100 --no-pager\n' ;;
  20) printf 'Next: check proxy service status/config test.\n' ;;
  21) printf 'Next: verify proxy server_name/vhost routes to 127.0.0.1:8080.\n' ;;
  22) printf 'Next: fix TLS vhost/certificate mapping for BASE_DOMAIN (redirect + SAN + 443 vhost).\n' ;;
  30) printf 'Next: open required firewall ports (ufw/security groups).\n' ;;
  40) printf 'Next: update DNS A record to this server public IP.\n' ;;
  50) printf 'Next: align COOKIE_SECURE_MODE with deploy mode and restart despatch.\n' ;;
esac
exit "$code"
