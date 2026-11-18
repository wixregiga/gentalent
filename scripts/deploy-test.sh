#!/usr/bin/env bash
# Runs the real scripts/deploy.sh against a throwaway main checkout in a temp folder and checks
# what it refuses, which commit it pins and which binary it installs. mise is stubbed: a start
# gives the bot a new PID unless .state/crash exists, so the steady check passes or fails on
# demand. The stub bot prints the commit it was built from and is busy while .state/busy exists.
set -euo pipefail

source_root="$(cd "$(dirname "$(realpath "$0")")/.." && pwd)"
go_bin="$(dirname "$(go env GOROOT)/bin/go")"
python_bin="$(dirname "$(python3 -c 'import sys; print(sys.executable)')")"
work="$(mktemp -d)"
holder=""
trap '[[ -z "$holder" ]] || kill "$holder" 2>/dev/null || true; git -C "$work/repo" worktree remove --force "$work/repo/.live" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT

mkdir -p "$work/stubs"
cat >"$work/stubs/mise" <<'STUB'
#!/bin/sh
pid_file=.state/daemon-pid
case "$1 $2" in
  "daemons start") echo $(($(cat "$pid_file" 2>/dev/null || echo 100) + 1)) >"$pid_file" ;;
  "daemons status")
    if [ ! -e .state/crash ] && [ -e "$pid_file" ]; then
      printf '{"name":"gentalent","status":"running","pid":%s}\n' "$(cat "$pid_file")"
    fi ;;
esac
exit 0
STUB
printf '#!/bin/sh\nexit 0\n' >"$work/stubs/sleep"
chmod +x "$work/stubs/mise" "$work/stubs/sleep"
export PATH="$work/stubs:$go_bin:$python_bin:/usr/bin:/bin"
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.invalid
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.invalid

repo="$work/repo"
git init --quiet -b main "$repo"
cd "$repo"
mkdir -p scripts tests gentalent cmd/gentalent
cp "$source_root/scripts/deploy.sh" scripts/deploy.sh
printf '[daemons.gentalent]\nrun = "exec ./.live/scripts/run-gentalent-live.sh ./config.json"\n' >mise.toml
printf 'import unittest\n\n\nclass Ok(unittest.TestCase):\n    def test_ok(self):\n        pass\n' >tests/test_ok.py
printf 'import sys\n\nsys.exit(0)\n' >gentalent/__main__.py
: >gentalent/__init__.py
printf 'module stub\n\ngo 1.27\n' >go.mod
printf '.live/\n.state/\nconfig.json\n/bin/\n__pycache__/\n*_local.go\n' >.gitignore
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
	if slices.Contains(os.Args, "version") {
		fmt.Print("$1")
	}
}
EOF
}
bot python
printf '#!/usr/bin/env bash\nexec "$python" -u -m gentalent --config "$1" run --live\n' >scripts/run-gentalent-live.sh
git add -A
git commit --quiet -m "Python bot"
python_commit="$(git rev-parse HEAD)"
cp "$source_root/scripts/run-gentalent-live.sh" scripts/run-gentalent-live.sh
bot first
git commit --quiet -am "First Go bot"
first_go="$(git rev-parse HEAD)"
bot second
git commit --quiet -am "Second Go bot"
second_go="$(git rev-parse HEAD)"

printf '{"database": ".state/gentalent.sqlite3"}\n' >config.json
mkdir -p .state
python3 -c 'import sqlite3, sys; sqlite3.connect(sys.argv[1]).close()' .state/gentalent.sqlite3
git worktree add --quiet --detach .live "$python_commit"

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
live_at() {
  git -C .live checkout --quiet --detach "$1"
}

printf 'untested\n' >.live/stray.txt
run_deploy --check "$first_go"
expect "an untracked file in .live is refused" 1 "$python_commit" ".live has local changes"
rm .live/stray.txt

printf '# edited in place\n' >>.live/mise.toml
run_deploy "$first_go"
expect "an edited tracked file in .live is refused" 1 "$python_commit" " M mise.toml"
grep -q 'edited in place' .live/mise.toml || fail "the refusal discarded the local edit"
git -C .live checkout --quiet -- mise.toml

ready="$work/locked"
python3 -c 'import sqlite3, sys, time
db = sqlite3.connect(sys.argv[1])
db.execute("BEGIN IMMEDIATE")
open(sys.argv[2], "w").close()
time.sleep(60)' .state/gentalent.sqlite3 "$ready" &
holder=$!
for _ in $(seq 1 100); do [[ -e "$ready" ]] && break; /usr/bin/sleep 0.1; done
run_deploy "$first_go"
expect "the first Go deploy waits while the running Python bot holds its write lock" 1 "$python_commit" "The live Python bot is mid-operation"
kill "$holder"
wait "$holder" 2>/dev/null || true
holder=""

live_at "$first_go"
"$go_bin/go" -C .live build -o bin/gentalent ./cmd/gentalent
touch .state/busy
run_deploy "$second_go"
expect "a deploy waits while the running Go bot has a photo search" 1 "$first_go" "a photo search is running"
rm .state/busy

printf 'package main\n\nimport "fmt"\n\nfunc init() { fmt.Print("LOCAL ") }\n' >.live/cmd/gentalent/extra_local.go
run_deploy "$second_go"
expect "a deploy pins the target and restarts" 0 "$second_go" "Live bot running"
expect_binary "the installed binary is the one tested, not one rebuilt in .live" second
rm .live/cmd/gentalent/extra_local.go

live_at "$first_go"
"$go_bin/go" -C .live build -o bin/gentalent ./cmd/gentalent
touch .state/crash
run_deploy "$second_go"
expect "a failed deploy rolls back to the previous Go commit" 1 "$first_go" "Rolling .live back to ${first_go:0:7}."
expect_binary "the rollback reinstalls the previous commit's binary" first

: >.state/messaging.sqlite3
live_at "$python_commit"
run_deploy "$first_go"
expect "a failed Go deploy is not rolled back to the Python bot after the Go bot ran" \
  1 "$first_go" "Not rolling back to ${python_commit:0:7}: it starts the Python bot"

live_at "$python_commit"
run_deploy "$python_commit"
expect "a deploy of the Python bot is refused after the Go bot ran" \
  1 "$python_commit" "${python_commit:0:7} starts the Python bot, but the Go bot has run"

rm .state/messaging.sqlite3
run_deploy "$first_go"
expect "a failed first Go deploy rolls back to the Python bot when the Go bot never ran" \
  1 "$python_commit" "Rolling .live back to ${python_commit:0:7}."
rm .state/crash

exit "$failures"
