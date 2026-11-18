# GenTalent

A bot for one three-person Google Messages group. Members nominate a "generational talent"
with a keyword, everyone votes, and a unanimous yes certifies the nominee. Any no denies them
permanently. It reads and replies through the Beeper Desktop local API, and `!!gt` uses your
own AI subscriptions (`codex`, then `claude`, then `agy`) to attach three photos.

Replies come from the account paired with Beeper, prefixed `@gentalent-bot says:`.

## Chat commands

| Command | Effect |
|---|---|
| `!gt Name. Pitch` | Nominate. The nominator's vote counts as yes (1/3). |
| `!!gt Name. Pitch` | Nominate with three AI-found photos (see Photos below). Runs AI at most once per nominee. |
| `!!Y` / `!!N` / `!!C` (or `!!yes`, `!!no`, `!!clear`) | Vote, change your vote, or clear it, until the third vote is in. |
| 👍 / 👎 on the nomination announcement | Same as `!!Y` / `!!N`, any skin tone. Switching changes the vote; removing it clears the vote it cast. Silent unless `announce_reaction_votes` is `true` in `config.json`. |
| `!final` | Nominator only, once all three votes are in. Otherwise the result posts after one minute. |
| `!gthelp` or `!!gthelp` | Rules and commands. Never runs AI. |
| `!gtstatus` | Current nominee, who voted what, and the record. |
| `!gtlist` / `!gtlist losers` | Certified winners (default) or denied nominees. |
| `!gtshame [nominee] [reason]` as a reply | Shame the author of that message, quoting it, with a GIF. One shame given per nominee each. |
| `!gtshame @Name [nominee] [reason]` | The same, by tag. The nominee defaults to the latest one. |
| `!gtfeedback idea` / `!gtsuggest idea` | Log an idea for the bot. Read them with `python3 -m gentalent feedback --live`. |

Messages from outside the configured group and roster, and edited, deleted or hidden
messages, are ignored, as are the bot's own replies.

Nominees can be named the way the bot shows them, squashed (`jordanabrewster`), in any
case, with or without emoji. A near-miss spelling gets "Did you mean ...?" and does nothing.
Shames are permanent records of that moment (the target's vote, the quoted message), like
denials. Shame GIFs live in `.state/media/shame-gifs/`, used least-used first.

## Photos

`photo_layout` in `config.json` picks how `!!gt` photos appear:

- `individual` (default): the nomination text, then each photo as its own message. When the
  vote finishes, the verdict (certified or denied) carries all three stitched into one image.
- `stitched`: one stitched image attached to the nomination, nothing on the verdict.

The AI search is the same either way; it always saves three photos and the stitched strip.
Changing the setting needs a restart, not a deploy:
`mise daemons stop gentalent && mise daemons start gentalent`. (`mise daemons restart`
also restarts the Beeper app.) Restart when no `!!gt` photo search is running, or that
search starts over.

## How it runs

Two `mise` daemons start at boot (`mise.toml`): `beeper` (the Linux Beeper Desktop app) and
`gentalent` (the bot).

The bot runs the commit pinned in `.live/`, a git worktree. **Editing files does not change
the live bot.** Only a deploy does:

    mise run ship <branch>           # fast-forward main to a branch, then deploy it
    mise run deploy                  # deploy main
    mise run deploy <commit>         # roll back to an earlier commit on main
    mise run deploy -- --check <ref> # run every check on any commit, change nothing

All of these run from the main checkout only, and a real deploy takes only commits on `main`.

A deploy runs the tests in a scratch checkout and `doctor --contract` against the real
Beeper API. It refuses while the bot is mid-operation (a `!!gt` photo search holds the
database for minutes; restarting then would repeat the AI call). It then pins `.live` and
restarts. If the bot doesn't stay up, it puts `.live` back on the previous commit. It
refuses while `.live` has local changes (anything `git -C .live status` shows), because a
checkout keeps them and the bot would run code no test saw. `scripts/deploy-test.sh` runs
the real deploy script in a throwaway repository to prove its refusals.

It refuses commits from before `.live` existed, because their startup script runs the
working tree.

Day to day:

    mise run status    # daemons, deployed commit, Beeper check, replies needing attention
    python3 -m gentalent doctor --contract   # does Beeper still behave as doc/beeper-api.md says?
    mise run logs      # recent bot output
    mise run test

`gentalent status` opens the database read-only and is safe while the bot runs. Use it,
not a database viewer on Windows: recent changes sit in `.state/gentalent.sqlite3-wal`,
which a viewer opened over `\\wsl.localhost` may not see.

## Working on it

The Python bot in `gentalent/` is the one that runs. Its Go replacement is built beside it
(`doc/architecture.md`): `go run ./cmd/gentalent <subcommand>` takes the same subcommands
and flags, plus `busy`, which exits non-zero while a photo search is running. The Go bot
keeps its own databases, `nominations_database` and `messaging_database` in `config.json`
(default `.state/nominations.sqlite3` and `.state/messaging.sqlite3`), and imports the
Python history from `database` the first time it opens them, reading it only. It also
reads `group_open_nomination_limit` and `member_open_nomination_limit` (default 1),
`single_message_nomination_limit` (3), `allow_renomination` (false) and
`photo_search_workers` (3, the photo searches it runs at once); a limit below one refuses
to start. `mise run test` and the deploy check run both suites.

Changes happen in T3 Code worktree threads and ship from the main checkout with
`mise run ship <branch>`. Commits and pushes are scanned for personal data and secrets
(`lefthook install` once per clone). See [AGENTS.md](AGENTS.md).

## When a reply is uncertain

The bot never sends the same reply twice. If it can't tell whether a reply went out
(a network error mid-send, or Beeper forgetting the pending ID), it marks the reply
`uncertain` and moves on. `mise run status` lists these. Check the group, then record what
happened:

    python3 -m gentalent resolve-send --live OUTBOX_ID --message-id BEEPER_MESSAGE_ID
    python3 -m gentalent resolve-send --live OUTBOX_ID --not-delivered

`--message-id` is checked against Beeper first: the message must be yours and carry that
reply's text (or, for a photo reply, its `strip.jpg`). `--not-delivered` never resends.
Members can simply repeat their command. Both commands are safe while the bot runs.

See [doc/beeper-api.md](doc/beeper-api.md) for what Beeper documents and what was only
observed. The send-tracking rules depend on it.

## Setup from scratch

1. Install Beeper Desktop for Linux, sign in, and connect Google Messages. Turn on the
   Desktop API (Settings, Integrations, Approved connections). Keep Remote Access off.
2. `python3 -m gentalent init`, then `python3 -m gentalent set-secret` to store the API
   token in `~/.config/gentalent/` (mode 600, hidden prompt).
3. Find the group: `accounts`, `list-groups`, `find-chat "title words"`,
   `inspect-chat CHAT_ID`. Fill `config.json` with the exact chat ID, account ID, your
   sender ID as `owner_id`, and the three members.
4. `python3 -m gentalent doctor --chat` must report an exact group and roster match.
5. Try it without sending anything: `python3 -m gentalent arm`, then
   `python3 -m gentalent run`. Replies print as `PREVIEW:` and use a separate dry-run
   database.
6. `mise run deploy`. The live database arms itself on first start and ignores messages
   sent before that.

## Data and privacy

- `config.json` (real IDs) and `.state/` (ballots, media, backups) are git-ignored.
  `config.example.json` is the template.
- Denials are permanent at the database level (triggers block deletes and updates). Keep
  `.state/gentalent.sqlite3`. A fresh database loses the record.
- The Beeper token can read and send as your account in every chat. GenTalent limits
  itself to one group and roster, but the token itself has no such limit.
- `!!gt` strips API-key variables before running the AI CLIs, so they use subscription
  logins only. They start in the nominee's media folder. Codex is sandboxed to it; `claude`
  and `agy` run with their permission checks turned off, so they are not confined.
