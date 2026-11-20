# multiple-gentalent-groups questions

Base: 6300d3f (last commit of domain.arclint.yaml)
Worktree base: fc79df1 (local main)
Source: the owner's request, verbatim:

> $coordinate-verified using $grill-domain  first for a feature that allows multiple gentalent groups to run. the architecture shouldn't be a drastic change. there's only a couple of places that need a change. you'll have to decide how the sql  is going to be handled I think but other than that, not too much change is predicted.

Facts found (not decisions):

- In `internal/nominations/domain`, `Group` is already an aggregate with `GroupID`, and all vote/lock/name-fix calculations already use `len(g.members)` rather than a hardcoded `3` (`group.go`, `vote.go:155`, `nominate.go:200`, `name_fix.go:270,294`).
- The hardcoded 3-member checks live in `cmd/gentalent/config.go:131`, `internal/messaging/infra/beeper/chat.go:27`, and `cmd/gentalent/discovery_commands.go:155`, and in three entries of `domain.arclint.yaml` (`Member`, `votes-change-until-the-third-vote`, `finalized-only-when-all-votes-are-in`), which also carries the open question `group-of-any-size`.
- Both SQLite stores (`internal/nominations/infra/sqlite` and `internal/messaging/infra/sqlite`) assume one Group per database file: `meta.group` / `meta.binding` enforce ownership, primary keys (`nominations.number`, `persons.id`, `aliases.alias`, `who_searches.number`, `outbox.tag`) are unscoped, and triggers enforce immutability.
- `worker.LockLiveGroup(binding)` already locks per Group (`live-<binding[:16]>.lock`).
- `internal/nominations/delivery/chat/photos.go:28` (`<media_dir>/nominations/<number>-<slug>`) and `who.go:157` (`<media_dir>/who/<number>`) would collide across Groups if two Groups share one `MediaDir`. Shame GIFs live in `<media_dir>/shame-gifs/`.
- `arclint agents hooks install` currently supports only `claude` and `codex` (both already `installed: complete` in user config); there is no `antigravity` host target in `arclint`.

Owner note on hooks: "you may continue without the hooks. just note that it wasn't possible with arclint today."

## Round one

## Q1 · answered
Group isolation (domain boundary): When multiple Groups run, is every Group completely independent (separate Persons, Nominations numbered from 1 per Group, certification/rejection outcomes, Shames, NameFixes, WhoSearches, and per-Member daily `!!gtwho` counts), or should anything be shared across Groups (such as a Member's daily `!!gtwho` count when they belong to two Groups, or a shared Person list)?

Answer: 1-5 agreed.
(Agreed to recommended answer: complete domain independence per Group; share only Beeper connection, AI providers / `photo_search_workers`, and the shame GIF bank.)

## Q2 · answered
Group size (`group-of-any-size`): Today `domain.arclint.yaml` has an open question (`group-of-any-size`) and three entries that still say "three" / "third" (`Member`, `votes-change-until-the-third-vote`, `finalized-only-when-all-votes-are-in`), while `config.go` and `beeper/chat.go` require exactly 3 Members (even though `internal/nominations/domain` already uses `len(g.members)`). Should this feature also allow Groups of any size (e.g., at least 2 Members, or 1+ if you want solo test chats) and update those three domain entries to every Member / the last vote, or keep every Group at 3 Members for now?

Answer: 1-5 agreed.
(Agreed to recommended answer: lift the 3-member cap, update `Member`, `votes-change-until-the-third-vote`, and `finalized-only-when-all-votes-are-in`, and close `group-of-any-size`; minimum member count deferred to Q6.)

## Q3 · answered
Runtime and config model: How should multiple Groups run? (a) One `gentalent` process/daemon and one `config.json` with a `groups` list (inheriting shared top-level settings like `beeper_url`, `token_file`, `poll_seconds`, `media_dir`, `ai_providers`, `photo_search_workers`, while keeping existing single-group `config.json` valid); or (b) separate config files and separate `mise` daemons, one process per Group?

Answer: 1-5 agreed.
(Agreed to recommended answer: (a) one `gentalent` daemon and one `config.json` with a `groups` list, keeping single-group `config.json` valid.)

## Q4 · answered
SQLite storage layout: How should SQLite store multiple Groups? (a) Separate SQLite pair (`nominations.sqlite3` + `messaging.sqlite3`) per Group (existing `.state/{nominations,messaging}.sqlite3` stay untouched for the original Group; new Groups get their own folder such as `.state/groups/<name>/`, overridable per Group); or (b) one shared SQLite pair for all Groups with a `group_id` column migrated onto every table, trigger, unique index, and query across both hexagons?

Answer: 1-5 agreed.
(Agreed to recommended answer: (a) separate SQLite pair per Group.)

## Q5 · answered
Per-group settings and Beeper accounts: Should each Group be able to override Group-level settings (`group_open_nomination_limit`, `member_open_nomination_limit`, `single_message_nomination_limit`, `allow_renomination`, `allow_name_fixes`, `name_fix_agreement_percent`, `name_fix_hours`, `who_search_daily_limit`, `photo_layout`, `announce_reaction_votes`, plus `account_id` / `owner_id` if a Group is on another Beeper network), inheriting top-level defaults when omitted — or should every Group share one global set of settings?

Answer: 1-5 agreed.
(Agreed to recommended answer: per-Group overrides for all Group-level settings plus `account_id` and `owner_id`, inheriting top-level defaults when omitted.)

## Round two

## Q6 · answered
Minimum Members per Group: To finish Q2 and settle the Group size rule, what is the minimum number of distinct Members in a Group? (a) At least 2 distinct Members (including the owner), so every Group is a real multi-person vote and supports `!gtshame`; or (b) at least 1 Member (the owner), allowing a solo test chat where nominating immediately locks at 1/1?

Answer: all approved
(Agreed to recommended option (a): at least 2 distinct Members including the owner.)

## Q7 · answered
Config layout and default DB/media paths:
- Flat single-group `config.json` (`chat_id` + `members` at top level, no `groups` array) stays valid unchanged.
- Adding `"groups": [...]` runs multiple Groups; each entry requires a unique slug `"name"` (letters/digits/`_`/`-`), `"chat_id"`, and `"members"`, and may override `"account_id"`, `"owner_id"`, any Group setting, `"nominations_database"`, `"messaging_database"`, or `"media_dir"`.
- Omitted DB paths in `"groups"` default to `.state/groups/<name>/nominations.sqlite3` and `.state/groups/<name>/messaging.sqlite3` (the original Group can point explicitly to `.state/nominations.sqlite3` and `.state/messaging.sqlite3` without moving files).
- Omitted `"media_dir"` in `"groups"` defaults to `<top-level media_dir>/groups/<name>/` for nomination and `!!gtwho` photos, while shame GIFs stay shared in `<top-level media_dir>/shame-gifs/`.
- Startup refuses duplicate group names, duplicate `chat_id`s, or two Groups sharing a database path.

Answer: all approved

## Q8 · answered
Fault isolation and `doctor` across Groups:
- During `run`: each Group polls and handles errors independently, so a transient API error or roster mismatch in one Group is logged with that Group's name and never stops other Groups.
- During `doctor --chat`: checks every configured Group's chat and roster, reporting failures by group name.
- During `doctor --contract` (used by `mise run deploy`): verifies every Group's chat and roster, and runs recent-sent-reply contract checks across all Groups that already have sent replies (requiring at least one sent reply across the configured Groups overall, so adding a brand-new Group with zero replies yet does not block deploys).

Answer: all approved

## Q9 · answered
CLI commands with multiple Groups:
- Add optional `--group <name>` to `run`, `arm`, `doctor`, `status`, `feedback`, `busy`, `backup`, and `resolve-send`.
- Without `--group`: single-group configs keep their exact existing CLI output; multi-group configs run `run`, `arm`, `doctor`, `busy`, and `backup` across all Groups (`busy` exits non-zero if any Group has a running AI search, naming the Group), return JSON keyed by group name for `status` and `feedback`, and require `--group <name>` for `resolve-send`.

Answer: all approved

## Q10 · answered
Approve the exact domain recording entries (and moving from `draft` to `inprogress` once recorded):
1. Update `entity` `nominations / Group / Member` (`identity: MemberID`): `One of the people in the Group.`
2. Remove `assertion` `nominations / Group / votes-change-until-the-third-vote` and define `assertion` `nominations / Group / votes-change-until-the-last-vote` (`on: CastVote`): `A Member can vote, change their vote, or clear it, until the last vote is in.`
3. Update `assertion` `nominations / Group / finalized-only-when-all-votes-are-in` (`on: Finalize`): `A Nomination is finalized early only once every Member's vote is in.`
4. Define `invariant` `nominations / Group / group-has-at-least-two-members` (if Q6 = a): `A Group has at least two distinct Members.` (Or if Q6 = b: `group-has-at-least-one-member`: `A Group has at least one Member.`)
5. Remove `question` `nominations / group-of-any-size`.

Answer: all approved
