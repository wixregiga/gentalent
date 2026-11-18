# Next features: shame, feedback, info

Planned 2026-11-16. Feedback and shame are built; info waits for the ArcLint session.

## Built

### `!gtfeedback` / `!gtsuggest`

- Stores the text with sender, message ID and time (`feedback` table), then replies
  "Thanks @Name, your feedback (or suggestion) is logged and will be considered for a future release."
- Empty text gets a usage reply and stores nothing. Read entries with
  `go run ./cmd/gentalent feedback --live` (read-only, safe while the bot runs).

### `!gtshame`

Decisions from the owner:

1. Each member **gives** at most one shame per nominee (you can't shame one friend twice over
   the same nominee, nor one friend and then another). Revisit now that GIFs cost no tokens.
2. Shaming is allowed before or after the target votes, and after the nominee is finalized.
3. Nominees are named the way the bot shows them, squashed (`jordanabrewster`), in any
   case, with or without emoji; omitted means the latest nominee. A near-miss spelling gets
   "Did you mean ...?" and records nothing.
4. GIFs come from a bank (`.state/media/shame-gifs/`), filled once by an AI search. No chat
   command adds GIFs, for now.
5. The command is `!gtshame` (no AI, so no `!!`).

How it works:

- **Reply form (primary):** reply to someone's message with `!gtshame [nominee] [reason]`.
  The target is that message's author, and the exact text is quoted and recorded. Before
  handling, the polling loop fetches the replied-to message (`linkedMessageID`). If that
  fails for any reason the reply is "I couldn't load the message you replied to" and the
  sender can retry; retrying the poll instead would freeze the bot on a lasting error.
  A reply that also tags a different member is refused as ambiguous.
- **Tag form:** `!gtshame @Name [nominee] [reason]`. Names match the roster's full or first
  name; otherwise a single Beeper `mentions` entry is used.
- **Nominee words:** an exact or squashed name selects that nominee. A typo, or a first
  name or possessive alone ("Sandra", "Jordana's"), gets "Did you mean ...?" and records
  nothing, so a shame never lands on the wrong nominee. With no nominee words it is the
  latest nominee.
- **Replies to the bot's own messages are ignored, silently.** Bot photos and GIFs, which
  have no text, are recognized by the file names the bot sent. The bot posts from the
  owner's account, so the replied-to author would be wrong. Supporting them needs each
  outbox row to record the member it is about; add that where `shameOver`
  in `internal/nominations/delivery/chat/shame.go` resolves the target.
- Self-shame is refused before a GIF is chosen. Each shame records: nominee (case), shamer,
  target, reason, the target's vote at that moment, the shamed message ID, text and time,
  the GIF, and when. Triggers make shames permanent, like denials.
- GIF choice: least used so far, random among ties. An empty bank still records the shame
  and says no GIFs are available.

Not yet observed live: the bot sending a GIF (`attachment.type = "gif"`), and how phones
show a shame. The first real `!gtshame` is that test.

## Deferred until after ArcLint

### `!!gtinfo`

- Facts about the nominee (birth date rather than age, occupation, notable achievements,
  interesting facts, partner, ethnicity), separate from the nominator's pitch.
- Stored once per nominee with the date filled; later requests read the database and never
  call the AI. Generated automatically when a nomination has no pitch.
- Each field needs a source link or is dropped. A name that is not clearly a public figure
  gets "no reliable public info" instead of a guess.

Open questions:

1. Retry when the AI lookup fails, or record the failure?
2. A pitch-less `!gt` would then call AI. Acceptable?
3. Include ethnicity only when the person has stated it publicly, with a source?

## Settled in the ArcLint session

- **AI work no longer holds the database.** `!!gt` photo searches run beside the ordered
  lane (`doc/architecture.md`, step 6), at most once per Nomination; the deploy refuses to
  restart while one runs, because a restart abandons it. `!!gtinfo` can run the same way.
- **Schema changes** are numbered migrations in each hexagon's `infra/sqlite`, tracked by
  `PRAGMA user_version` (`doc/architecture.md`). Recording the member each outbox row is
  about is a new migration.
- **Database access** lives in each hexagon's `infra/sqlite`; chat commands reach it only
  through ports.
- **Beeper behavior** is recorded in `doc/beeper-api.md` (documented vs observed), and
  `go run ./cmd/gentalent doctor --contract` checks the observed parts against the live API.
