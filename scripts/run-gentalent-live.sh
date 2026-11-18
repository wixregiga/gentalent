#!/usr/bin/env bash
# Starts the pinned live copy of GenTalent. The mise daemon runs this from the repo root as
#   ./.live/scripts/run-gentalent-live.sh ./config.json
# so the code that runs is always the commit checked out in .live, never the working tree.
# scripts/deploy.sh builds that commit into .live/bin/gentalent when it pins it.
set -euo pipefail

unset ELECTRON_RUN_AS_NODE
export PATH="$HOME/.local/share/mise/shims:$HOME/.local/bin:$PATH"

config="$(realpath "${1:?usage: run-gentalent-live.sh CONFIG_JSON}")"
cd "$(dirname "$(realpath "$0")")/.."
bot="$PWD/bin/gentalent"
if [[ ! -x "$bot" ]]; then
  echo "$bot is missing; deploy this commit with mise run deploy." >&2
  exit 1
fi
gentalent() { "$bot" --config "$config" "$@"; }

for _ in $(seq 1 30); do
  gentalent doctor --chat >/dev/null 2>&1 && break
  sleep 1
done

gentalent arm --live --if-unarmed
echo "Running commit $(git rev-parse --short HEAD)."
exec "$bot" --config "$config" run --live
