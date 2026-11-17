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
| `!final` | Nominator only, once all three votes are in. Otherwise the result posts after one minute. |
| `!gthelp` or `!!gthelp` | Rules and commands. Never runs AI. |
| `!gtstatus` | Current nominee, who voted what, and the record. |
| `!gtlist` / `!gtlist losers` | Certified winners (default) or denied nominees. |

Messages from outside the configured group and roster, and edited, deleted or hidden
messages, are ignored, as are the bot's own replies.

## Photos

`photo_layout` in `config.json` picks how `!!gt` photos appear:

- `individual` (default): the nomination text, then each photo as its own message. When the
  vote finishes, the verdict (certified or denied) carries all three stitched into one image.
- `stitched`: one stitched image attached to the nomination, nothing on the verdict.

The AI search is the same either way; it always saves three photos and the stitched strip.
Changing the setting needs a restart (`mise daemons restart gentalent`), not a deploy.
Restart when no `!!gt` photo search is running, or that search starts over.

## How it runs

Two `mise` daemons start at boot (`mise.toml`): `beeper` (the Linux Beeper Desktop app) and
`gentalent` (the bot).

The bot runs the commit pinned in `.live/`, a git worktree. **Editing files does not change
the live bot.** Only a deploy does:

    mise run deploy                  # deploy HEAD
    mise run deploy <commit>         # deploy, or roll back to, an earlier commit
    mise run deploy -- --check       # run every check, change nothing

A deploy runs the tests in a scratch checkout and `doctor --contract` against the real
Beeper API. It refuses while the bot is mid-operation (a `!!gt` photo search holds the
database for minutes; restarting then would repeat the AI call). It then pins `.live` and
restarts. If the bot doesn't stay up, it puts `.live` back on the previous commit.

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
