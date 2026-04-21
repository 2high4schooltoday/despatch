#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PROJECT_DIR=$ROOT_DIR/packaging/despatch-installer

if [ -n "${DESPATCH_LAUNCHPAD_BIN:-}" ] && [ -x "${DESPATCH_LAUNCHPAD_BIN}" ]; then
  exec "${DESPATCH_LAUNCHPAD_BIN}" run "$PROJECT_DIR" "$@"
fi

if command -v launchpad >/dev/null 2>&1; then
  exec launchpad run "$PROJECT_DIR" "$@"
fi

if [ -n "${HOME:-}" ] && [ -x "$HOME/.cargo/bin/launchpad" ]; then
  exec "$HOME/.cargo/bin/launchpad" run "$PROJECT_DIR" "$@"
fi

printf 'Despatch now runs through Launchpad, but no Launchpad CLI was found.\n' >&2
printf 'Install Launchpad or run the packaged standalone installer from packaging/despatch-installer.\n' >&2
exit 1
