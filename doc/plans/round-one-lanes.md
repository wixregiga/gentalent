# Round one: lane contract

The orchestrator's contract for the Go rewrite. Lanes read this, `doc/architecture.md`,
`AGENTS.md`, `rules.arclint.yaml`, the root `domain.arclint.yaml`, and
`.arclint/specs/*/round-one/` (`questions.md`, `behavior-inventory.md`, `round-one.ears.md`,
`round-one.feature`). Decisions the orchestrator made are recorded in `questions.md` under
"Orchestrator decisions".

## Standing rules (owner, verbatim)

"I just want easy to understand, manageable code. You are responsible for the codebases
maintainability long term and the complexity and understandability of the code between
agents of different models, effort level and providers is what you will be graded on. The
amount of code it takes for a model to get something done. The code paths that are possible
for features, how modular it is, how the rules for arclint are used to protect code."

"YOU ARE DIRECTLY JUDGED ON HOW EASY TO MAINTAIN THE CODEBASE IS, HOW EASY IT IS TO MODIFY
WITHOUT BREAKING OTHER THINGS, HOW COHERENT IT IS, HOW STRUCTURED IT IS"

"I'm telling you to make it fast and concurrent not break functionality (... just need to
make sure you're not breaking some undefined feature or some feature you don't know about
right now)"

"Every request is production-intent. There are no toy features, no demo-grade deliverables,
no simplified stand-ins. Treat placeholder logic, hardcoded happy paths, fixture-only
validation, stubbed returns, and example-grade shortcuts as defects, not drafts. Scope may
be narrow; depth may not be fake."

No backwards-compatibility code or reasoning. No em dashes anywhere. No comments in Go.

## Never

Do not ship, deploy, restart daemons, run the live bot, touch the main checkout
(`~/ai/gentalent`), its `config.json` or `.state/`, contact the Beeper API with a write, or
send anything into the group chat. Do not commit personal data (friends' names or phone IDs,
account or chat IDs, the owner's username, home path or email); tests use fictional names
and placeholder IDs. The Python package keeps working until the Go suite covers it.

## Process for every lane

1. `arclint context <paths>` before editing; state the change, who owns the decision, and
   what must stay intact.
2. A domain meaning that is new or changed is recorded with `arclint domain define` (see
   `.claude/skills/domain-librarian/SKILL.md`) before code. Only the nominations lane writes
   `domain.arclint.yaml`.
3. Tests first for each behavior; a test proves something that would hurt if broken. Break
   the guarded rule once and see the test fail.
4. Finish with `gofmt -l .` empty, `go vet ./...`, `go test ./...`,
   `flock /tmp/gentalent-arclint.lock arclint check . --format json --no-baseline` (exit 3
   or any `not_evaluated` means rerun) showing no violation in your paths, and
   `arclint rules test`. Never weaken a rule, a baseline or an exclusion to clear a
   finding; report the finding and propose the zone change instead.
5. Commit on your lane branch with a message ending in a `Co-Authored-By:` line naming
   your model. Hand back: files changed, contract deviations, tests and checks run with
   their output, and open items.

## Boundary contract between hexagons

The hexagons never import each other. `cmd/gentalent` translates between these types.
All IDs are platform strings; a Member is identified by the sender ID configured for them.

### nominations, called by cmd (package `internal/nominations/delivery/chat`)

```go
type Message struct {
	ID       string
	Sender   string
	Text     string
	SentAt   time.Time
	ReplyTo  *Quoted
	Mentions []string
}
type Quoted struct {
	ID      string
	Author  string
	Text    string
	SentAt  time.Time
	FromBot bool
	Missing bool
}
type Reply struct {
	Key        string
	Text       string
	Attachment string
	Announces  int
}
type PhotoSearch struct {
	Nomination int
	Person     string
	Pitch      string
	Dir        string
}
type PhotoResult struct {
	Nomination int
	Photos     []string
	Strip      string
	Failure    string
}
```

`chat.Bot` (built in cmd from the nominations handlers and adapters) offers:

| Method | Does |
|---|---|
| `Handle(ctx, Message) error` | Applies one Member message: command or query; a message ID already handled does nothing. Its replies commit with it. |
| `FinalizeDue(ctx, now) error` | Finalizes every locked Nomination whose minute is up; one batched verdict, then strips, in number order. |
| `CountReactions(ctx, nomination int, byMember map[string]string) error` | The reaction keys each Member has on that Nomination's announcement now; changes votes as the reactions say. |
| `OpenAnnouncements(ctx) ([]int, error)` | Nominations whose announcement reactions count right now. |
| `ClaimPhotoSearches(ctx) ([]PhotoSearch, error)` | Requested searches not yet started, marked started in the same transaction (at most once per Nomination). |
| `AttachPhotos(ctx, PhotoResult) error` | A finished or failed search; its replies commit with it. |
| `AbandonInterruptedSearches(ctx) error` | At startup: a search marked started with no result is recorded failed and the Group told, never rerun. |
| `Unrelayed(ctx) ([]Reply, error)` / `MarkRelayed(ctx, keys []string) error` | The nominations outbox. |

`Reply.Key` is unique and stable (for example `<message ID>/1`, `verdict/<n>/1`,
`photos/<n>/2`). `Announces` is the NominationNumber whose announcement this reply is, so
its reactions are votes; 0 otherwise. Which reply is the announcement depends on
`photo_layout` and is the nominations lane's decision.

### messaging, called by cmd (package `internal/messaging/app/...`)

```go
type Incoming struct {
	ID       string
	Sender   string
	Text     string
	SentAt   time.Time
	Linked   *Linked
	Mentions []string
}
type Linked struct {
	ID      string
	Sender  string
	Text    string
	SentAt  time.Time
	FromBot bool
	Missing bool
}
type Outgoing struct {
	Key        string
	Text       string
	Attachment string
	Tag        string
}
```

| Operation | Does |
|---|---|
| Poll | Reads new platform messages after the cursor into the inbox with the cursor, in one transaction. Keeps only messages from the configured chat and roster, sent after arming, not edited, deleted or hidden, not the bot's own. Loads each linked message; a failed load sets `Missing`, never stalls the poll. |
| Next | Inbox messages not yet handled, oldest first. |
| Handled(id) | Marks one handled. |
| Enqueue([]Outgoing) | Idempotent by `Key`; a known key is ignored. |
| Flush | Sends queued replies exactly once and tracks each (sent, uncertain, failed) as `doc/beeper-api.md` says. |
| Reactions(tag) | Reaction keys by Member sender ID on the sent message tagged `tag`. |
| Arm, ResolveSend, UncertainReplies, doctor checks, discovery | As the Python CLI does today. |

cmd tags an announcement reply `announces/<n>`.

### ai, called by cmd (package `internal/ai/app/command`)

`Find(ctx, Subject{Name, Pitch, Dir}) (Result{Photos []string, Strip string}, error)`:
tries the configured providers in order, each sandboxed and stripped of API-key variables
as the Python does, validates the manifest, saves three photos and the stitched strip in
`Dir`.

### Databases and migrations

`internal/shared/sqlitedb` opens databases, migrates them by `PRAGMA user_version` and takes
online backups; only cmd imports it (no hexagon may import `internal/shared`). Each
hexagon's `infra/sqlite` imports `database/sql` only: its store takes a `*sql.DB` (cmd opens a
read-write one and, for the read side, a read-only one), and it exports
`Migrations(pythonDatabase string) []func(context.Context, *sql.Tx) error`, which cmd runs
through `sqlitedb.Migrate`. `nominations.sqlite3` and `messaging.sqlite3` each have their own
numbered migrations. Migration 1 of each creates its schema and,
when the Python bot's database exists (config `database`), imports that hexagon's share of
its history from it, opened read-only, inside the same transaction. A rehearsal test runs the
import against a database built by the Python bot's own `SCHEMA` with every table filled.

### Config (`config.json`, read by cmd only)

Existing keys keep their meaning. New keys and defaults:
`group_open_nomination_limit` 1, `member_open_nomination_limit` 1,
`single_message_nomination_limit` 3, `allow_renomination` false, `photo_search_workers` 3,
`nominations_database` `.state/nominations.sqlite3`, `messaging_database`
`.state/messaging.sqlite3`. A limit below one refuses to start and names the setting.
