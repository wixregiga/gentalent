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

# Resolve the tools here, at the trusted repo root, so nothing depends on how mise treats the
# mise.toml inside a scratch checkout or .live.
go="$(go env GOROOT)/bin/go"
if ! jq="$(command -v jq)"; then
  echo "jq is missing, so the deploy cannot tell whether the bot stays up. Run mise install." >&2
  exit 1
fi

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
if ! git show "$target:scripts/run-gentalent-live.sh" 2>/dev/null | grep -q 'bin/gentalent'; then
  echo "${target:0:7} does not start .live/bin/gentalent, the binary the deploy tests and installs." >&2
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
gentalent="$scratch/bin/gentalent"
if ! (cd "$scratch/code" && "$go" build -o "$gentalent" ./cmd/gentalent && "$go" vet ./... && "$go" test ./...) >"$scratch/tests.log" 2>&1; then
  tail -n 30 "$scratch/tests.log"
  echo "Tests failed at $target; live bot untouched." >&2
  exit 1
fi
echo "Tests passed."

"$gentalent" --config "$root/config.json" doctor --contract

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

# Ask the running bot's own binary: its searches are the ones a restart abandons, and it reads the
# databases at the schema it migrated them to. Before the first deploy there is none to ask.
running="$gentalent"
if [[ -x .live/bin/gentalent ]]; then
  running="$root/.live/bin/gentalent"
fi
if ! "$running" --config "$root/config.json" busy --live; then
  # A restart abandons a running photo or who search for good (it is never rerun).
  echo "The live bot is mid-operation (an AI search is running). Try again in a few minutes." >&2
  exit 1
fi

if [[ -n "$check_only" ]]; then
  echo "Check only: would pin .live ${previous:+from ${previous:0:7} }to ${target:0:7} and restart."
  exit 0
fi

daemon_pid() {
  mise daemons status --json 2>/dev/null \
    | "$jq" -r 'select(.name == "gentalent" and .status == "running") | .pid // empty' 2>/dev/null || true
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

# The backup runs this checkout's own gentalent, so a rollback to a commit from before the
# backup command still backs up.
"$go" run ./cmd/gentalent --config "$root/config.json" backup --live --label "before-${target:0:7}"

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
