# round-one layout options: Go code layout and ArcLint rules

Status: options for the owner to choose from. No Go code exists yet and no existing file was changed.
Answers Q35 in `questions.md`. The owner asked for hexagonal shapes with better separation of context, CQRS first, concurrent photo searches, queued message handling, and nothing existing broken.

How to read the labels: a sentence with a source in brackets is checked against that source. `[ASSUMPTION]` and `[INFERENCE]` mark what is not checked. Business rules are named by their domain key (for example `Group / open-nominations-within-group-limit`) and are not restated here.

## 1. What bskilled taught

Sources:

- **Orchestrator:** the reply of [(O) Redesign Dashboard Navigation](t3-thread://v2/c24b92a2-3b7b-4c1e-905f-a355d7ad1dd0/70b7e848-0106-4d71-9e53-f1bfb40eec67), run 27, 2026-11-17. It said its own experience is in the web zones and one source rule, and it marked its points about the Go rules as inference.
- **Rules file:** `bskilled/rules.arclint.yaml`, by line number.
- **README:** `bskilled/.arclint/README.md`.
- **Scratch:** my own runs of `arclint 0.1.0-beta.1` under a scratch directory (section 5).

### Keep

| Lesson | Source | Used below as |
|---|---|---|
| A per-zone "imports only" list is the core. It forced a real design decision: two features stayed apart and only the app shell knows they are related. | Orchestrator (`web-features/independent`, `web-app/composes-features`); Rules file line 474 | The pattern's six import rules, plus `cqrs/*`, `relay/alone`, `chat/parses-and-renders-only`, `contexts/*-alone` |
| One short layer list states the whole direction. | Orchestrator, as inference; Rules file lines 316 to 322 | `hexagon/dependencies-inward`, `hexagon/no-cycles` |
| Forbid IO imports in the use-case layer, and treat it as a tripwire. ArcLint reports such a rule as "exact for its pattern; its rationale is not checked". | Orchestrator; Rules file lines 126 and 324; Scratch shows the same wording | `app/no-direct-io`, `domain/no-effect-imports` |
| One library behind one adapter (bskilled keeps Cobra inside the CLI adapter). | Rules file line 290 | `beeper/sdk-behind-the-layer`, `sqlite/sql-behind-the-store`, `photosearch/programs-behind-the-finder` |
| The model has a named file, no generic container files, and no serialization tags. | Rules file lines 371, 377, 387 | `domain/aggregate-has-its-file`, `go/concept-named-files`, `domain/no-transport-tags` |
| Library code returns errors: no panic, no process exit, no direct stdout. | Rules file line 334 and the two rules after it | `go/no-panic-in-library`, `go/no-process-exit-in-library`, `go/no-direct-stdout-in-library` |
| Run `arclint check . --format json --no-baseline`, one run at a time. Exit code 3 or a `not_evaluated` entry means the run decided nothing. Read `arclint context <path>` before editing. | Orchestrator | How section 5 was run; worth one line in `AGENTS.md` when Go lands |
| A passing structural check does not prove a rule's body is right. | README lines 3 and 40 | Each recorded assertion still needs its own test |

### Avoid

| Lesson | Source | Decision here |
|---|---|---|
| `source/no-comments` was added late and left 3,973 adopted findings. Every touched file needs a full strip, a strip script changed 5 files it should not have, and the check exits 1 on every run. The orchestrator's advice: decide on day one or not at all. | Orchestrator; README line 33; Rules file line 525 | Not in the rules below. It is an owner decision (section 6). It is also not built into ArcLint: it is the extension `bskilled/.arclint/extensions/no_comments.ts`. |
| `polymorphism/contract-only-consumers` lists about 50 contract types by hand. A new port that is not added to the list is silently not covered. | Orchestrator, drift risk as inference; Rules file lines 76 to 91 | Not in the rules below. It is also a bskilled extension (`contract_only_consumers.ts`), not built in. The pattern's import rules already stop a use case from importing an adapter. |
| The `legacy/*`, `no-legacy-*` and `no-retired-*` rules fence off bskilled's own migration history. | Orchestrator; Rules file lines 200 to 315 | Not ported. GenTalent's Go tree has no history. |
| A zone list that grows one zone per package (bskilled has 21 zones and four `chosen-by-composition` rules). | Rules file lines 161, 221, 226, 252, 285. `[INFERENCE]` that this is a cost; the orchestrator did not say so. | Each shape binds the six pattern zones and adds a zone only where a rule needs one. The pattern's allow-lists already leave composition as the only importer of adapters. |

### What the scratch runs added

- **The model may not import `internal/shared`.** ArcLint's built-in `domain_isolation/model-imports-nothing-outside-itself` reports it, although `arclint/hexagonal` allows it. Each shape states this as `domain/standard-library-only` so an agent reads it in `arclint context` before writing code.
- **A zone named like a recorded context becomes that context's code.** With the pending `beeper` context recorded, a zone named `beeper` made the Beeper adapter "context beeper", and its import of a GenTalent port became an error. The zone is therefore named `beeper_layer`. The folder name does not matter.
- **`independent` did not fire between two folders of one zone.** A query file importing the command package passed `independent: ["internal/gentalent/app/*"]` in three spellings. The shapes use one `imports` allow-list per side instead, which did fire.
- **Overlapping zones work.** A file can sit in a pattern zone and a narrower house zone, and an import is allowed when any of its zones is on the allow-list.

## 2. What is the same in every shape

Every shape is ports and adapters with these parts. Folder names differ; the parts and the rules do not.

- **Model.** The Group aggregate and its entities, with the methods ArcLint expects for each recorded assertion. Standard library only.
- **Command side.** One file per command: the command value, its outcome value, and its handler. `ports.go` holds the ports that change things (the Group store, the outbox, the photo finder, the clock).
- **Query side.** One file per query. `ports.go` holds one read port. The query side cannot import the command side, so it cannot reach a port that changes the Group.
- **Relay.** Takes new messages in and sends replies out exactly once, through one chat port. This is where today's `runner.py` delivery tracking goes.
- **Chat-command parser.** One file per chat command: it turns the words into a command or query and turns the outcome into reply text. `routes.go` maps each keyword to its file. It imports only the command side, the query side and the model.
- **Engine.** The only place goroutines start. It runs the poll loop, a queue of parsed messages, one ordered lane for commands, a concurrent lane for queries, a pool of photo-search workers (one search per Nomination, several at once), and the timers. Replies leave as each is ready.
- **Adapters.** Beeper (the only importer of `github.com/beeper/desktop-api-go`), SQLite (the only importer of `database/sql`; read models live in `read_*.go`), photo search (the only importer of `os/exec`).
- **Composition.** `cmd/gentalent/main.go` builds the adapters and hands them to the handlers.

`[ASSUMPTION]` Commands are applied in arrival order and only replies may leave out of order. The owner asked for CQRS, buffering and concurrent searches (Q27) but did not read the Q31 proposal that says this.
`[ASSUMPTION]` The file names (`ports.go`, `routes.go`, `read_*.go`) and the `Handlers` struct are mine. Only `read_*.go` is fixed by a rule.
`[ASSUMPTION]` `runtime: [go]` and the Python package stays unjudged until it is deleted at cutover. The `scan.exclude` list is the one approved in Q16.

## 3. The shapes

### Shape A: one hexagon, folders named for the layer

`internal/<x>/` is a layer of the hexagon. This is the `arclint/hexagonal` default layout.

```text
cmd/gentalent/main.go                 composition
internal/
  domain/                             model: group.go, nomination.go, member.go
  application/
    command/                          ports.go, nominate.go, cast_vote.go, search_photos.go, ...
    query/                            ports.go, status.go, list.go, help.go
    relay/                            ports.go, poll.go, flush.go
  adapters/
    driving/
      chat/                           parse.go, routes.go, one file per chat command
      engine/                         poller.go, lanes.go, photo_worker.go, timers.go
      cli/                            run, status, doctor, resolve-send, setup
    driven/
      beeper/                         the Beeper adapter
      sqlite/                         group_store.go, outbox.go, read_*.go, migrations
      photosearch/                    runs the AI tools
  shared/                             code with no product meaning
```

| Thing | Lives in |
|---|---|
| Commands | `internal/application/command/` |
| Queries | `internal/application/query/` |
| Photo-search worker | pool in `internal/adapters/driving/engine/photo_worker.go`; the use case it runs in `internal/application/command/search_photos.go`; the AI tools in `internal/adapters/driven/photosearch/` |
| Beeper adapter | `internal/adapters/driven/beeper/` |
| SQLite | `internal/adapters/driven/sqlite/` |
| Chat-command parser | `internal/adapters/driving/chat/` |

**Path of one new chat command** (files a model touches):

1. `internal/domain/`: the behavior on the Group, only when the command brings a rule, which is recorded in `domain.arclint.yaml` first.
2. `internal/application/command/<name>.go` (or `query/<name>.go`).
3. `internal/adapters/driving/chat/<name>.go`, plus one line in `routes.go`.
4. Only when it stores something new: a file and a migration in `internal/adapters/driven/sqlite/`. Only when it needs a new port: one line in `cmd/gentalent/main.go`.

Not touched: engine, relay, Beeper, photo search.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/wixregiga/arclint/main/docs/schemas/rules.arclint.schema.json
# GenTalent, shape A: one hexagon, folders named for the layer.
# internal/adapters/driven/beeper is the anticorruption layer toward Beeper;
# internal/shared has no product meaning.
# Ask before editing: arclint context <path> · arclint rules [selector]

runtime: [go]

scan:
  unknown_imports: warn
  exclude:
    - ".live/**"
    - ".state/**"
    - "welven_da_great_text/**"
    - ".venv/**"

domain:
  file: domain.arclint.yaml

extends:
  - pattern: arclint/hexagonal@0.1.0
    bind:
      domain: "internal/domain/**"
      application: "internal/application/**"
      driving: "internal/adapters/driving/**"
      driven: "internal/adapters/driven/**"
      shared: "internal/shared/**"
      composition: "cmd/**"

zones:
  command:
    paths: "internal/application/command/**"
    description: "Write side: one file per command, its handler, and the ports a handler needs to change the Group."
  query:
    paths: "internal/application/query/**"
    description: "Read side: one file per query, its handler, and the read port. Changes nothing."
  relay:
    paths: "internal/application/relay/**"
    description: "Taking messages in and sending replies out exactly once, through the chat port."
  chat:
    paths: "internal/adapters/driving/chat/**"
    description: "The chat-command parser and reply text: turns message text into one command or query and its outcome into a reply."
  engine:
    paths: "internal/adapters/driving/engine/**"
    description: "The only home of goroutines: the poll loop, the ordered command lane, the concurrent query lane, the photo-search workers, and timers."
  cli:
    paths: "internal/adapters/driving/cli/**"
    description: "Operator commands: run, status, doctor, resolve-send, setup."
  sqlite:
    paths: "internal/adapters/driven/sqlite/**"
    description: "The only code that speaks SQL: the Group store, the outbox, read models in read_*.go, and migrations."
  photosearch:
    paths: "internal/adapters/driven/photosearch/**"
    description: "The only code that starts other programs: runs the AI tools that find a nominee's photos and checks what they return."
  beeper_layer:
    paths: "internal/adapters/driven/beeper/**"
    description: "Anticorruption layer: the only code that imports the Beeper SDK; implements the relay chat port in GenTalent's words."

rules:
  hexagon/dependencies-inward:
    rationale: "Dependencies point from wiring through adapters and use cases to the model."
    layers: [composition, driving, driven, application, domain, shared]

  hexagon/no-cycles:
    rationale: "The six parts of the hexagon form no import cycle."
    acyclic: [composition, driving, driven, application, domain, shared]

  domain/standard-library-only:
    rationale: "The model imports the Go standard library and nothing else, internal/shared included; ArcLint's built-in domain_isolation rule refuses a model that imports shared code."
    on: domain
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests are not the model."
    imports:
      internal: []
      external: forbid

  domain/no-effect-imports:
    rationale: "The model decides and performs no effect. Declare the effect as a port in internal/application and implement it in an adapter."
    on: domain
    files: "internal/domain/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test fixtures may use IO."
    content:
      forbid: '"(os|os/exec|io/fs|net|net/http|database/sql|encoding/json|encoding/xml|encoding/gob)"'

  domain/no-transport-tags:
    rationale: "Model values carry no serialization tags; adapters map them to rows and payloads."
    on: domain
    files: "internal/domain/**/*.go"
    content:
      forbid: '`(json|yaml|toml|db):'

  domain/aggregate-has-its-file:
    rationale: "Every recorded aggregate has a file named after it, so a reader finds the root without searching."
    on: domain
    structure:
      each: domain.aggregates
      require: ["internal/domain/{name:snake_case}.go"]

  go/concept-named-files:
    rationale: "A file is named for the one concept, command, query or adapter it holds, never for a generic container."
    on: [domain, application, driving, driven, shared, composition]
    structure:
      forbid:
        - "**/model.go"
        - "**/models.go"
        - "**/types.go"
        - "**/utils.go"
        - "**/util.go"
        - "**/helpers.go"
        - "**/common.go"
        - "**/misc.go"

  go/snake-case-files:
    rationale: "Go file names use snake_case so files are easy to find and scan."
    on: [domain, application, driving, driven, shared, composition]
    naming: snake_case

  app/no-direct-io:
    rationale: "Use cases coordinate ports; adapters perform IO. A tripwire for the named imports, not a proof."
    on: application
    files: "internal/application/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test fixtures may use IO."
    content:
      forbid: '"(os|os/exec|io/fs|net|net/http|database/sql)"'

  cqrs/command-side-alone:
    rationale: "The command side imports the model and shared code only: never the query side or the relay."
    on: command
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [domain, shared]

  cqrs/query-side-alone:
    rationale: "The query side imports the model and shared code only, so a query cannot reach a port that changes the Group."
    on: query
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [domain, shared]

  relay/alone:
    rationale: "The relay moves messages and replies and knows no command or query; the engine is where the three meet."
    on: relay
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [domain, shared]

  cqrs/read-models-read-only:
    rationale: "Read models in read_*.go run no statement that changes data. A tripwire; the read side also opens the database read-only."
    on: sqlite
    files: "internal/adapters/driven/sqlite/read_*.go"
    content:
      forbid: '\.Exec(Context)?\(|\b(INSERT|UPDATE|DELETE|REPLACE|CREATE|ALTER|DROP)\b'

  driving/never-imports-wiring:
    rationale: "Delivery reaches use cases through what composition hands it and never imports cmd."
    on: driving
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [application, domain, shared]

  chat/parses-and-renders-only:
    rationale: "The chat-command parser builds commands and queries and words their outcomes. It never polls, sends or schedules."
    on: chat
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [command, query, domain, shared]

  concurrency/engine-owns-goroutines:
    rationale: "Only the engine starts goroutines. The model, use cases and the parser are plain blocking calls, so each is tested without timing."
    on: [domain, application, chat]
    files: "internal/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests may start goroutines to exercise blocking calls."
    content:
      forbid: '^\s*go\s+(func\b|[A-Za-z_][A-Za-z0-9_.]*\()'

  beeper/sdk-behind-the-layer:
    rationale: "Only internal/adapters/driven/beeper imports the Beeper SDK, so a change in Beeper is repaired in one folder."
    files: ["internal/**/*.go", "cmd/**/*.go"]
    exclude:
      paths: ["internal/adapters/driven/beeper/**"]
      reason: "The anticorruption layer owns the SDK."
    content:
      forbid: '"github\.com/beeper/desktop-api-go'

  sqlite/sql-behind-the-store:
    rationale: "Only the sqlite adapter imports database/sql; composition opens the database through it."
    files: ["internal/**/*.go", "cmd/**/*.go"]
    exclude:
      paths: ["internal/adapters/driven/sqlite/**"]
      reason: "The sqlite adapter owns SQL."
    content:
      forbid: '"database/sql(/driver)?"'

  photosearch/programs-behind-the-finder:
    rationale: "Only the photo-search adapter starts other programs."
    files: ["internal/**/*.go", "cmd/**/*.go"]
    exclude:
      paths: ["internal/adapters/driven/photosearch/**", "**/*_test.go"]
      reason: "The photo-search adapter owns the AI tools; tests may start helper programs."
    content:
      forbid: '"os/exec"'

  go/no-panic-in-library:
    rationale: "Library code returns errors; a panic would stop the bot for the whole group."
    files: "internal/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test helpers may panic to fail fixture setup."
    content:
      forbid: '\bpanic\s*\('

  go/no-process-exit-in-library:
    rationale: "Only cmd decides to end the process; library code returns errors."
    files: "internal/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test helpers are outside the library contract."
    content:
      forbid: '\b(os\.Exit|log\.(Fatal|Fatalf|Fatalln|Panic|Panicf|Panicln))\s*\('

  go/no-direct-stdout-in-library:
    rationale: "Library code writes to a supplied writer or logger, never to process stdout."
    files: "internal/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test output is outside the library contract."
    content:
      forbid: '\bfmt\.(Print|Printf|Println)\s*\('
```

### Shape B: one hexagon per bounded context

`internal/<x>/` is a bounded context from `domain.arclint.yaml`. A context whose model is ours gets a full hexagon (`domain`, `app`, `infra`, `delivery`). A context whose model belongs to someone else gets one folder that translates it, and nothing more. Today that is `gentalent` (ours) and `beeper` (pending, external). This is the owner's `internal/thing/domain,app,infra,delivery/` with "thing" answered: a context.

```text
cmd/gentalent/main.go                 composition
internal/
  gentalent/                          the contest (context gentalent)
    domain/                           model: group.go, nomination.go, member.go
    app/
      command/                        ports.go, nominate.go, cast_vote.go, search_photos.go, ...
      query/                          ports.go, status.go, list.go, help.go
      relay/                          ports.go, poll.go, flush.go
    infra/
      sqlite/                         group_store.go, outbox.go, read_*.go, migrations
      photosearch/                    runs the AI tools
    delivery/
      chat/                           parse.go, routes.go, one file per chat command
      engine/                         poller.go, lanes.go, photo_worker.go, timers.go
      cli/                            run, status, doctor, resolve-send, setup
  beeper/                             anticorruption layer toward Beeper: SDK in, GenTalent's words out
  shared/                             code with no product meaning
```

| Thing | Lives in |
|---|---|
| Commands | `internal/gentalent/app/command/` |
| Queries | `internal/gentalent/app/query/` |
| Photo-search worker | pool in `internal/gentalent/delivery/engine/photo_worker.go`; the use case it runs in `internal/gentalent/app/command/search_photos.go`; the AI tools in `internal/gentalent/infra/photosearch/` |
| Beeper adapter | `internal/beeper/` |
| SQLite | `internal/gentalent/infra/sqlite/` |
| Chat-command parser | `internal/gentalent/delivery/chat/` |

**Path of one new chat command:**

1. `internal/gentalent/domain/`: only when the command brings a rule, recorded first.
2. `internal/gentalent/app/command/<name>.go` (or `query/<name>.go`).
3. `internal/gentalent/delivery/chat/<name>.go`, plus one line in `routes.go`.
4. Only when it stores something new: `internal/gentalent/infra/sqlite/`. Only when it needs a new port: one line in `cmd/gentalent/main.go`.

Not touched: engine, relay, `internal/beeper/`, photo search. Everything a chat command needs is under one folder, `internal/gentalent/`.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/wixregiga/arclint/main/docs/schemas/rules.arclint.schema.json
# GenTalent, shape B: one hexagon per bounded context.
# internal/gentalent is the contest's hexagon; internal/beeper is the
# anticorruption layer toward Beeper; internal/shared has no product meaning.
# Ask before editing: arclint context <path> · arclint rules [selector]

runtime: [go]

scan:
  unknown_imports: warn
  exclude:
    - ".live/**"
    - ".state/**"
    - "welven_da_great_text/**"
    - ".venv/**"

domain:
  file: domain.arclint.yaml

extends:
  - pattern: arclint/hexagonal@0.1.0
    bind:
      domain: "internal/gentalent/domain/**"
      application: "internal/gentalent/app/**"
      driving: "internal/gentalent/delivery/**"
      driven: ["internal/gentalent/infra/**", "internal/beeper/**"]
      shared: "internal/shared/**"
      composition: "cmd/**"

zones:
  command:
    paths: "internal/gentalent/app/command/**"
    description: "Write side: one file per command, its handler, and the ports a handler needs to change the Group."
  query:
    paths: "internal/gentalent/app/query/**"
    description: "Read side: one file per query, its handler, and the read port. Changes nothing."
  relay:
    paths: "internal/gentalent/app/relay/**"
    description: "Taking messages in and sending replies out exactly once, through the chat port."
  chat:
    paths: "internal/gentalent/delivery/chat/**"
    description: "The chat-command parser and reply text: turns message text into one command or query and its outcome into a reply."
  engine:
    paths: "internal/gentalent/delivery/engine/**"
    description: "The only home of goroutines: the poll loop, the ordered command lane, the concurrent query lane, the photo-search workers, and timers."
  cli:
    paths: "internal/gentalent/delivery/cli/**"
    description: "Operator commands: run, status, doctor, resolve-send, setup."
  sqlite:
    paths: "internal/gentalent/infra/sqlite/**"
    description: "The only code that speaks SQL: the Group store, the outbox, read models in read_*.go, and migrations."
  photosearch:
    paths: "internal/gentalent/infra/photosearch/**"
    description: "The only code that starts other programs: runs the AI tools that find a nominee's photos and checks what they return."
  beeper_layer:
    paths: "internal/beeper/**"
    description: "Anticorruption layer: the only code that imports the Beeper SDK; implements the relay chat port in GenTalent's words."

rules:
  hexagon/dependencies-inward:
    rationale: "Dependencies point from wiring through adapters and use cases to the model."
    layers: [composition, driving, driven, application, domain, shared]

  hexagon/no-cycles:
    rationale: "The six parts of the hexagon form no import cycle."
    acyclic: [composition, driving, driven, application, domain, shared]

  domain/standard-library-only:
    rationale: "The model imports the Go standard library and nothing else, internal/shared included; ArcLint's built-in domain_isolation rule refuses a model that imports shared code."
    on: domain
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests are not the model."
    imports:
      internal: []
      external: forbid

  domain/no-effect-imports:
    rationale: "The model decides and performs no effect. Declare the effect as a port in internal/gentalent/app and implement it in an adapter."
    on: domain
    files: "internal/gentalent/domain/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test fixtures may use IO."
    content:
      forbid: '"(os|os/exec|io/fs|net|net/http|database/sql|encoding/json|encoding/xml|encoding/gob)"'

  domain/no-transport-tags:
    rationale: "Model values carry no serialization tags; adapters map them to rows and payloads."
    on: domain
    files: "internal/gentalent/domain/**/*.go"
    content:
      forbid: '`(json|yaml|toml|db):'

  domain/aggregate-has-its-file:
    rationale: "Every recorded aggregate has a file named after it, so a reader finds the root without searching."
    on: domain
    structure:
      each: domain.aggregates
      require: ["internal/gentalent/domain/{name:snake_case}.go"]

  go/concept-named-files:
    rationale: "A file is named for the one concept, command, query or adapter it holds, never for a generic container."
    on: [domain, application, driving, driven, shared, composition]
    structure:
      forbid:
        - "**/model.go"
        - "**/models.go"
        - "**/types.go"
        - "**/utils.go"
        - "**/util.go"
        - "**/helpers.go"
        - "**/common.go"
        - "**/misc.go"

  go/snake-case-files:
    rationale: "Go file names use snake_case so files are easy to find and scan."
    on: [domain, application, driving, driven, shared, composition]
    naming: snake_case

  app/no-direct-io:
    rationale: "Use cases coordinate ports; adapters perform IO. A tripwire for the named imports, not a proof."
    on: application
    files: "internal/gentalent/app/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test fixtures may use IO."
    content:
      forbid: '"(os|os/exec|io/fs|net|net/http|database/sql)"'

  cqrs/command-side-alone:
    rationale: "The command side imports the model and shared code only: never the query side or the relay."
    on: command
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [domain, shared]

  cqrs/query-side-alone:
    rationale: "The query side imports the model and shared code only, so a query cannot reach a port that changes the Group."
    on: query
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [domain, shared]

  relay/alone:
    rationale: "The relay moves messages and replies and knows no command or query; the engine is where the three meet."
    on: relay
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [domain, shared]

  cqrs/read-models-read-only:
    rationale: "Read models in read_*.go run no statement that changes data. A tripwire; the read side also opens the database read-only."
    on: sqlite
    files: "internal/gentalent/infra/sqlite/read_*.go"
    content:
      forbid: '\.Exec(Context)?\(|\b(INSERT|UPDATE|DELETE|REPLACE|CREATE|ALTER|DROP)\b'

  driving/never-imports-wiring:
    rationale: "Delivery reaches use cases through what composition hands it and never imports cmd."
    on: driving
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [application, domain, shared]

  chat/parses-and-renders-only:
    rationale: "The chat-command parser builds commands and queries and words their outcomes. It never polls, sends or schedules."
    on: chat
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [command, query, domain, shared]

  concurrency/engine-owns-goroutines:
    rationale: "Only the engine starts goroutines. The model, use cases and the parser are plain blocking calls, so each is tested without timing."
    on: [domain, application, chat]
    files: "internal/gentalent/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests may start goroutines to exercise blocking calls."
    content:
      forbid: '^\s*go\s+(func\b|[A-Za-z_][A-Za-z0-9_.]*\()'

  beeper/sdk-behind-the-layer:
    rationale: "Only internal/beeper imports the Beeper SDK, so a change in Beeper is repaired in one folder."
    files: ["internal/**/*.go", "cmd/**/*.go"]
    exclude:
      paths: ["internal/beeper/**"]
      reason: "The anticorruption layer owns the SDK."
    content:
      forbid: '"github\.com/beeper/desktop-api-go'

  sqlite/sql-behind-the-store:
    rationale: "Only the sqlite adapter imports database/sql; composition opens the database through it."
    files: ["internal/**/*.go", "cmd/**/*.go"]
    exclude:
      paths: ["internal/gentalent/infra/sqlite/**"]
      reason: "The sqlite adapter owns SQL."
    content:
      forbid: '"database/sql(/driver)?"'

  photosearch/programs-behind-the-finder:
    rationale: "Only the photo-search adapter starts other programs."
    files: ["internal/**/*.go", "cmd/**/*.go"]
    exclude:
      paths: ["internal/gentalent/infra/photosearch/**", "**/*_test.go"]
      reason: "The photo-search adapter owns the AI tools; tests may start helper programs."
    content:
      forbid: '"os/exec"'

  go/no-panic-in-library:
    rationale: "Library code returns errors; a panic would stop the bot for the whole group."
    files: "internal/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test helpers may panic to fail fixture setup."
    content:
      forbid: '\bpanic\s*\('

  go/no-process-exit-in-library:
    rationale: "Only cmd decides to end the process; library code returns errors."
    files: "internal/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test helpers are outside the library contract."
    content:
      forbid: '\b(os\.Exit|log\.(Fatal|Fatalf|Fatalln|Panic|Panicf|Panicln))\s*\('

  go/no-direct-stdout-in-library:
    rationale: "Library code writes to a supplied writer or logger, never to process stdout."
    files: "internal/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test output is outside the library contract."
    content:
      forbid: '\bfmt\.(Print|Printf|Println)\s*\('
```

### Shape C: three hexagons that never import each other

`internal/<x>/` is a hexagon with its own words: the contest, messaging, and photo search. Each has its own `domain`, `app` and `infra`. They meet only in `internal/bot/` (the shell) and `cmd/`.

`[ASSUMPTION]` `messaging` and `photos` are not recorded contexts. This shape needs them recorded through the librarian first. Until then only the house rules `contexts/*-alone` keep them apart; ArcLint's built-in context rules do not see them.

```text
cmd/gentalent/main.go                 composition
internal/
  gentalent/                          the contest (context gentalent)
    domain/                           group.go, nomination.go, member.go
    app/command/, app/query/          as in shape B
    infra/sqlite/                     group_store.go, read_*.go, migrations
    delivery/chat/                    parse.go, routes.go, one file per chat command
  messaging/                          messages in, replies out exactly once
    domain/                           message.go, reply.go, delivery state
    app/                              relay.go and its chat port
    infra/beeper/                     the Beeper adapter
    infra/sqlite/                     outbox and cursor
  photos/                             finding a nominee's photos
    domain/                           bundle.go, manifest checks
    app/                              search.go and its finder port
    infra/agents/                     runs the AI tools
  bot/                                shell: engine, lanes, photo-search workers, timers,
                                      and the translation between the three hexagons
  shared/                             code with no product meaning
```

| Thing | Lives in |
|---|---|
| Commands | `internal/gentalent/app/command/` |
| Queries | `internal/gentalent/app/query/` |
| Photo-search worker | pool in `internal/bot/`; the search use case in `internal/photos/app/`; the AI tools in `internal/photos/infra/agents/` |
| Beeper adapter | `internal/messaging/infra/beeper/` |
| SQLite | `internal/gentalent/infra/sqlite/` and `internal/messaging/infra/sqlite/` |
| Chat-command parser | `internal/gentalent/delivery/chat/` |

**Path of one new chat command:**

1. to 4. as in shape B, inside `internal/gentalent/`.
5. When the command needs something new from a message (for example the message it replies to) or from a photo search: also the other hexagon's `domain`, `app` and `infra`, and the translation in `internal/bot/`. That is more files in two more folders.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/wixregiga/arclint/main/docs/schemas/rules.arclint.schema.json
# GenTalent, shape C: three hexagons that never import each other.
# internal/gentalent is the contest, internal/messaging takes messages in and
# sends replies out, internal/photos finds a nominee's photos. internal/bot is
# the shell where they meet; internal/shared has no product meaning.
# Ask before editing: arclint context <path> · arclint rules [selector]

runtime: [go]

scan:
  unknown_imports: warn
  exclude:
    - ".live/**"
    - ".state/**"
    - "welven_da_great_text/**"
    - ".venv/**"

domain:
  file: domain.arclint.yaml

extends:
  - pattern: arclint/hexagonal@0.1.0
    bind:
      domain: "internal/*/domain/**"
      application: "internal/*/app/**"
      driving: ["internal/*/delivery/**", "internal/bot/**"]
      driven: "internal/*/infra/**"
      shared: "internal/shared/**"
      composition: "cmd/**"

zones:
  contest:
    paths: "internal/gentalent/**"
    description: "The contest's hexagon, recorded as context gentalent: the Group, its commands and queries, its store and its chat-command parser."
  messaging:
    paths: "internal/messaging/**"
    description: "The messaging hexagon: incoming messages, replies sent exactly once, and the Beeper adapter that is the only code importing the Beeper SDK."
  photos:
    paths: "internal/photos/**"
    description: "The photo-search hexagon: what a photo bundle is, the search use case, and the adapter that runs the AI tools."
  bot:
    paths: "internal/bot/**"
    description: "The shell and the only home of goroutines: the poll loop, the ordered command lane, the concurrent query lane, the photo-search workers, timers, and the translation between hexagons."
  command:
    paths: "internal/gentalent/app/command/**"
    description: "Write side: one file per command, its handler, and the ports a handler needs to change the Group."
  query:
    paths: "internal/gentalent/app/query/**"
    description: "Read side: one file per query, its handler, and the read port. Changes nothing."
  chat:
    paths: "internal/gentalent/delivery/chat/**"
    description: "The chat-command parser and reply text: turns message text into one command or query and its outcome into a reply."
  sqlite:
    paths: "internal/*/infra/sqlite/**"
    description: "The only code that speaks SQL: each hexagon's own store, read models in read_*.go, and migrations."

rules:
  hexagon/dependencies-inward:
    rationale: "Dependencies point from wiring through adapters and use cases to the model."
    layers: [composition, driving, driven, application, domain, shared]

  hexagon/no-cycles:
    rationale: "The six parts of the hexagon form no import cycle."
    acyclic: [composition, driving, driven, application, domain, shared]

  contexts/contest-alone:
    rationale: "The contest imports no other hexagon. What it needs from outside is a port in internal/gentalent/app that internal/bot or cmd satisfies."
    on: contest
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [shared]

  contexts/messaging-alone:
    rationale: "Messaging imports no other hexagon; it moves text and knows no command, vote or photo."
    on: messaging
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [shared]

  contexts/photos-alone:
    rationale: "Photo search imports no other hexagon; it is given a subject and returns a bundle."
    on: photos
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [shared]

  domain/standard-library-only:
    rationale: "The model imports the Go standard library and nothing else, internal/shared included; ArcLint's built-in domain_isolation rule refuses a model that imports shared code."
    on: domain
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests are not the model."
    imports:
      internal: []
      external: forbid

  domain/no-effect-imports:
    rationale: "The model decides and performs no effect. Declare the effect as a port in the hexagon's app folder and implement it in its infra folder."
    on: domain
    files: "internal/*/domain/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test fixtures may use IO."
    content:
      forbid: '"(os|os/exec|io/fs|net|net/http|database/sql|encoding/json|encoding/xml|encoding/gob)"'

  domain/no-transport-tags:
    rationale: "Model values carry no serialization tags; adapters map them to rows and payloads."
    on: domain
    files: "internal/*/domain/**/*.go"
    content:
      forbid: '`(json|yaml|toml|db):'

  domain/aggregate-has-its-file:
    rationale: "Every recorded aggregate has a file named after it, so a reader finds the root without searching."
    on: domain
    structure:
      each: domain.aggregates
      require: ["internal/gentalent/domain/{name:snake_case}.go"]

  go/concept-named-files:
    rationale: "A file is named for the one concept, command, query or adapter it holds, never for a generic container."
    on: [domain, application, driving, driven, shared, composition]
    structure:
      forbid:
        - "**/model.go"
        - "**/models.go"
        - "**/types.go"
        - "**/utils.go"
        - "**/util.go"
        - "**/helpers.go"
        - "**/common.go"
        - "**/misc.go"

  go/snake-case-files:
    rationale: "Go file names use snake_case so files are easy to find and scan."
    on: [domain, application, driving, driven, shared, composition]
    naming: snake_case

  app/no-direct-io:
    rationale: "Use cases coordinate ports; adapters perform IO. A tripwire for the named imports, not a proof."
    on: application
    files: "internal/*/app/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test fixtures may use IO."
    content:
      forbid: '"(os|os/exec|io/fs|net|net/http|database/sql)"'

  cqrs/command-side-alone:
    rationale: "The command side imports the model and shared code only: never the query side or the relay."
    on: command
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [domain, shared]

  cqrs/query-side-alone:
    rationale: "The query side imports the model and shared code only, so a query cannot reach a port that changes the Group."
    on: query
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [domain, shared]

  cqrs/read-models-read-only:
    rationale: "Read models in read_*.go run no statement that changes data. A tripwire; the read side also opens the database read-only."
    on: sqlite
    files: "internal/*/infra/sqlite/read_*.go"
    content:
      forbid: '\.Exec(Context)?\(|\b(INSERT|UPDATE|DELETE|REPLACE|CREATE|ALTER|DROP)\b'

  driving/never-imports-wiring:
    rationale: "Delivery reaches use cases through what composition hands it and never imports cmd."
    on: driving
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [application, domain, shared]

  chat/parses-and-renders-only:
    rationale: "The chat-command parser builds commands and queries and words their outcomes. It never polls, sends or schedules, and never sees another hexagon."
    on: chat
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests wire fakes across zones."
    imports:
      internal: [command, query, domain, shared]

  concurrency/bot-owns-goroutines:
    rationale: "Only internal/bot starts goroutines. Models, use cases and the parser are plain blocking calls, so each is tested without timing."
    on: [domain, application, chat]
    files: "internal/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Tests may start goroutines to exercise blocking calls."
    content:
      forbid: '^\s*go\s+(func\b|[A-Za-z_][A-Za-z0-9_.]*\()'

  beeper/sdk-behind-the-layer:
    rationale: "Only the messaging hexagon's Beeper adapter imports the Beeper SDK, so a change in Beeper is repaired in one folder."
    files: ["internal/**/*.go", "cmd/**/*.go"]
    exclude:
      paths: ["internal/messaging/infra/beeper/**"]
      reason: "The Beeper adapter owns the SDK."
    content:
      forbid: '"github\.com/beeper/desktop-api-go'

  sqlite/sql-behind-the-store:
    rationale: "Only sqlite adapters import database/sql; composition opens the database through them."
    files: ["internal/**/*.go", "cmd/**/*.go"]
    exclude:
      paths: ["internal/*/infra/sqlite/**"]
      reason: "The sqlite adapter owns SQL."
    content:
      forbid: '"database/sql(/driver)?"'

  photos/programs-behind-the-finder:
    rationale: "Only the photo-search adapter starts other programs."
    files: ["internal/**/*.go", "cmd/**/*.go"]
    exclude:
      paths: ["internal/photos/infra/agents/**", "**/*_test.go"]
      reason: "The photo-search adapter owns the AI tools; tests may start helper programs."
    content:
      forbid: '"os/exec"'

  go/no-panic-in-library:
    rationale: "Library code returns errors; a panic would stop the bot for the whole group."
    files: "internal/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test helpers may panic to fail fixture setup."
    content:
      forbid: '\bpanic\s*\('

  go/no-process-exit-in-library:
    rationale: "Only cmd decides to end the process; library code returns errors."
    files: "internal/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test helpers are outside the library contract."
    content:
      forbid: '\b(os\.Exit|log\.(Fatal|Fatalf|Fatalln|Panic|Panicf|Panicln))\s*\('

  go/no-direct-stdout-in-library:
    rationale: "Library code writes to a supplied writer or logger, never to process stdout."
    files: "internal/**/*.go"
    exclude:
      paths: ["**/*_test.go"]
      reason: "Test output is outside the library contract."
    content:
      forbid: '\bfmt\.(Print|Printf|Println)\s*\('
```

## 4. Recommendation

**Shape B.**

Judged by the owner's grading words:

| Graded on | A: layer folders | B: context folders | C: three hexagons |
|---|---|---|---|
| Understandable to any agent | The layer names are textbook, but a chat command is spread over three top-level branches and nothing says "contest". | One folder holds the contest; its four subfolders are the ones the owner already named. Beeper is visibly outside it. | Clearest borders, but an agent must first learn which hexagon owns what, and the shell that joins them. |
| Code to get one thing done | 2 new files and 1 line for a plain command. | 2 new files and 1 line for a plain command. | Same for a plain command; more files in two more folders when it crosses a hexagon. |
| Possible code paths for a feature | One: parser, handler, model. | One: parser, handler, model. | Two: inside one hexagon, or across hexagons through the shell. |
| Modularity | Layers are separate. Beeper is one adapter among others. | The contest and Beeper are separate at the top level. A second context gets its own folder with no rule rewrite beyond its bind line. | Messaging and photo search can be changed or replaced without opening the contest. |
| Rules protecting code | 6 pattern rules and 22 house rules; results in section 5. | The same 28 rules with other paths; same results. | 30 rules; same results, and two of the three hexagons are protected by house rules only. |

Trade-offs of B, stated plainly:

- **B is A with different folder names plus one top-level split.** Its rules file differs from A's only in paths. If the owner prefers textbook layer names, A costs nothing extra.
- **B keeps messaging and photo search inside the contest's hexagon** (as `app/relay` and `infra/photosearch`). They are separated by rules, not by folders. `[INFERENCE]` At about 2,500 lines of Python today, three hexagons would add more joining code than they remove confusion. Moving to C later means moving `app/relay` and `infra/photosearch` out and adding the `contexts/*-alone` rules; the command, query and chat folders do not move.
- **`internal/beeper/` holds GenTalent's translation, not Beeper's model.** That is why its zone is named `beeper_layer` (section 1).
- **The content rules are tripwires.** They catch the named imports and statements, not every way around them. The import rules are exact.

## 5. Validation

Each rules file was copied into its own scratch directory under a scratch directory with a Go skeleton matching its zones and the recorded `domain.arclint.yaml`. The skeleton is 19 or 20 small files that exist only to prove the zones and rules resolve. It is not product code and is not in this repository. For each shape there are three trees: clean, deliberately violating (one violation per house rule), and clean with the pending `beeper` context and its anticorruption relation added to the scratch domain file.

| Shape | Tree | `arclint rules` | `arclint check . --format json --no-baseline` |
|---|---|---|---|
| A | `a` clean | exit 0; 28 pattern and house rules, 28 built-in | exit 0; 0 errors, 1 warning |
| A | `a-bad` | exit 0 | exit 1; 27 errors, 1 warning |
| A | `a-acl` | exit 0 | exit 0; 0 errors, 1 warning |
| B | `b` clean | exit 0; 28 pattern and house rules, 28 built-in | exit 0; 0 errors, 1 warning |
| B | `b-bad` | exit 0 | exit 1; 27 errors, 1 warning |
| B | `b-acl` | exit 0 | exit 0; 0 errors, 1 warning |
| C | `c` clean | exit 0; 30 pattern and house rules, 28 built-in | exit 0; 0 errors, 1 warning |
| C | `c-bad` | exit 0 | exit 1; 29 errors, 1 warning |
| C | `c-acl` | exit 0 | exit 0; 0 errors, 1 warning |

No run had a `not_evaluated` entry.

What the results do and do not show:

- **The one warning is the same in every run** and comes from the recorded domain, not from a layout: `aggregate/protects-an-invariant` on `Group`. It belongs to the open domain round (the owner's E5 comments).
- **Every house rule fired in the violating tree, with one exception.** `contexts/messaging-alone` (shape C) was not exercised; it has the same form as `contexts/contest-alone` and `contexts/photos-alone`, which fired.
- **Five pattern rules were not exercised** by the violating trees: `application/through-ports`, `application/core-libraries-only`, `domain/depends-on-nothing-outside`, `driven/implements-ports`, `shared/product-free`. `driving/calls-the-application` fired.
- **`application/core-libraries-only` allows only the package `effect`.** `[INFERENCE]` from the pattern text: for Go this means use cases import no third-party package at all. Not exercised.
- **Compilation:** `go build` passed for the skeleton's model, use-case and shared packages in shape B. The packages that import the Beeper SDK were not compiled, because the SDK was not downloaded.
- **Rule tests** (`arclint rules test`, under `.arclint/tests`) were not written. The violating trees did that job once, by hand.

## 6. Owner decisions this leaves open

1. Which shape.
2. `source/no-comments`: adopt it now, while the Go tree is empty, or never. It needs bskilled's extension copied in.
3. For shape C only: record `messaging` and `photos` as contexts.
