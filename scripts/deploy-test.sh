#!/usr/bin/env bash
# Runs the real scripts/deploy.sh against a throwaway main checkout in a temp folder and checks
# what it refuses, which commit it pins, which binary it installs and what it backs up. mise is
# stubbed: a start gives the bot a new PID unless .state/crash exists, so the steady check passes
# or fails on demand. The stub bot prints the commit it was built from, is busy while .state/busy
# exists, and logs each backup label to .state/backups.log.
set -euo pipefail

source_root="$(cd "$(dirname "$(realpath "$0")")/.." && pwd)"
go_bin="$(dirname "$(go env GOROOT)/bin/go")"
jq_bin="$(dirname "$(mise which jq)")"
work="$(mktemp -d)"
trap 'git -C "$work/repo" worktree remove --force "$work/repo/.live" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT

mkdir -p "$work/stubs"
cat >"$work/stubs/mise" <<'STUB'
#!/bin/sh
pid_file=.state/daemon-pid
case "$1 $2" in
  "daemons start") echo $(($(cat "$pid_file" 2>/dev/null || echo 100) + 1)) >"$pid_file" ;;
  "daemons status")
    if [ ! -e .state/crash ] && [ -e "$pid_file" ]; then
      printf '{"name":"beeper","status":"running","pid":7}\n{"name":"gentalent","status":"running","pid":%s}\n' "$(cat "$pid_file")"
    fi ;;
esac
exit 0
STUB
printf '#!/bin/sh\nexit 0\n' >"$work/stubs/sleep"
chmod +x "$work/stubs/mise" "$work/stubs/sleep"
tools_path="$work/stubs:$go_bin:/usr/bin:/bin"
export PATH="$tools_path:$jq_bin"
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.invalid
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.invalid

repo="$work/repo"
git init --quiet -b main "$repo"
cd "$repo"
mkdir -p scripts cmd/gentalent
cp "$source_root/scripts/deploy.sh" scripts/deploy.sh
cp "$source_root/scripts/run-gentalent-live.sh" scripts/run-gentalent-live.sh
printf '[daemons.gentalent]\nrun = "exec ./.live/scripts/run-gentalent-live.sh ./config.json"\n' >mise.toml
printf 'module stub\n\ngo 1.27\n' >go.mod
printf '.live/\n.state/\nconfig.json\n/bin/\n*_local.go\n' >.gitignore
bot() {
  cat >cmd/gentalent/main.go <<EOF
package main

import (
	"fmt"
	"os"
	"slices"
)

func main() {
	if _, err := os.Stat(".state/busy"); err == nil && slices.Contains(os.Args, "busy") {
		os.Exit(1)
	}
	if slices.Contains(os.Args, "backup") {
		log, err := os.OpenFile(".state/backups.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			os.Exit(1)
		}
		fmt.Fprintln(log, os.Args[len(os.Args)-1])
		log.Close()
	}
	if slices.Contains(os.Args, "version") {
		fmt.Print("$1")
	}
}
EOF
}
bot first
git add -A
git commit --quiet -m "First bot"
first="$(git rev-parse HEAD)"
bot second
git commit --quiet -am "Second bot"
second="$(git rev-parse HEAD)"
git checkout --quiet -b elsewhere
printf '#!/usr/bin/env bash\nexec ./some-other-program "$@"\n' >scripts/run-gentalent-live.sh
git commit --quiet -am "Start some other program"
elsewhere="$(git rev-parse HEAD)"
git checkout --quiet main

printf '{}\n' >config.json
mkdir -p .state
git worktree add --quiet --detach .live "$first"
"$go_bin/go" -C .live build -o bin/gentalent ./cmd/gentalent

failures=0
code=0
output=""
run_deploy() {
  code=0
  output="$(scripts/deploy.sh "$@" 2>&1)" || code=$?
}
fail() {
  echo "FAIL $1" >&2
  echo "$output" >&2
  failures=$((failures + 1))
}
expect() {
  local name="$1" want_code="$2" want_live="$3" want_text="$4"
  local live
  live="$(git -C .live rev-parse HEAD)"
  if [[ "$code" != "$want_code" || "$live" != "$want_live" || "$output" != *"$want_text"* ]]; then
    fail "$name: exit $code (want $want_code), .live at ${live:0:7} (want ${want_live:0:7})"
  else
    echo "ok   $name"
  fi
}
expect_binary() {
  local name="$1" want="$2" got
  got="$(.live/bin/gentalent version 2>/dev/null || true)"
  if [[ "$got" != "$want" ]]; then
    fail "$name: .live/bin/gentalent is \"$got\", want \"$want\""
  else
    echo "ok   $name"
  fi
}
expect_backups() {
  local name="$1" want="$2" got
  got="$(cat .state/backups.log 2>/dev/null || true)"
  if [[ "$got" != "$want" ]]; then
    fail "$name: backups \"$got\", want \"$want\""
  else
    echo "ok   $name"
  fi
}
live_at() {
  git -C .live checkout --quiet --detach "$1"
  "$go_bin/go" -C .live build -o bin/gentalent ./cmd/gentalent
}

printf 'untested\n' >.live/stray.txt
run_deploy --check "$second"
expect "an untracked file in .live is refused" 1 "$first" ".live has local changes"
rm .live/stray.txt

printf '# edited in place\n' >>.live/mise.toml
run_deploy "$second"
expect "an edited tracked file in .live is refused" 1 "$first" " M mise.toml"
grep -q 'edited in place' .live/mise.toml || fail "the refusal discarded the local edit"
git -C .live checkout --quiet -- mise.toml

code=0
output="$(PATH="$tools_path" scripts/deploy.sh "$second" 2>&1)" || code=$?
expect "a deploy without jq, which could not tell whether the bot stays up, is refused" 1 "$first" "jq is missing"

run_deploy --check "$elsewhere"
expect "a commit whose startup does not run the installed binary is refused" 1 "$first" "does not start .live/bin/gentalent"

touch .state/busy
run_deploy "$second"
expect "a deploy waits while the running bot has an AI search" 1 "$first" "an AI search is running"
rm .state/busy

run_deploy --check "$second"
expect "a check changes nothing" 0 "$first" "Check only: would pin .live from ${first:0:7} to ${second:0:7}"
expect_backups "a check backs up nothing" ""

printf 'package main\n\nimport "fmt"\n\nfunc init() { fmt.Print("LOCAL ") }\n' >.live/cmd/gentalent/extra_local.go
run_deploy "$second"
expect "a deploy pins the target and restarts" 0 "$second" "Live bot running"
expect_binary "the installed binary is the one tested, not one rebuilt in .live" second
expect_backups "a deploy backs up the databases, labelled with its target" "before-${second:0:7}"
rm .live/cmd/gentalent/extra_local.go .state/backups.log

live_at "$first"
touch .state/crash
run_deploy "$second"
expect "a failed deploy rolls back to the previous commit" 1 "$first" "Rolling .live back to ${first:0:7}."
expect_binary "the rollback reinstalls the previous commit's binary" first
rm .state/crash

exit "$failures"
