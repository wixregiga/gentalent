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

1. Work and commit in the thread's worktree, branched from `main`. `mise run test` needs no
   config or network.
2. From the main checkout: `mise run ship <branch>`. It fast-forwards `main`, tests the
   commit in a scratch checkout, runs `doctor --contract` against the real Beeper API,
   refuses while the bot is mid-operation, backs up the ballot, pins `.live`, restarts only
   the bot, and rolls back if it does not stay up. Shipping changes what friends see in the
   chat: do it only when the owner asks.
3. `mise run deploy -- --check <commit>` runs every check without changing anything.

Branches ship by fast-forward, so rebase onto `main` before shipping if `main` moved.

## Never

- Copy `config.json` into a worktree, or run `python3 -m gentalent run --live` outside the
  main checkout. A second live bot would answer every command twice; a machine-wide lock
  per group refuses it, but do not rely on that.
- Write to `.state/gentalent.sqlite3` by hand. Rehearse against a copy made with the
  SQLite backup API, and use `gentalent status` / `resolve-send` for the live one.
- Run `mise daemons restart`: it also restarts the Beeper app. Deploys stop and start only the bot.
- Send messages into the group to test something without the owner's go-ahead.
- Commit personal data: friends' names or phone IDs, account IDs, chat IDs, the owner's
  username, home path or email. Tests use fictional names; real captured shapes get
  placeholder IDs. The lefthook pre-commit and pre-push hooks enforce this with gitleaks
  (`scripts/privacy-check`); `mise run privacy` scans all history. Mark a line
  `gitleaks:allow` only when the owner explicitly asks to keep that data.

## Code

- One module per chat feature (`shame.py`, `feedback.py`, `reactions.py`): it owns its
  command, checks and reply text. `workflow.py` routes; `nominees.py` resolves what people
  type; `replies.py` lays out replies (read its header before writing reply text).
- New tables: `CREATE TABLE IF NOT EXISTS` in `state.SCHEMA`. Changing an existing table
  needs a live migration plan; see the ArcLint notes in `doc/plans/next-features.md`.
- Network calls stay out of chat logic; the polling loop in `runner.py` fetches what a
  command needs first, and a failed fetch must never stall the poll.
- Tests prove behavior that would hurt if it broke. Break the rule a test guards and
  confirm it fails. No tests of formatting trivia or of mocks.
- The interpreter: inside scratch checkouts the `mise` shim may refuse an untrusted
  `mise.toml`; scripts resolve `python3` once at a trusted root and call it by path.
