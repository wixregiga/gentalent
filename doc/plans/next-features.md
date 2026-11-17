# Next features: shame, feedback, info

Planned 2026-11-16. Feedback and shame are built; info waits for the ArcLint session.

## Built

### `!gtfeedback` / `!gtsuggest`

- Stores the text with sender, message ID and time (`feedback` table), then replies
  "Thanks @Name, your feedback (or suggestion) is logged and will be considered for a future release."
- Empty text gets a usage reply and stores nothing. Read entries with
  `python3 -m gentalent feedback --live` (read-only, safe while the bot runs).

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
  handling, the polling loop fetches the replied-to message (`linkedMessageID`); a deleted
  one gets "I couldn't load the message you replied to", other API errors retry the poll.
- **Tag form:** `!gtshame @Name [nominee] [reason]`. Names match the roster's full or first
  name; otherwise a single Beeper `mentions` entry is used.
- **Replies to the bot's own messages are ignored, silently.** The bot posts from the
  owner's account, so the replied-to author would be wrong. Supporting them needs each
  outbox row to record the member it is about; add that where `shame.run` resolves the target.
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

## For the ArcLint session

- **AI work holds the database.** `!!gt` photo searches run inside the message
  transaction, so the bot answers nothing else for minutes, and a restart mid-search
  repeats it (`deploy.sh` refuses to restart while one runs). `!!gtinfo` would add more of
  this. Move AI calls to a background job queue first.
- **Schema changes.** New tables use `CREATE TABLE IF NOT EXISTS` and need no migration.
  Changing an existing table (for example recording the member each outbox row is about)
  needs a decision on how live-database migrations are run without in-code
  backwards-compatibility paths.
- **Database access is spread out.** `workflow.py` still runs SQL directly. New features
  follow the module pattern instead: `shame.py` and `feedback.py` own their command,
  checks and replies; `nominees.py` resolves what people type; `replies.py` formats text.
- **Beeper behavior** is recorded in `doc/beeper-api.md` (documented vs observed), and
  `python3 -m gentalent doctor --contract` checks the observed parts against the live API.
