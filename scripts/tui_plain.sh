#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
BRIDGE_SCRIPT="$ROOT_DIR/scripts/launchpad_bridge.sh"

have_cmd() { command -v "$1" >/dev/null 2>&1; }

pause() {
  printf '\nPress Enter to continue...'
  read -r _
}

show_header() {
  clear
  cat <<'EOF'
+--------------------------------------------------------------+
|                       DESPATCH CONSOLE                       |
|              Plain Shell (no extra dependencies)             |
+--------------------------------------------------------------+
EOF
}

service_state() {
  if have_cmd systemctl; then
    systemctl is-active despatch 2>/dev/null || true
  else
    printf 'n/a'
  fi
}

show_status() {
  show_header
  if [ -x "$BRIDGE_SCRIPT" ]; then
    sh "$BRIDGE_SCRIPT" --operation status || true
  else
    echo "Launchpad bridge not found: $BRIDGE_SCRIPT"
  fi
  pause
}

show_ports() {
  show_header
  echo "Local IMAP/SMTP listeners (25,143,465,587,993)"
  echo
  if have_cmd ss; then
    ss -ltnp | awk 'NR==1 || /:25 |:143 |:465 |:587 |:993 /'
  elif have_cmd netstat; then
    netstat -ltnp 2>/dev/null | awk 'NR==1 || /:25 |:143 |:465 |:587 |:993 /'
  else
    echo "Neither ss nor netstat is available."
  fi
  pause
}

run_install() {
  show_header
  if [ -x "$BRIDGE_SCRIPT" ]; then
    sh "$BRIDGE_SCRIPT" --operation install
  else
    echo "Launchpad bridge not found: $BRIDGE_SCRIPT"
  fi
  pause
}

run_uninstall() {
  show_header
  if [ -x "$BRIDGE_SCRIPT" ]; then
    sh "$BRIDGE_SCRIPT" --operation uninstall
  else
    echo "Launchpad bridge not found: $BRIDGE_SCRIPT"
  fi
  pause
}

run_diagnose() {
  show_header
  if [ -x "$BRIDGE_SCRIPT" ]; then
    sh "$BRIDGE_SCRIPT" --operation diagnose || true
  else
    echo "Launchpad bridge not found: $BRIDGE_SCRIPT"
  fi
  pause
}

main_menu() {
  while :; do
    show_header
    echo "Service state: $(service_state)"
    echo
    echo "1) Install / Upgrade Despatch"
    echo "2) Uninstall Despatch (safe)"
    echo "3) Service Status"
    echo "4) Mail Port Probe"
    echo "5) Diagnose Internet Access"
    echo "6) Quit"
    echo
    printf 'Select an option [1-6]: '
    read -r choice
    case "${choice:-}" in
      1) run_install ;;
      2) run_uninstall ;;
      3) show_status ;;
      4) show_ports ;;
      5) run_diagnose ;;
      6) exit 0 ;;
      *) echo "Invalid option"; sleep 1 ;;
    esac
  done
}

main_menu
