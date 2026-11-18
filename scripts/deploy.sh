#!/usr/bin/env bash
# Deploy a commit to the live bot: test it in a scratch checkout, check Beeper still behaves
# as doc/beeper-api.md says, pin .live to it, restart, and roll back if it does not stay up.
#   mise run deploy                    deploy HEAD
#   mise run deploy <commit>           deploy, or roll back to, a specific commit
#   mise run deploy -- --check [ref]   run every check, change nothing
# Runs only from the main checkout, which owns .live, config.json and the daemons. A real
# deploy takes only commits already on main (merge with `mise run ship <branch>`).
set -euo pipefail
cd "$(dirname "$(realpath "$0")")/.."
root="$PWD"
common="$(dirname "$(realpath "$(git rev-parse --git-common-dir)")")"
if [[ "$root" != "$common" ]]; then
  echo "Deploy from the main checkout ($common), not a worktree." >&2
  exit 1
fi

check_only=""
if [[ "${1:-}" == "--check" ]]; then
  check_only=1
  shift
fi

# Resolve the interpreter and the Go toolchain here, at the trusted repo root, so nothing
# depends on how mise treats the mise.toml inside a scratch checkout or .live.
python="$(python3 -c 'import sys; print(sys.executable)')"
go="$(go env GOROOT)/bin/go"

target="$(git rev-parse --verify "${1:-HEAD}^{commit}")"
echo "Target: $(git log -1 --format='%h %s' "$target")"
if [[ -z "$check_only" ]] && ! git merge-base --is-ancestor "$target" main; then
  echo "${target:0:7} is not on main. Merge it first: mise run ship <branch>." >&2
  exit 1
fi
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
go_suite() {
  [[ ! -f go.mod ]] || { "$go" build -o "$scratch/bin/gentalent" ./cmd/gentalent && "$go" vet ./... && "$go" test ./...; }
}
if ! (cd "$scratch/code" && "$python" -m unittest discover -s tests && go_suite) >"$scratch/tests.log" 2>&1; then
  tail -n 30 "$scratch/tests.log"
  echo "Tests failed at $target; live bot untouched." >&2
  exit 1
fi
echo "Tests passed."
gentalent="$scratch/bin/gentalent"
if [[ ! -x "$gentalent" ]]; then
  echo "${target:0:7} has no Go bot to deploy; the live bot runs the Go binary." >&2
  exit 1
fi

databases="$("$python" -c 'import json, sys
from pathlib import Path
config = Path(sys.argv[1]).resolve()
data = json.loads(config.read_text())
for key, default in (("database", ".state/gentalent.sqlite3"),
                     ("nominations_database", ".state/nominations.sqlite3"),
                     ("messaging_database", ".state/messaging.sqlite3")):
    p = Path(data.get(key) or default).expanduser()
    print(p if p.is_absolute() else config.parent / p)' "$root/config.json")"
{ read -r python_db; read -r nominations_db; read -r messaging_db; } <<<"$databases"

starts_python() {
  git show "$1:scripts/run-gentalent-live.sh" 2>/dev/null | grep -q -- '-m gentalent'
}
# A failed migration can leave the file behind without committing any Go state.
messaging_version() {
  if [[ ! -e "$messaging_db" ]]; then
    echo 0
    return
  fi
  "$python" -c 'import sqlite3, sys
from pathlib import Path
with sqlite3.connect(Path(sys.argv[1]).as_uri() + "?mode=ro", uri=True) as db:
    print(db.execute("PRAGMA user_version").fetchone()[0])' "$messaging_db"
}
version="$(messaging_version)"
if (( version > 0 )) && starts_python "$target"; then
  echo "${target:0:7} starts the Python bot, but the Go bot has run ($messaging_db was migrated)." >&2
  echo "The Python bot would answer again every message the Go bot answered. Deploy a commit that starts the Go bot." >&2
  exit 1
fi

if (( version > 0 )); then
  "$gentalent" --config "$root/config.json" doctor --contract
else
  # The Go bot has not started yet. At its first start it imports exactly these Python
  # replies, so the Python check reads the same evidence.
  (cd "$scratch/code" && "$python" -m gentalent --config "$root/config.json" doctor --contract)
fi

if [[ -e .live && "$(git -C .live rev-parse --show-toplevel 2>/dev/null)" != "$root/.live" ]]; then
  echo ".live exists but is not a git worktree; refusing to touch it." >&2
  exit 1
fi
# A checkout keeps local edits, so a dirty .live would run code no test saw.
if [[ -e .live && -n "$(git -C .live status --porcelain --untracked-files=all)" ]]; then
  echo ".live has local changes, so pinning it would run untested code. Nothing was changed." >&2
  git -C .live status --short --untracked-files=all >&2
  echo "Move them out of .live (commit them on a branch and ship it, or delete them), then deploy again." >&2
  exit 1
fi
previous="$(git -C .live rev-parse HEAD 2>/dev/null || true)"

python_idle() {
  # The Python bot holds a write transaction while handling a message, including the minutes
  # a !!gt photo search takes. Restarting then would roll it back and repeat the AI call.
  "$python" -c 'import sqlite3, sys
from pathlib import Path
db = sqlite3.connect(Path(sys.argv[1]).as_uri() + "?mode=rw", uri=True, timeout=0.5)
try:
    db.execute("BEGIN IMMEDIATE")
    db.rollback()
except sqlite3.OperationalError:
    sys.exit(1)' "$python_db"
}
if [[ -n "$previous" ]] && starts_python "$previous"; then
  if ! python_idle; then
    echo "The live Python bot is mid-operation (probably generating photos). Try again in a few minutes." >&2
    exit 1
  fi
elif ! "$gentalent" --config "$root/config.json" busy --live; then
  # A restart abandons a running Go photo search for good (it is never rerun).
  echo "The live bot is mid-operation (a photo search is running). Try again in a few minutes." >&2
  exit 1
fi

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

# pin COMMIT [BINARY]: check out COMMIT in .live and install BINARY, the bot built and tested
# from that commit, as .live/bin/gentalent. Never build in .live: it may hold ignored files.
pin() {
  if [[ -d .live ]]; then
    git -C .live checkout --quiet --detach "$1"
  else
    git worktree add --quiet --detach .live "$1"
  fi
  if [[ -n "${2:-}" ]]; then
    mkdir -p .live/bin
    cp "$2" .live/bin/gentalent.new
    chmod 755 .live/bin/gentalent.new
    mv .live/bin/gentalent.new .live/bin/gentalent
  fi
}

restart() {
  # Not `mise daemons restart`: that is pitchfork's `start --force`, which also force-restarts
  # dependencies, and it restarted the Beeper app on the first deploy. A plain start leaves an
  # already-running Beeper alone. A crashing start can exit non-zero or retry indefinitely;
  # the steady check below decides, so neither may skip the rollback.
  timeout 60 mise daemons stop gentalent || true
  timeout 120 mise daemons start gentalent || true
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

stamp="$(date +%Y%m%d-%H%M%S)"
for db in "$python_db" "$nominations_db" "$messaging_db"; do
  [[ -e "$db" ]] || continue
  backup="$(dirname "$db")/backups/$(basename "$db" .sqlite3)-$stamp-before-${target:0:7}.sqlite3"
  mkdir -p "$(dirname "$backup")"
  "$python" -c 'import sqlite3, sys
src = sqlite3.connect(sys.argv[1]); dst = sqlite3.connect(sys.argv[2])
src.backup(dst); dst.close(); src.close()' "$db" "$backup"
  chmod 600 "$backup"
  echo "Backed up $(basename "$db") to $backup"
done

previous_binary=""
if [[ -x .live/bin/gentalent ]]; then
  previous_binary="$scratch/previous-gentalent"
  cp .live/bin/gentalent "$previous_binary"
fi
old_pid="$(daemon_pid)"
pin "$target" "$gentalent"
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
if starts_python "$previous"; then
  # Stop daemon retries before deciding whether the old cursor is safe to use.
  timeout 60 mise daemons stop gentalent
  version="$(messaging_version)"
fi
if (( version > 0 )) && starts_python "$previous"; then
  cat >&2 <<EOF
Not rolling back to ${previous:0:7}: it starts the Python bot, and the Go bot has run. The Python
cursor in $python_db did not move while the Go bot ran, so the Python bot would answer again
every message the Go bot answered. .live stays on ${target:0:7}.
Instead:
  1. Read why the Go bot stopped: mise daemons logs gentalent -n 50
  2. Fix forward: mise run ship <branch with the fix>, or mise run deploy <an earlier commit
     on main that starts the Go bot>.
EOF
  exit 1
fi
echo "Rolling .live back to ${previous:0:7}." >&2
failed_pid="$(daemon_pid)"
pin "$previous" "$previous_binary"
restart
if pid="$(steady_pid "$failed_pid")"; then
  echo "Rolled back; live bot running ${previous:0:7} (pid $pid)." >&2
else
  echo "Rollback did not come up either. Check: mise daemons logs gentalent" >&2
fi
exit 1