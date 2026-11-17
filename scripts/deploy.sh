#!/usr/bin/env bash
# Deploy a commit to the live bot: test it in a scratch checkout, check Beeper still behaves
# as doc/beeper-api.md says, pin .live to it, restart, and roll back if it does not stay up.
#   mise run deploy                    deploy HEAD
#   mise run deploy <commit>           deploy, or roll back to, a specific commit
#   mise run deploy -- --check [ref]   run every check, change nothing
set -euo pipefail
cd "$(dirname "$(realpath "$0")")/.."
root="$PWD"

check_only=""
if [[ "${1:-}" == "--check" ]]; then
  check_only=1
  shift
fi

# Resolve the interpreter here, at the trusted repo root, so nothing depends on how mise
# treats the mise.toml inside a scratch checkout or .live.
python="$(python3 -c 'import sys; print(sys.executable)')"

target="$(git rev-parse --verify "${1:-HEAD}^{commit}")"
echo "Target: $(git log -1 --format='%h %s' "$target")"
if ! git show "$target:mise.toml" | grep -q '\./\.live/scripts/run-gentalent-live\.sh'; then
  echo "$target predates the pinned .live layout; deploying it would run the working tree." >&2
  exit 1
fi

scratch="$(mktemp -d)"
cleanup() {
  git worktree remove --force "$scratch/code" >/dev/null 2>&1 || true
  rm -rf "$scratch"
  git worktree prune
}
trap cleanup EXIT
git worktree add --quiet --detach "$scratch/code" "$target"
if ! (cd "$scratch/code" && "$python" -m unittest discover -s tests >"$scratch/tests.log" 2>&1); then
  tail -n 30 "$scratch/tests.log"
  echo "Tests failed at $target; live bot untouched." >&2
  exit 1
fi
echo "Tests passed."
(cd "$scratch/code" && "$python" -m gentalent --config "$root/config.json" doctor --contract)

live_db="$("$python" -c 'import json, pathlib, sys
from pathlib import Path
config = Path(sys.argv[1]).resolve()
p = Path(json.loads(config.read_text())["database"]).expanduser()
print(p if p.is_absolute() else config.parent / p)' "$root/config.json")"

bot_idle() {
  # The worker holds a write transaction while handling a message, including the minutes a
  # !!gt photo search takes. Restarting then would roll it back and repeat the AI call.
  "$python" -c 'import sqlite3, sys
from pathlib import Path
db = sqlite3.connect(Path(sys.argv[1]).as_uri() + "?mode=rw", uri=True, timeout=0.5)
try:
    db.execute("BEGIN IMMEDIATE")
    db.rollback()
except sqlite3.OperationalError:
    sys.exit(1)' "$live_db"
}
if ! bot_idle; then
  echo "The live bot is mid-operation (probably generating photos). Try again in a few minutes." >&2
  exit 1
fi

if [[ -e .live && "$(git -C .live rev-parse --show-toplevel 2>/dev/null)" != "$root/.live" ]]; then
  echo ".live exists but is not a git worktree; refusing to touch it." >&2
  exit 1
fi
previous="$(git -C .live rev-parse HEAD 2>/dev/null || true)"

if [[ -n "$check_only" ]]; then
  echo "Check only: would pin .live ${previous:+from ${previous:0:7} }to ${target:0:7} and restart."
  exit 0
fi

daemon_pid() {
  mise daemons status --json 2>/dev/null | "$python" -c '
import json, sys
text, decoder, i = sys.stdin.read(), json.JSONDecoder(), 0
while i < len(text):
    if text[i].isspace():
        i += 1
        continue
    item, i = decoder.raw_decode(text, i)
    if item.get("name") == "gentalent" and item.get("status") == "running":
        print(item.get("pid"))
' || true
}

pin() {
  if [[ -d .live ]]; then
    git -C .live checkout --quiet --detach "$1"
  else
    git worktree add --quiet --detach .live "$1"
  fi
}

restart() {
  # A crashing start can make this exit non-zero or retry indefinitely; the steady check
  # below decides, so neither may skip the rollback.
  timeout 120 mise daemons restart gentalent || true
}

steady_pid() {
  local old="$1" pid
  for _ in $(seq 1 12); do
    sleep 5
    pid="$(daemon_pid)"
    if [[ -n "$pid" && "$pid" != "$old" ]]; then
      sleep 10
      [[ "$(daemon_pid)" == "$pid" ]] && echo "$pid" && return 0
    fi
  done
  return 1
}

old_pid="$(daemon_pid)"
pin "$target"
restart
if pid="$(steady_pid "$old_pid")"; then
  echo "Live bot running $(git -C .live log -1 --format='%h %s') (pid $pid)."
  exit 0
fi

echo "Live bot did not stay up on ${target:0:7}." >&2
mise daemons logs gentalent -n 20 >&2 || true
if [[ -z "$previous" ]]; then
  echo "No earlier pinned commit to roll back to; .live stays on ${target:0:7}." >&2
  exit 1
fi
echo "Rolling .live back to ${previous:0:7}." >&2
failed_pid="$(daemon_pid)"
pin "$previous"
restart
if pid="$(steady_pid "$failed_pid")"; then
  echo "Rolled back; live bot running ${previous:0:7} (pid $pid)." >&2
else
  echo "Rollback did not come up either. Check: mise daemons logs gentalent" >&2
fi
exit 1
