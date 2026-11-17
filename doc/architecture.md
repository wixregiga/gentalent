# GenTalent architecture (Go)

Three hexagons that never import each other, wired only in `cmd/gentalent`.
`rules.arclint.yaml` enforces every line of this page that says "only" or "never";
run `arclint context <path>` before editing a file.

```text
cmd/gentalent/                 composition: CLI, config, the engine, translation between hexagons
internal/
  nominations/                 the nomination process (domain context nominations)
    domain/                    Group aggregate and its entities and values; standard library only
    app/command/               one file per command and its handler
    app/query/                 one file per query and its handler
    port/                      every interface the use cases need, and the plain values they pass
    infra/sqlite/              the nominations database: store, read models (read_*.go), migrations
    infra/files/               the shame GIF bank on disk
    delivery/chat/             the chat commands: text in, command or query out, outcome to reply text
  messaging/                   the Group's conversation (domain context messaging)
    domain/                    messages, replies, delivery states
    app/command/, app/query/   poll, enqueue, flush, resolve-send; status of replies
    port/                      the chat platform and the messaging database
    infra/beeper/              Beeper Desktop API adapter, the only importer of the Beeper SDK
    infra/sqlite/              the messaging database: inbox, cursor, outbox, migrations
  ai/                          AI agents that do work for the Group (domain context ai)
    domain/                    a photo search, its manifest and its result
    app/command/               find a Person's photos
    port/                      the agent programs and the image tool
    infra/agents/              codex, claude, agy; the image stitcher
  shared/                      no product meaning
    sqlitedb/                  open (WAL, busy timeout), migrate by PRAGMA user_version, backup
```

## How a message flows

1. **Messaging polls.** `messaging` reads new messages from the platform, drops what the
   Group should never see (other chats, non-members, edits, deletions, hidden messages,
   anything before arming, the bot's own replies), loads the message each one replies to,
   and stores them in its inbox with the cursor, in one transaction.
2. **The engine applies them in order.** `cmd/gentalent` takes inbox messages oldest
   first, translates each into a `chat.Message`, and calls `nominations` delivery. Commands
   change the Group; queries read it. Both run in this one ordered lane, so a status after a
   vote shows the vote.
3. **Nominations commits the change and its replies together.** A command's Group change,
   the message ID it handled, and the replies it produced commit in one transaction of the
   nominations database (its outbox). A message ID already handled is skipped.
4. **The relay copies replies to messaging.** The engine reads nominations' unrelayed
   replies, enqueues them in messaging by their key (messaging ignores a key it already
   has), marks them relayed in nominations, then marks the inbox message handled. A crash
   at any step repeats only idempotent steps.
5. **Messaging sends each reply exactly once** and tracks it (sent, uncertain, failed),
   as `doc/beeper-api.md` describes.
6. **Photo searches run beside the lane.** A `!!gt` Nomination asks for photos. After the
   command commits, the engine claims each requested search (at most once per Nomination),
   runs up to `photo_search_workers` of them at once through `ai`, and hands each result
   back to the ordered lane as a command, whose replies go out when ready.
7. **Timers and reactions are commands too.** Each poll, the engine asks nominations to
   finalize Nominations whose minute is up, and gives it the reactions messaging read on
   each open announcement.

Two databases, one per stateful hexagon: `nominations.sqlite3` and `messaging.sqlite3`,
each with its own forward-only migrations tracked by `PRAGMA user_version`. The first
migration of each imports its share of the Python bot's `gentalent.sqlite3` when that file
exists, reading it only; the Python file itself is never changed.

## Where a change goes

| Change | Files |
|---|---|
| A new rule about nominating, voting, shaming | `arclint domain define`, then `nominations/domain`, then its test |
| A new chat command | `nominations/delivery/chat/<command>.go`, its handler in `app/command` or `app/query` |
| Reply wording | `nominations/delivery/chat` only |
| A new chat platform | a new adapter in `messaging/infra/`, chosen in `cmd/gentalent` |
| A new AI program | `ai/infra/agents/<program>.go` |
| A table change | a new numbered migration in that hexagon's `infra/sqlite` |

## Conventions

- No comments in Go (`source/no-comments`): names, types and tests carry the meaning.
- Every interface lives in a `port` package (`ports/interfaces-in-port-zone`).
- Errors are returned, never panicked; only `cmd` exits or prints to stdout.
- Tests use real adapters where they are cheap (SQLite in a temp dir) and fakes of ports
  in `_test.go` files. A test proves a behavior that would hurt if it broke.
