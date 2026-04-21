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
printf 'Download the verified universal installer from:\n' >&2
printf 'https://github.com/2high4schooltoday/despatch/releases/latest/download/despatch-installer-installer-linux-universal\n' >&2
exit 1
