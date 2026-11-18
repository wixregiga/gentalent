# round-one questions

Base: none (no root domain recording exists yet; branch head 9358d96)
Source: the owner's round_one_requirements.ears.md (Arclint Integration, Round 1)

## Q1 · answered
Which business rules does this spec cover, and which does it build? Proposed: record and build rules 1, 2, 4, 5, 6, 8 and 9 plus the shame and help/status functional requirements; leave rules 3, 7, 10 and 11 (feature announcements, batching, acknowledgements, AI limits) and the Config rules and terms for later specs; keep settings in config.json for now.

Answer: Agree. I did not read or review your numbering just the general sentiment that config, secret and preferences are for later

## Q2 · answered
One bounded context for nominating, voting and shaming? Proposed key `talent`: "The group's running contest to name Generational Talents: members nominate people, vote on each nomination, and shame each other over it."

Answer: No. That is for when you actually analyze the domain. That isn't something you just ask the owner. It has clear rules

## Q3 · answered
What is the thing members vote on called? Proposed canonical term Nomination, alias Case (the code's word).

Answer: Yes, but we do not use an alias in the domain. We are recording the domain properly now. we use  the domain language.

## Q4 · answered
When is a Nomination open? Proposed: from its announcement until its verdict (certified or denied) is recorded, including the locked minute after the third vote.

Answer: When a member nominates a person. A person becomes a generational talent when the nomination is finalized and they accepted.

## Q5 · answered
What is a "user" in the rules? Proposed canonical term Member (one of the three people on the roster), alias user. The Owner is one Member.

Answer: Agree, but no alias necessary.

## Q6 · answered
Rule 1's GROUP_MULTIPLE_OPEN_NOMINATIONS = false means the same as GROUP_OPEN_NOMINATION_LIMIT = 1. Collapse rule 1 into rule 5? Proposed: yes, one setting; rule 8 becomes "lowering the limit below the number already open".

Answer: Agree

## Q7 · answered
With several Nominations open, how does a typed vote (!!Y / !!N / !!C) name its Nomination? Proposed: exactly one open needs no name; otherwise the vote must name the nominee (`!!Y Name`) or reply to its announcement; anything else is refused with the list of open nominees. Reactions keep targeting the announcement they are on.

Answer: Agree

## Q8 · answered
How does one message nominate several people (rule 2), and what is SINGLE_MESSAGE_NOMINATION_LIMIT's default and range? Proposed: repeat the command per nominee in one message (`!gt Alice. Pitch !gt Bob. Pitch`); default 3, min 1, max 10; the nominator's yes counts on each.

Answer: Agree

## Q9 · answered
Is "accepted" the domain word for what the bot and README call "certified" (every Member voted yes), with "denied" for the rest? Proposed: yes; the code and replies take the domain word.

Answer: As the product owner, I don't care what the codebase says right now. Focus on what the domain should be, not on backwards compatibility.

## Q10 · answered
Restated in words because Q1's numbering was not reviewed. Proposed build for this spec: several open Nominations with a Group limit and a per-Member limit; several people nominated in one message; votes on several Nominations in one message; lowering the limit while Nominations are open; help and status for several open Nominations; shames with several open Nominations. Later specs: feature-announcement messages, batched verdict and acknowledgement messages, AI switch and AI limits.

Answer: Almost, [quoting "batched verdict and acknowledgement messages"; comment: "Verdict messages need to be resolved in the same way. The acknowledgment messages are the thing we can do later."]

## Q11 · answered
How does one message vote on several Nominations? Proposed: repeat the vote per Nomination, each naming its nominee (`!!Y Alice !!N Bob`); a message whose every vote is valid counts all of them, otherwise none count and the reply says which part failed.

Answer: Almost.
    1. what about spelling? what  about case?
    2. I feel like it could be a range of numbers as long as the numbers stay consistent for what  is open/active or whatever we call it. It's difficult.

## Q12 · answered
A message nominates more people than the Group or Member limit has room for. Proposed: none of them are nominated, and the reply says how many slots are free.

Answer: Agree

## Q13 · answered
After the limit is lowered below the number open, which actions stay allowed, and what does "The after the current state that requires it is satisfied only" mean? Proposed: votes, vote changes and clears, !final, !!gt photos for an open Nomination, shames, help and status stay allowed; only new Nominations are refused, until the open count is below the new limit.

Answer: Agree.
    1. I may have been overcomplicated and is fixed naturally by just having certain actions be allowable in and of itself (separate from the limit). But if we are going towards just using the limit then you will have to come up  with that and if it makes sense for the user.

## Q14 · answered
!final and the one-minute timer with several Nominations locked. Proposed: each Nomination keeps its own timer; !final names its nominee when the nominator has more than one locked Nomination.

Answer: Yes. But I don't know if it should be mentioned for each one or the messge should be batched together by default.

## Q15 · answered
Which Nomination does `!gtshame` with no nominee words pick when several are open? Proposed: the same rule as votes; it must name the nominee when more than one is open, and keeps defaulting to the latest when at most one is open.

Answer: Agree
    1. just have to make sure it's easy to accurately name the nominees or at least reference the right one

## Q16 · answered
scan.exclude in rules.arclint.yaml (review comment on L9). Proposed: exclude `.live/**`, `.state/**`, `welven_da_great_text/**` and `.venv/**`.

Answer: Approve

## Q17 · answered
Should every Nomination get a number when it is nominated, shown on its announcement, status and verdict, never changed and never reused, so votes, !final and shames can reference it by number, by range (`!!Y 12-14`) or by name? Proposed: yes, and the number is the Nomination's identity (NominationNumber).

Answer: Agree

## Q18 · answered
How are nominee names matched in votes, !final and shames? Proposed: as !gtshame does: exact or squashed, any case, with or without emoji; a near-miss or a first name alone gets "Did you mean ...?" and nothing in that message counts.

Answer: Agree. But maybe add nomination/talent/person number?

## Q19 · answered
When one message or one poll finalizes several Nominations, how is the verdict sent? Proposed: one verdict message listing each result in number order, then each nominee's photo strip as its own message in the same order; batching is always on this round and becomes a setting with the Config spec.

Answer: Agree.

## Q20 · answered
What does a bare `!final` do when the nominator has several locked Nominations? Proposed: it finalizes all of them in one batched verdict; `!final 12` or `!final Name` finalizes only those named.

Answer: Agree

## Q21 · answered
What are the outcomes of a finalized Nomination called? Proposed: accepted (every Member voted yes) and denied (anything else).

Answer: Agree

## Q22 · answered
"open/active or whatever we call it": one word for a Nomination that is not yet finalized? Proposed: open, the word the rules already use; no separate "active".

Answer: Agree

## Entry review (round 3)
Owner's words, verbatim:

E1. `gentalent` as a folder because i don't generally like dashes in my folder structure
E2-E4 approved. But just think more deeply about it first. A person is a Nominee and a Person can be voted on. When they receive votes, they are either a talent or not meaning being a talent is a function of being finalized with the correct requirements. A nomination relates to a message about a person  being voted on. I don't know if that can help. I just want to check if we even need to separate out things at all. Just explore it. No, is a suitable answer that I would not mind hearing.
E5. invariant: `group-nomination-limits` , `member-nomination-limits`  or maybe asserting that it stays within the limit or something or that it isn't zero or negative or you can have it be some kind of range or min max. Putting the number in the rule doesn't make the rule flexible or show that it can be counted upon when ran with a different config value that the user selects.
E6 approved. you actually got a lot of what i was saying in e5.

## Q23 · answered
Can the same person ever be nominated more than once (after a denial, or after being accepted)? Proposed: never; each person is nominated at most once, so Nomination and Nominee are one thing and no separate Nominee entry is recorded.

Answer: Disagree. Make it optional with the default being false

## Q24 · answered
What do status and help show with several open Nominations, without polluting the chat? Proposed: status gives one line per open Nomination (number, name, votes in, locked or not), then the record; help adds one line on naming a Nomination by number or name.

Answer: Agree

## Q25 · answered
Functional req 3 reads "I don't  in a way that isn't annoying  and doesn't pollute the group chat". What is the missing verb? Proposed reading: the bot replies at most once per message, refusals from reactions stay silent, and batched verdicts replace per-Nomination messages.

Answer: Don't be so verbose in the message. Keep it short or have a short mode wher e things are dropped that don't need to  be there.

## Q26 · answered
How are live database changes run ("Databases must be easy to migrate"; the one-voting-case index must go)? Proposed: numbered forward-only migrations, each applied once at bot startup in one transaction after the deploy's ballot backup, recorded in meta; no compatibility paths.

Answer: I don't know. Make that determination based upon best practice.

Determination (assistant, delegated by the owner): numbered forward-only migrations tracked with SQLite's `PRAGMA user_version`, each applied once at bot startup inside one transaction, after the deploy's ballot backup; table changes use SQLite's create-copy-rename procedure; no compatibility paths.

## Q27 · answered
"It must be capable of running multiple processes at a time": does this spec move `!!gt` photo searches out of the message transaction so other commands keep working during a search? Proposed: yes; with several open Nominations a minutes-long search blocking every vote is no longer acceptable.

Answer: What? IDK. Maybe?  I don't know what you mean. My answer is a summation of the following:
    1. First and foremost: CQRS
    2. You can still queue up messages or buffer them right?
       1. order of which you answer messages doesn't HAVE to depend on an individual message being received right? Like you can receive more than one
       2. **SEE IF THERE IS A GOLANG LIBRARY INSTEAD OF PYTHON AS WELL**
    4. We  can SEARCH simultaneously for photos
    5. I'm telling you to make it fast and concurrent not break functionality (not sure if you were doing that or not just need to make sure you're not breaking some undefined feature or some feature you don't know about right now)
    6. Also, this is partly why i talked about having an acknowledgment be done too. There's many parts to something like this.

## Comments on round 3 reply (verbatim)
On "the number is NominationNumber ... a person number would only make sense with a separate Person entity": But that's what i'm saying. A person number is a nomination number. Please push back if I am wrong here. I want to see you push back appropriately and I encourage it here.
On "because the person outlives any one Nomination": They could. The way we code it now makes that impossible for no reason

## Q28 · answered
With renomination turned on, who can be nominated again: a denied person, an accepted person, or both? Proposed: both; every Nomination keeps its own permanent outcome.

Answer: I don't know anymore dude

## Q29 · answered
A Nominee with several finalized Nominations: when are they a Generational Talent? Proposed: while their most recent finalized Nomination is accepted.

Answer: Can not renominate people that are generational talents. You can use the word certified if you want.  I don't know.

## Q30 · answered
Short replies: always short, or a short mode as a setting? Proposed: always short; no mode.

Answer: Agree

## Q31 · answered
CQRS shape. Proposed: commands (nominate, vote, finalize, shame, feedback) change the Group and are applied in message order; queries (help, status, list) change nothing and are answered from the read side; photo searches run after the command commits, concurrently, one per Nomination; replies go out as each is ready; the photo-search acknowledgement (one short message, batched when several start together) moves into this spec.

Answer: Didn't read. Just do things right. Been here a long time

## Q32 · answered
Go: Beeper publishes an official Go SDK (github.com/beeper/desktop-api-go). Does this spec build the bot in Go? Proposed: yes, in this spec, because the change already rewrites the modules that assume one open Nomination.

Answer: Agree. Golang for the win. I don't know if it's hexagonal or vertical but `internal/thing/domain,app,infra,delivery/`, `internal/shared/` seems cool. consult your designers

## Q33 · answered
Not breaking unknown features. Proposed: before any code changes, inventory every current behavior (README commands, existing tests, the observed Beeper contract) as tests; each behavior the new rules would change is listed for the owner's decision instead of dropped.

Answer: (left blank; Q27 item 5 already says: "I'm telling you to make it fast and concurrent not break functionality (not sure if you were doing that or not just need to make sure you're not breaking some undefined feature or some feature you don't know about right now)")

## Comments on round 4 reply (verbatim)
On "the model needs two identities. Nominee is the person and Nomination is one attempt": Yeah, but  what happens when we ask to see people who were rejected? they are not talents and they are not up for nomination if multiple-nomination is off. Therefore it doesn't accurately describe the data object. The data object is therefore a person. A person moves through different STATES of being. PErson can be a nominee, talent, can be rejected or a reject. a person can then be re-nominated. A person can be nominated by one member. Renominated by another. You see what I mean? But the nomination number could be separate too. I just want you to think about the model first! Push back or ask fable 5.2 or astra 6 or 6.1 sol high
On E5 "NominationLimit, defined as \"How many Nominations a limit allows.\"": No, that would just be a list of nominees. that's just a collection of nominees.
On E8 "A person a Member has nominated at least once.": You keep having to use the noun that I talk about in the definition of another noun lol. It's possible to be context specific or aggregate specific. Both can be right in certain contexts. if that is true something like an Anti Corruption Layer is needed per DDD. Look it up. Ask librarian, etc.
On E9 "A Member nominates a person only when that person has no open Nomination.": No, that is not the hard rule. It is that it is not more than is configured.
On E10 "A Member nominates a person who was nominated before only when the Group allows renomination.": agee
On Q32's layout `internal/thing/...`: thing isn't actually thing
On "internal/<feature>/{domain,app,infra,delivery}/": no, not feature either unless that's what is supposed to go there. Just a possible thing is in there

## Second opinion (round 5)
An independent Fable 5.2 review, given the owner's words and the librarian protocol, agreed on: one `gentalent` context; Group as the aggregate; Person and Nomination as separate entities; Nominee, Generational Talent and denied as states of a Person; no anticorruption layer inside the contest; an anticorruption layer at the Beeper edge; the owner's layout matching arclint/vertical with the context as the folder. It differed on GenerationalTalent (no specification) and flagged that a per-Person open limit above one makes voting by name ambiguous.

## Q34 · answered
The person open nomination limit ("not more than is configured"): what default and range? Proposed: default 1, range at least 1; above 1, a vote by name on a Person with several open Nominations is refused and asks for the number.

Answer: Whatever. Feel like i answered this already.  I'm tired

## Q35 · answered
Go layout. Proposed: `internal/gentalent/{domain,app,infra,delivery}/`, `internal/shared/`, `cmd/gentalent/`, adopting arclint/vertical with its `application` zone bound to `internal/*/app/**` and its `app` zone bound to `internal/*/delivery/**`; the Beeper translation lives in `internal/gentalent/infra/beeper/`.

Answer: see below (use fable 5.2) don't tell it  any made up hard rules that you think I have. I just want easy to understand, manageable code. You are responsihblie for the codebases maintainabiulity long term and the complexity and understandability of fthe code between agents of different models, effort level and providers is what you will be graded on. The amount of code it takes for a model to get somet hing done. The code paths that are possible for features, how modular it is, how the rules for arclint are used to protect code. That is what is good and what you are graded on Tell fable that
- go an ask the latest orchestrator for bskilled to see what they learned from using  arclint they have some  good rules and some rules to avoid.
- I want you to have a better separation of context therefore i reject your vertical layout. I want. Try again. Hexagonal. Give me a couple of shapes and rules and stuff

I am done

## Comments on round 6 entries (verbatim)
On N8 beeper context: this is just  an integrator. What happens when we go off of beeper or want to use a new messaging platform instead of google?
On C1 Nomination: Nomination: One members proposal that a person be certified a Generational talent.
Renomination: One members proposal that a person be certified a Generational Talent despite being rejected as a generational talent before.
On N7: nomination-limit-greater-or-equal-to-one
or you can do:
nomination-limit-not-less-than-one
On N2 GenerationalTalent: A person who has been certified
On N3: No, see this is why you shoul fucking  listen t o me. I can't even tell you all the ways this doesn't make sense
On N1 Person: someone that has been nominated at least once, by a member to be a Generational Talent. A person can be rejected or certified through the nomination process.
On N2 GenerationalTalent: A person who has been certified through the Nomination Process
On N3: - A person may only have one open nomination at a time.
- A person may only be re-nominated through the re-nomination process.F
- A person may only be renominated if their certification statius (or whatever we call it) has been rejected and the group allows re-nomination.
On N4: and only through the renomination process.
On N8: this is a technology that we are using and an api. this is  infra  shared infra or delivery  i don't know
On N9: messages not beeper messages is how we on messages in the domain. right now we use google messages and beeper api and bbctl. Man, we have to use our own stuff and isolate third party apps apps and things.
On Shape C: I would even think about ai in there  or something. ai in the domain you just got to ensure you are separating things in a way that is easy to maintain and modular and understandable  for agents. YOU ARE DIRECTLY JUDGED ON HOW EASY TO MAINTAIN THE CODEBASE IS, HOW EASY IT IS TO MODIFY WITHOUT BREAKING OTHER THINGS, HO COHERENT IT IS, HOW STRUCTURED IT IS

## Q36 · answered
How does a Member start the renomination process? Proposed: its own command, `!gtrenominate Name. Reason` (and `!!gtrenominate` with photos), refused unless the Person is rejected and the Group allows renomination.

Answer: Agree with 36 and 37.

## Q37 · answered
Do Renominations count against the Group and Member open nomination limits? Proposed: yes; an open Renomination is an open Nomination for every limit.
On "Whether to adopt source/no-comments now, while the Go code is still empty, or never.": do this! yes!

Answer: Agree with 36 and 37.

On Renomination (with the Q36/Q37 answer): "A renomination is a kind of nomination. should have the same functions or somet hing . or int ernface. you know how to write good golang code

## Comments on P1 to P4 (verbatim)
On P2 "three contexts (gentalent, messaging, ai), with Beeper as an adapter.": so what makes gentalent a context? why is it better or worse than having nominations, person, contest, voting, e t c.? have we thought of any others? have we looked at the domain-librarian skill? have we actually called it? Have we launched an agent/subagent? contacted an advisor?
On C2 Group: Group is the members    and their shared chat space that the owner
On N6 message-nominates-all-or-none: sure, i thought i said yes to this already
On P4: agree
On P1 CertificationStatus: I guess. this is exhausting
On "Beeper and bbctl as adapters inside messaging.": I agree with this portion

## Round 7 (verbatim)
Hey, i'm going on a break. you have decision authority from here on as long as you adhere to what i have already agreed to
On Shame: shames are messages to members of the group that borrow id's from persons or nominations or something right? I dont' know I'm tired but you can sumise from something like that.
On the Group sentence guess: no. i'm tired man
On relations: follow ddd and shit, i'm tired. orchestrate this and make sure it's maintainable and makes sense
On ai: i already said yes, fuck
On messaging: come the hell on I said y es
On renaming to nominations: llord approved
On the context review: do that and then orchestrate and coordinate-verified in a new thread. You can start a new t3 orchestrator thread with the directions and the things that you learned from this one or you can do it yourself with subagents. I don't care i am just tired and need to see progress.

## Round 8 (verbatim, during orchestration)
"hey, let's make sure we don't  overtake the python process until we have completed the golang features. I don't want it to stop working while we are h ere"

## Orchestrator decisions
Made under the owner's round 7 delegation ("you have decision authority from here on as long as you adhere to what i have already agreed to"). Each names its reason.

- **D1 Promotion.** The root `domain.arclint.yaml` was created from this spec's copy with `arclint domain define`, entry for entry; a normalized comparison of the two files shows no difference. Reason: grill-domain section 3 step 1; Base is none, so every entry is an addition and no entry changed on both sides.
- **D2 Python keeps running until Go is complete.** The Python package, its tests and the Python run and deploy path stay working on this branch. Go is added beside it; `mise run test` and the deploy check run both suites. The switch of `scripts/run-gentalent-live.sh` to the Go binary is one separate, final commit made only once the Go suite covers every inventoried behavior, and the Python package is removed only after that. Reason: Round 8.
- **D3 Layout and rules.** `rules.arclint.yaml` extends `arclint/hexagonal@0.1.0`, binding `port/` into the application layer, and adds one zone per context (`nominations`, `messaging`, `ai`, each importing only `shared`), a `port` zone that holds every interface (new extension `ports/interfaces-in-port-zone`, discovering ports by location as the bskilled lesson advises), CQRS import contracts, and the IO, SQL, SDK, program, goroutine, panic, exit and stdout tripwires. A bskilled-style "contract-only consumers" rule is not added: the import contracts already make it impossible for a use case to name an adapter type, so it would duplicate an exact rule with a heuristic one. Every house rule has a fixture under `.arclint/tests`. Reason: the task's decided layout and the bskilled short list.
- **D4 Two databases.** `nominations.sqlite3` and `messaging.sqlite3`, each owned by one hexagon with its own `PRAGMA user_version` migrations. One file would need one migration sequence shared by two hexagons that may not import each other. Exactly-once across them uses a transactional outbox in nominations and idempotent enqueue by key in messaging (`doc/architecture.md`). Reason: Q26 and the three-hexagon decision.
- **D5 History import without touching the Python file.** Migration 1 of each Go database imports its share of `gentalent.sqlite3` read-only. The Python file is never changed, so the Python bot stays runnable and the file stays a backup. Reason: Q26 ("must migrate without loss") and Round 8.
- **D6 Queries in the ordered lane.** Queries are separate read-side handlers on a read-only connection but are answered in message order with commands, so a status sent after a vote shows that vote. Only photo searches run concurrently, because they take minutes and queries take milliseconds. Reason: Q27 and Q31 ("commands apply in message order; queries change nothing"), without surprising a Member.
- **D7 Interrupted photo search.** A search marked started with no result when the bot starts is recorded failed and the Group is told; it is never rerun. Reason: Q31 "at most once per Nomination"; the Python bot reran it after a restart, which the deploy script only avoided by refusing to restart mid-search.
- **D8 Limit defaults.** `group_open_nomination_limit` 1, `member_open_nomination_limit` 1, `single_message_nomination_limit` 3 (Q8), `allow_renomination` false (Q23). The group and member defaults keep today's one-at-a-time behavior until the owner raises them in `config.json`. Reason: the owner never set those two defaults, and "not break functionality".
- **D9 No bbctl adapter.** The Python bot never calls bbctl (no reference in `gentalent/` or `scripts/`), so none is built. A bbctl adapter belongs in `internal/messaging/infra/` when a feature needs it. Reason: no stand-ins.
- **D10 Feedback stays in nominations.** `!gtfeedback` and `!gtsuggest` keep their behavior as a nominations command with its own table, not a Group change and not a domain entry: it records a Member's idea and enforces no business rule. Reason: the three contexts the owner approved; messaging "knows no command".
- **D11 One text form (B7).** Messaging hands nominations the message text with HTML tags removed and entities decoded, the form the own-message check already uses, and commands match on it. Reason: one text form for every check; `doc/beeper-api.md` item 7 shows Beeper storing formatted text as HTML.
- **D12 Certified Person refused (B44).** Nominating or renominating a certified Person is refused, pointing to the Nomination that certified them by number. Reason: Q29 ("Can not renominate people that are generational talents"), R6, R7.
- **D13 Photo search failure (B52).** An ordinary failure leaves the Nomination open without photos and replies once that no photos were found for that Nomination; it is not retried (at most once per Nomination). The provider's error text goes to the log, never the chat. Reason: Q30 (short replies), and provider stderr can carry file paths, which are personal data under AGENTS.md.
- **D14 Locked and finalize replies (B59, B81).** A refusal on a locked Nomination names it by number. A bare `!final` finalizes every locked Nomination the sender nominated (Q20); with none locked it says which of the sender's Nominations still wait for votes. `!final <number|range|name>` finalizes only those named, and every named one must be locked and the sender's, or nothing is finalized (Q11, Q12 "one bad part voids the whole message").
- **D15 Outcome words (B84).** Certified and rejected, as the recorded CertificationStatus and the owner's own definitions in round 6 say ("A person can be rejected or certified"). The Python `denied` phase and `denials` rows import as rejected. Q21's "accepted or denied" was superseded by those definitions.
- **D16 One shame per Member per Nomination (B102).** A Shame refers to its Nomination by number; each Member gives at most one per Nomination. A renominated Person's new Nomination allows a new shame. Reason: the recorded Shame definition, and today's rule is per case.
- **D17 Lists (B124).** `!gtlist` lists certified Persons; `!gtlist losers` lists Persons whose certification status is rejected, each once. The second section lists open Nominations by number with their vote count, or says none are open. Reason: Q24.
- **D18 Names escaped (B129).** Person names, Member names and every other typed text are escaped like shame text before sending. Reason: names are typed by Members; `doc/beeper-api.md` item 7 gives the escaping rule and why.
- **D19 Short replies (B132).** Each reply keeps only what the Member needs now: the outcome, the Nomination's number and name, and the next action. Rule explanations live in `!gthelp`. Every reply string that changes is marked CHANGES in `behavior-inventory.md` for the owner. Reason: Q25, Q30.
- **D20 Uncertain send stops that flush (B137).** Kept: one uncertain send ends that flush and later replies wait for the next poll. Reason: an uncertain send usually means Beeper is unreachable, and sending on would mark more replies uncertain.
- **D21 HTTP behavior through the SDK (B149).** The Beeper SDK client is built with an HTTP client that keeps today's behavior: 15 s timeout (30 s uploads), redirects refused, proxies bypassed, no automatic retry of any request. Reason: "no automatic write retry was made" is what keeps a reply from being sent twice.
- **D22 A failing message (B169).** A message whose handling fails with an error that is not a refusal stays unhandled, is logged, and is retried on the next poll, without stopping the process; the poll and the photo searches keep running. Reason: same retry semantics as the Python crash-and-restart, without restarting.
- **D23 Person identity (B174).** PersonID is the squashed name key the Python bot derives; the import maps `nominee:<key>` to `<key>` and keeps every alias pointing at its Person. Reason: names are matched by that key today (B-entries on matching), and history must survive.
- **D24 Permanent outcomes (B176).** Every finalized Nomination's outcome is permanent (triggers refuse UPDATE and DELETE), and a Person's certification status is that of their latest finalized Nomination. Shames stay permanent the same way. Reason: README "Denials are permanent at the database level", generalized to renomination (Q23, Q28).
- **D25 Config checked at start (B216).** A missing or invalid `media_dir`, `ai_providers` or other required key refuses to start and names the key. This is a startup guard, not a domain rule. Reason: a crash in the middle of a Member's command is worse than a refusal to start.
- **D26 One media folder per Nomination (B227).** Photos go to `<media_dir>/nominations/<number>-<slug>/`. Reason: concurrent searches (Q27) must never share a folder.
- **D27 Deploy refuses mid-search (B241).** For the Go bot the deploy refuses while a photo search is started and unfinished, read from the nominations database read-only, because a restart abandons it (D7). Reason: same protection the Python deploy gives, against the new failure mode.
- **D28 Hexagons never import internal/shared.** ArcLint's built-in `domain_isolation/model-imports-nothing-outside-itself` treats all of `internal/<context>/**` as that context's code and forbids it to import code no context holds, `internal/shared` included (found by the messaging lane). So only cmd imports `internal/shared/sqlitedb`: it opens each database and runs the migrations each hexagon's `infra/sqlite` exports as plain functions, and the house rules `contexts/*-alone` now allow no internal import at all. Reason: the built-in rule states the DDD direction (dependencies run toward the model); copying the helpers into each hexagon would only move code to clear a finding.
- **D29 Reactions keep every key.** `CountReactions` and messaging's `Reactions` carry every reaction key per Member (`map[string][]string`), so a Member with both a yes and a no counts as none (B67, KEEP). `cases.messages_sent` is not imported: it only made a repeated vote silent once the announcement was queued (B58), which always holds in Go, so a repeated vote is always silent. Reason: B58, B67; the column holds no history.
- **D30 Red state verified, spec in progress.** On commit 5f6a688, `go test ./internal/nominations/...` fails to build (no production code) and the complete ArcLint check (exit 1, no `not_evaluated`) reports this spec's findings: `aggregate/root-declared` for Group, 10 `ubiquitous_language/terms-declared-in-code`, `specification/satisfaction-method` for GenerationalTalent, `domain/aggregate-has-its-file`. Every invariant and assertion records its failing test with `--verified-by`. The spec folder moved to `inprogress/`. Reason: grill-domain section 3.
- **D31 Live lock shared with the Python bot.** Messaging's group binding hashes the chat, account, owner and roster exactly as the Python bot's `config.binding` does, so the machine-wide live lock file is the same file for both bots and a Go bot and the Python bot can never both answer the Group (AGENTS.md). This is an interlock between two programs that run on the same machine during the switch, not a compatibility path. Reason: Round 8 keeps the Python bot running while the Go bot is built.
- **D32 Messaging stays command-free.** Messaging loads the linked message of every reply, instead of the Python bot's fetch only for `!gtshame`, so it needs no predicate about commands; the cost is one local API read per reply message. Forwarding-only wrappers (`Acknowledger`, `query.Inbox`, `query.Status`) were removed: cmd calls the store for those reads. Discovery port types carry no JSON tags; cmd renders their output. Reason: "Messaging ... knows no command"; wrappers added a type and a path without behavior.
- **D33 Owner rules recorded in the domain's words.** The nominations lane recorded the voting, finalizing, photo and shame rules the owner already wrote in README.md and `doc/plans/next-features.md` (evidence quoted in the merge of 2a5ed4f). Three keys and four statements were then restated in the recorded language: `certified-by-unanimous-yes` ("certifies its Person", "rejects", not "nominee" and "denies"), `finalized-a-minute-after-the-last-vote` (was `result-posts-after-one-minute`: posting is messaging's word), `photos-searched-at-most-once-per-nomination` (was `ai-runs-at-most-once-per-nominee`, per Q31 and D26), `one-shame-per-member-per-nomination` (was `one-shame-given-per-nominee`, per D16). Methods and tests follow the keys. Reason: Q3 and Q5 ("we use the domain language", no aliases); D15, D16.
- **D34 One path through app.** Nominations' delivery always calls an `app` handler, even where the handler only forwards to its port (answer, mark relayed, record feedback, the four queries), so every chat command has the same shape: delivery, handler, port. Messaging has no delivery adapter; cmd drives it and reads its store directly (D32). Reason: the owner grades "the code paths that are possible for features"; one shape per hexagon beats saving a few small files.
- **D35 Review fixes and the reply-to vote.** An independent GPT-6.1 Sol review of cf6d499 proved six defects (a range that overflows and hangs the bot; votes ignoring the announcement they reply to; a final message ID claimable by two replies; the lock minute starting at the message's sent time instead of the processing clock, B65; manifest.json and the AI summary dropped, B234 and B50; certification status read from the latest Nomination instead of the latest finalized one, D24). To make a reply to an announcement name its Nomination (Q7), messaging's `Linked` carries the opaque tag of the bot reply it points to and `chat.Quoted` carries `Announces`, which cmd translates from `announces/<n>`. Reason: Q7 ("or reply to its announcement"), B50, B65, B234, D24.
