# GenTalent: agent guide

A live bot in a real three-person group chat. Read `README.md` for what it does,
`doc/beeper-api.md` for what Beeper documents versus what was only observed, and
`doc/plans/next-features.md` for planned work and open questions.

## Where things run

| Place | Role |
|---|---|
| `~/ai/gentalent` (main checkout, branch `main`) | Owns `config.json`, `.state/` (live ballot, backups, media, GIF bank), `.live/`, and the `mise` daemons. Ship and deploy only from here. |
| `~/ai/gentalent/.live` (detached worktree) | The commit the live bot runs. Only `scripts/deploy.sh` moves it. |
| T3 Code worktrees | One per thread. T3 creates the worktree and branch. |

Editing files anywhere never changes the live bot; only a deploy does.

## Workflow

1. Work and commit in the thread's worktree, branched from `main`. `mise run test` runs the
   Go suite and the deploy and ship script checks and needs no config or network.
2. Once the branch has commits, push it, open a pull request against `main`, and link the PR
   to the thread (T3's `link_pull_request`). The PR is how the owner follows each thread;
   push again after every later commit so the PR shows what will ship.
3. From the main checkout: `mise run ship <branch>`. In order, it catches `main` up with
   GitHub's `main`, pushes the branch, opens its PR if it has none, then deploys the branch's
   commit: tests it in a scratch checkout, runs `doctor --contract` against the real Beeper
   API, refuses while the bot is mid-operation, backs up the ballot, pins `.live`, restarts
   only the bot, and rolls back if it does not stay up. Only after the bot stays up does it
   fast-forward `main` and push it, which marks the PR merged. A failed deploy leaves `main`
   and the PR as they were. Shipping changes what friends see in the chat: do it only when
   the owner asks.
4. `mise run deploy -- --check <commit>` runs every check without changing anything.

Branches ship by fast-forward, so rebase onto `main` before shipping if `main` moved. Merge
PRs only by shipping, never with GitHub's merge button: a merge there is not deployed, and
its new commit leaves every open branch behind `main`.

## Never

- Copy `config.json` into a worktree, or run `go run ./cmd/gentalent run --live` outside the
  main checkout. A second live bot would answer every command twice; a machine-wide lock
  per group refuses it, but do not rely on that.
- Write to `.state/nominations.sqlite3` or `.state/messaging.sqlite3` by hand. Rehearse
  against a copy made with `gentalent backup`, and use `gentalent status` / `resolve-send`
  for the live ones.
- Run `mise daemons restart`: it also restarts the Beeper app. Deploys stop and start only the bot.
- Send messages into the group to test something without the owner's go-ahead.
- Commit personal data: friends' names or phone IDs, account IDs, chat IDs, the owner's
  username, home path or email. Tests use fictional names; real captured shapes get
  placeholder IDs. The lefthook pre-commit and pre-push hooks enforce this with gitleaks
  (`scripts/privacy-check`); `mise run privacy` scans all history. Mark a line
  `gitleaks:allow` only when the owner explicitly asks to keep that data.

## Code

- `doc/architecture.md` is the map. The live bot is `cmd/gentalent`; the deploy installs the
  binary its tests ran against as `.live/bin/gentalent`. `cmd/gentalent` is the only place
  that wires the hexagons, starts goroutines or prints.
- One file per chat command in `internal/nominations/delivery/chat` (`shame.go`,
  `feedback.go`, `reactions.go`): it owns the command's parsing and reply text, and its
  handler in `app/command` or `app/query` owns the change. `bot.go` routes; `references.go`
  and `nominee_name.go` resolve what people type; `layout.go` lays out replies (read
  `doc/beeper-api.md` item 7 before writing reply text).
- A table change is the next numbered migration in that hexagon's `infra/sqlite`, run by
  `sqlitedb.Migrate` and tracked by `PRAGMA user_version`. Never renumber or drop a migration
  the live databases have applied, or change the schema it creates.
- Network calls stay out of chat logic; messaging's poll fetches what a command needs
  first (the replied-to message), and a failed fetch must never stall the poll.
- Tests prove behavior that would hurt if it broke. Break the rule a test guards and
  confirm it fails. No tests of formatting trivia or of mocks.
- The toolchain: inside scratch checkouts the `mise` shim may refuse an untrusted
  `mise.toml`; scripts resolve `go` and `jq` once at a trusted root and call them by path.
