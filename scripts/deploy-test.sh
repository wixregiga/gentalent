#!/usr/bin/env bash
# Runs the real scripts/deploy.sh against a throwaway main checkout in a temp folder, with
# mise and sleep stubbed so nothing is started, and checks what it refuses and what it pins.
set -euo pipefail

source_root="$(cd "$(dirname "$(realpath "$0")")/.." && pwd)"
go_bin="$(dirname "$(go env GOROOT)/bin/go")"
python_bin="$(dirname "$(python3 -c 'import sys; print(sys.executable)')")"
work="$(mktemp -d)"
trap 'git -C "$work/repo" worktree remove --force "$work/repo/.live" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT

mkdir -p "$work/stubs"
printf '#!/bin/sh\nexit 0\n' >"$work/stubs/mise"
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
printf 'package main\n\nfunc main() {}\n' >cmd/gentalent/main.go
printf '#!/usr/bin/env bash\nexec "$python" -u -m gentalent --config "$1" run --live\n' >scripts/run-gentalent-live.sh
printf '.live/\n.state/\nconfig.json\n/bin/\n__pycache__/\n' >.gitignore
git add -A
git commit --quiet -m "Python bot"
first="$(git rev-parse HEAD)"
printf '# changed\n' >>mise.toml
git commit --quiet -am "Next commit"
printf '{"database": ".state/gentalent.sqlite3"}\n' >config.json
mkdir -p .state
python3 -c 'import sqlite3, sys; sqlite3.connect(sys.argv[1]).close()' .state/gentalent.sqlite3
git worktree add --quiet --detach .live "$first"

failures=0
code=0
output=""
run_deploy() {
  code=0
  output="$(scripts/deploy.sh "$@" 2>&1)" || code=$?
}
expect() {
  local name="$1" want_code="$2" want_live="$3" want_text="$4"
  local live
  live="$(git -C .live rev-parse HEAD)"
  if [[ "$code" != "$want_code" || "$live" != "$want_live" || "$output" != *"$want_text"* ]]; then
    echo "FAIL $name: exit $code (want $want_code), .live at ${live:0:7} (want ${want_live:0:7}); output:" >&2
    echo "$output" >&2
    failures=$((failures + 1))
  else
    echo "ok   $name"
  fi
}

printf 'untested\n' >.live/stray.txt
run_deploy --check
expect "an untracked file in .live is refused" 1 "$first" ".live has local changes"
rm .live/stray.txt

printf '# edited in place\n' >>.live/mise.toml
run_deploy
expect "an edited tracked file in .live is refused" 1 "$first" " M mise.toml"
if ! grep -q 'edited in place' .live/mise.toml; then
  echo "FAIL the refusal discarded the local edit" >&2
  failures=$((failures + 1))
fi
git -C .live checkout --quiet -- mise.toml

mkdir -p .live/bin .live/gentalent/__pycache__
printf 'built\n' >.live/bin/gentalent
printf 'cache\n' >.live/gentalent/__pycache__/x.pyc
run_deploy --check
expect "git-ignored files in .live do not block a deploy" 0 "$first" "Check only: would pin .live from ${first:0:7}"

exit "$failures"
