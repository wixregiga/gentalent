#!/usr/bin/env bash
# Starts the pinned live copy of GenTalent. The mise daemon runs this from the repo root as
#   ./.live/scripts/run-gentalent-live.sh ./config.json
# so the code that runs is always the commit checked out in .live, never the working tree.
set -euo pipefail

unset ELECTRON_RUN_AS_NODE
export PATH="$HOME/.local/share/mise/shims:$HOME/.local/bin:$PATH"
export PYTHONUNBUFFERED=1

config="$(realpath "${1:?usage: run-gentalent-live.sh CONFIG_JSON}")"
# Resolve the interpreter at the repo root (the daemon's working directory) before moving
# into .live, so the mise shim is never asked to read .live/mise.toml.
python="$(python3 -c 'import sys; print(sys.executable)')"
cd "$(dirname "$(realpath "$0")")/.."
gentalent() { "$python" -m gentalent --config "$config" "$@"; }

for _ in $(seq 1 30); do
  gentalent doctor --chat >/dev/null 2>&1 && break
  sleep 1
done

gentalent arm --live --if-unarmed
echo "Running commit $(git rev-parse --short HEAD)."
exec "$python" -u -m gentalent --config "$config" run --live
