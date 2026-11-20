#!/usr/bin/env bash
# Release a branch: bring main up to date with GitHub, push the branch, make sure it has a pull
# request, deploy the branch's commit, and only once the bot stays up on it fast-forward main and
# push main. The push lands the PR's commits on main, so GitHub marks the PR merged.
#   mise run ship <branch>
# Runs only from the main checkout, with main checked out. A failed deploy leaves main, GitHub's
# main and the open PR as they were.
set -euo pipefail
cd "$(dirname "$(realpath "$0")")/.."
root="$PWD"
common="$(dirname "$(realpath "$(git rev-parse --git-common-dir)")")"
if [[ "$root" != "$common" ]]; then
  echo "Ship from the main checkout ($common), not a worktree." >&2
  exit 1
fi
if [[ "$(git rev-parse --abbrev-ref HEAD)" != main ]]; then
  echo "Run ship from the main checkout, with main checked out." >&2
  exit 1
fi
branch="${1:?usage: mise run ship <branch>}"
if ! git show-ref --verify --quiet "refs/heads/$branch"; then
  echo "There is no local branch $branch." >&2
  exit 1
fi
if ! gh="$(command -v gh)"; then
  echo "gh is missing, so ship cannot open or close the branch's pull request." >&2
  exit 1
fi
# Tracked edits in the main checkout would ride along with the fast-forward into main.
if [[ -n "$(git status --porcelain --untracked-files=no)" ]]; then
  echo "The main checkout has uncommitted changes; ship moves main, so commit or remove them first." >&2
  git status --short --untracked-files=no >&2
  exit 1
fi

git fetch --quiet origin main
if ! git merge-base --is-ancestor origin/main main; then
  if git merge-base --is-ancestor main origin/main; then
    git merge --quiet --ff-only origin/main
    echo "Brought main up to GitHub's main ($(git rev-parse --short main))."
  else
    echo "main and GitHub's main have diverged; reconcile them before shipping." >&2
    exit 1
  fi
fi

target="$(git rev-parse --verify "$branch^{commit}")"
if git merge-base --is-ancestor "$target" main; then
  echo "$branch has nothing that is not already on main." >&2
  exit 1
fi
if ! git merge-base --is-ancestor main "$target"; then
  echo "$branch does not build on the current main. Rebase it onto main, then ship again." >&2
  exit 1
fi

git push --quiet origin "$branch"
pr="$("$gh" pr list --head "$branch" --base main --state open --json url --jq '.[0].url // empty')"
if [[ -z "$pr" ]]; then
  pr="$("$gh" pr create --base main --head "$branch" --fill)"
  echo "Opened $pr"
else
  echo "Pull request: $pr"
fi

scripts/deploy.sh --ship "$target"

git merge --quiet --ff-only "$target"
if ! git push --quiet origin main; then
  echo "The bot runs ${target:0:7} and main is there, but pushing main to GitHub failed." >&2
  echo "Push it with: git push origin main. That also closes $pr." >&2
  exit 1
fi
state=""
for _ in 1 2 3 4 5 6; do
  state="$("$gh" pr view "$pr" --json state --jq .state)"
  [[ "$state" == MERGED ]] && break
  sleep 5
done
if [[ "$state" == MERGED ]]; then
  echo "Shipped ${target:0:7}: live, on main and on GitHub; $pr is merged."
else
  echo "Shipped ${target:0:7}: live, on main and on GitHub, but $pr still shows $state." >&2
  exit 1
fi
