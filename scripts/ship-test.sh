#!/usr/bin/env bash
# Runs the real scripts/ship.sh in a throwaway main checkout whose origin is a local bare repo,
# and checks the release order: main catches up with origin, the branch is pushed and gets one
# pull request, the deploy runs on the branch commit while main has not moved, and main moves
# and reaches origin only after the deploy succeeds. scripts/deploy.sh (tested by deploy-test.sh)
# is replaced by a stub that logs its arguments and main, and fails while .state/deploy-fails
# exists. gh is a stub that keeps one pull request in .state and, as GitHub does, reports it
# merged once its head commit is on origin's main.
set -euo pipefail

source_root="$(cd "$(dirname "$(realpath "$0")")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

mkdir -p "$work/stubs"
cat >"$work/stubs/gh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
case "$1 $2" in
  "pr list")
    [[ -e .state/pr-url ]] && ! git -C "$ORIGIN" merge-base --is-ancestor "$(cat .state/pr-head)" main && cat .state/pr-url
    exit 0 ;;
  "pr create")
    head=""
    while [[ $# -gt 0 ]]; do [[ "$1" == --head ]] && head="$2"; shift; done
    echo "create $head" >>.state/gh.log
    echo "https://github.invalid/pull/1" >.state/pr-url
    git -C "$ORIGIN" rev-parse "refs/heads/$head" >.state/pr-head
    cat .state/pr-url ;;
  "pr view")
    if git -C "$ORIGIN" merge-base --is-ancestor "$(cat .state/pr-head)" main; then echo MERGED; else echo OPEN; fi ;;
esac
STUB
printf '#!/bin/sh\nexit 0\n' >"$work/stubs/sleep"
chmod +x "$work/stubs/gh" "$work/stubs/sleep"
export PATH="$work/stubs:/usr/bin:/bin"
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.invalid
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.invalid
export ORIGIN="$work/origin.git"

git init --quiet --bare -b main "$ORIGIN"
repo="$work/repo"
git init --quiet -b main "$repo"
cd "$repo"
mkdir -p scripts
cp "$source_root/scripts/ship.sh" scripts/ship.sh
cat >scripts/deploy.sh <<'STUB'
#!/usr/bin/env bash
echo "$* main=$(git rev-parse main)" >>.state/deploys.log
[[ -e .state/deploy-fails ]] && { echo "Live bot did not stay up." >&2; exit 1; }
echo "Live bot running $2."
STUB
chmod +x scripts/deploy.sh
printf '.state/\n' >.gitignore
git add -A
git commit --quiet -m "Start"
git remote add origin "$ORIGIN"
git push --quiet origin main
mkdir -p .state

commit_on() {
  git checkout --quiet -B "$1" "$2"
  echo "$1 $RANDOM" >>"$1.txt"
  git add -A
  git commit --quiet -m "Work on $1"
  git checkout --quiet main
}

failures=0
code=0
output=""
run_ship() {
  code=0
  output="$(scripts/ship.sh "$@" 2>&1)" || code=$?
}
fail() {
  echo "FAIL $1" >&2
  echo "$output" >&2
  failures=$((failures + 1))
}
check() {
  local name="$1" want_code="$2" want_text="$3"
  shift 3
  if [[ "$code" != "$want_code" || "$output" != *"$want_text"* ]]; then
    fail "$name: exit $code (want $want_code)"
  elif ! "$@"; then
    fail "$name: state check failed"
  else
    echo "ok   $name"
  fi
}
same() { [[ "$(git rev-parse "$1")" == "$(git rev-parse "$2")" ]]; }
on_origin() { [[ "$(git -C "$ORIGIN" rev-parse "$1" 2>/dev/null)" == "$(git rev-parse "$2")" ]]; }
not_on_origin() { ! git -C "$ORIGIN" rev-parse --verify --quiet "refs/heads/$1" >/dev/null; }
creates() { [[ "$(grep -c "^create $1$" .state/gh.log 2>/dev/null || true)" == "$2" ]]; }
start="$(git rev-parse main)"

run_ship
check "ship needs a branch" 1 "usage: mise run ship <branch>" true

commit_on feature main
printf 'edited\n' >>.gitignore
run_ship feature
check "uncommitted changes in the main checkout are refused before anything moves" 1 "uncommitted changes" not_on_origin feature
git checkout --quiet -- .gitignore

git branch --quiet level main
run_ship level
check "a branch with nothing beyond main is refused" 1 "nothing that is not already on main" not_on_origin level

touch .state/deploy-fails
run_ship feature
deployed_on_old_main() { grep -q "^--ship $(git rev-parse feature) main=$start$" .state/deploys.log; }
untouched() { same main "$start" && on_origin main "$start" && on_origin feature feature && creates feature 1 && deployed_on_old_main; }
check "a failed deploy leaves main and origin's main alone, after pushing the branch and opening its PR" 1 "did not stay up" untouched
rm .state/deploy-fails

run_ship feature
released() { same main feature && on_origin main feature && creates feature 1; }
check "a deploy that stays up moves main, pushes it and closes the existing PR without opening another" 0 "is merged" released

commit_on stale "$start"
run_ship stale
check "a branch that does not build on main is refused before it is pushed" 1 "Rebase it onto main" not_on_origin stale

helper="$work/helper"
git clone --quiet "$ORIGIN" "$helper"
git -C "$helper" commit --quiet --allow-empty -m "Landed on GitHub first"
git -C "$helper" push --quiet origin main
git fetch --quiet origin main
commit_on follow origin/main
run_ship follow
caught_up() { same main follow && on_origin main follow && creates follow 1; }
check "main catches up with origin's main before a branch built on it ships" 0 "Brought main up to GitHub's main" caught_up

git -C "$helper" pull --quiet --ff-only
git -C "$helper" commit --quiet --allow-empty -m "Elsewhere"
git -C "$helper" push --quiet origin main
git commit --quiet --allow-empty -m "Here"
here="$(git rev-parse main)"
commit_on split main
run_ship split
diverged() { same main "$here" && not_on_origin split; }
check "main that has diverged from origin's main is refused" 1 "have diverged" diverged

exit "$failures"
