# fix-nominee-spelling questions

Base: ef6c145 (last commit of domain.arclint.yaml)
Source: a Member's feedback, read with `.live/bin/gentalent feedback --live` (2026-11-19T01:25Z, kind feedback): "give us a way to fix spelling mistakes for the people nominated that doesn't break the integrity of the system. 1 way is to use ai judge but that complicates the deterministic first policy. 2nd way is.... There's gotta be some second way lol". A Member's request, not an owner decision.

Facts found (not decisions):

- A Person's identity is the typed name folded: lower case, NFKC, letters and digits only (`internal/nominations/delivery/chat/person_key.go:13`). The first spelling becomes the stored name (`INSERT OR IGNORE INTO persons`, `internal/nominations/infra/sqlite/group_rows.go:185`).
- So a misspelling makes a different Person. Nominating the right spelling later starts a second Person, with its own nomination history, outside one-open-nomination-per-person and the renomination rules.
- The `aliases` table already maps several spellings to one Person, but only the identity itself is ever written to it (`group_rows.go:188`).
- Nominations, votes and shames refer to the Person by identity, not by spelling. Finalized outcomes are permanent by trigger; `persons.name` is not.
- The photo search uses the stored name (`internal/nominations/delivery/chat/photos.go:26`).
- The live ballot (read-only `status --live`) shows misspelled names on one open Nomination and one certified Nomination.

## Q1 · answered
Who may ask for a spelling to be fixed: any Member, or only the Person's nominator?

Answer: Anyone can request it but  a config maybe stops it from going to AI automatically? or maybe it's just  a command line thing that sends something on the bots behalf? or maybe
   1. Maybe it can be gtfix and it can only be called like 5 times a day. It will have AI be the judge it would just fix the names in the database. 
   2. Or maybe it is something that has a cap but it just runs once over the names and changes them to their proper spelling?

## Q2 · answered
What makes a fix take effect, deterministically, with no AI? (a) the Member who asks, alone; (b) the asker and one other Member; (c) every Member agrees, the same standard as certification.


Reply (not yet an answer): let's say I agree, how does that work prior to anyone else voting on it? 
   1. I say that if the owner of the channel or the user that was..... I have no idea. 

**I have no idea how to responsibly do this, can you ask sol 6.2 from a UI/UX user experience perspective in a side chat?**

Answer: I agree with sol

## Q3 · answered
Must the new spelling be close to the old one (the close-spelling measure votes already use), or may it be any name?

Answer: any name

## Q4 · answered
Which Persons can be fixed: only those with an open Nomination, or also certified and rejected ones?

Answer: yes they are the same person unless the pictures returned the wrong person. but what stops me from nominating a radically different person? 
   1. The only correct ay is to limit the fix to one time and one time only. and only to open nominations   and .... there are some other rules in here that solve it to

## Q5 · answered
After a fix, does the old spelling still name the same Person (for votes, `!final`, `!gtshame`, and nominating it again)?

Answer: Yes,  but this is where I see the problem. We have to go based upon the spelling. and what  you are bringing up is already the case. right now `Luucy Liu` would make a completely different person.
    1. damn, there is probably another bug ticket living in here

## Q6 · answered
The new spelling already names a different Person. Refuse, or merge the two?

Answer: giving one person another persons name is probably not the accept able

## Q7 · answered
Does a fix search for photos again under the new name?

Answer: `!gtfix` and `!!gtfix`

## Q8 · answered
The command. Proposed `!gtfix <number> <correct name>`, single `!` because no AI runs.

Answer: Agreed

## Owner input after round one (verbatim)

### Unacceptable
we need a super fast, capable, uuid that is used as the id for a person, not their name. what happens when two people have the same name? Doing it any other way is crazy. Lookup does not have to change and we do not need to display the uuid to the user. But being locked into a name because our database is a YOU problem not the USERS problem

Later: "i may have over stated the uuid thing"

### Thought
 I was halfway through the answers you gave me and I thought: "If there was an election to change the name of candidates, and we all voted on it, that would be fine." that should be determined by the owner only in that case it would be me for the channel that I am using. So, it would be:
- input (repeatable)
  - the command to fix 
  - the number
  - the correct name.
- message
  - Asks you to agree or disagree
  - Gives a time limit to respond with a no before it goes into effect 
- rules
  - the rules that you were talking about in your questions, some still have to be accounted for. 
- thoughts
  - what do you tthink ?

Q5, Q7 and Q8 received no answer in round one.

## Round two

Asked of GPT 6.2 Sol (side consult, the owner's request): the approval flow, how a no is given, the time limit, reply texts, edge cases, and whether an AI judge adds anything. Q2 and the AI part of Q1 wait for it.

## Q9 · answered
A Person's identity. (a) Every Person gets a UUIDv7 that never changes and is never shown; existing Persons are migrated to one in the next numbered migration; a spelling is only a way to find a Person. (b) Keep identity as the folded name. Evidence for the cost of (b): nominations refer to Persons by identity and finalized Nominations cannot change `person_id` (trigger `keep_outcomes_update`, `internal/nominations/infra/sqlite/schema.go`), so a renominated Person's earlier history could not follow a new name.

Answer: agree

## Q10 · answered
Two different real people with the same name. Proposed: out of scope; a known spelling keeps naming the one Person it names today.

Answer: agree

## Q11 · answered
"One time only": once per Person ever, or once per Nomination (so a renominated Person could be fixed again)?

Answer: We don't have a renomination function today so why are we talking about it? Or do we have that? I  think we do. If we do then fine.

(Fact: renomination exists, `!gtrenominate`, `internal/nominations/delivery/chat/nominate.go`, and the recorded Renominate assertions.)

## Q12 · answered
"that should be determined by the owner only in that case it would be me": (a) the owner alone approves each fix; (b) the Members decide and the owner can also stop or force one; (c) the owner decides only whether fixes are allowed at all, in the config.

Answer: Sure

## GPT 6.2 Sol consult (advice, not a decision)

Recommendation: both other Members approve in chat; sending `!gtfix` is the proposer's yes; any Member can stop it. Approve and stop with `!gtfixyes <fix>` and `!gtfixno <fix>`, numbered per proposal so a late reply never approves a different proposal; never reuse `!!Y`/`!!N` or 👍/👎. Window: 24 hours or until the Nomination closes, whichever comes first; an unanswered proposal expires. A stopped, withdrawn or expired proposal does not spend the one fix. Recheck that the Nomination is open and the name is free when the fix applies. Keep AI out of approval: "silence is too weak for a one-shot change that can carry votes to a completely different name". Photos: a search that has not started uses the corrected name; the once-per-Nomination limit stays. Reads "determined by the owner only" as the owner choosing the approval rule; if it meant the owner decides each fix, use explicit owner approval in chat.

Owner, on the PersonID change: "there are many different rules that use the id though so be careful"
Owner, round three preface: "My first answers were confusing and I do not particularly know what I want so you will have to go based off of what you have learned thusfar and then give a definitive thing you are going off of at the end."

## Q13 · answered
Does any AI take part in a fix (a judge, a daily cap, a pass over all names)?

Answer: (no direct answer) The owner: "My first answers were confusing and I do not particularly know what I want so you will have to go based off of what you have learned thusfar and then give a definitive thing you are going off of at the end." Decided on that authority: no AI takes part in approving a fix.

## Q14 · answered
Owner: "so it stops all nominations until the fix is in? or does it stop the nomination for that person?" Proposed: neither. Voting, finalizing and nominating go on; only a second fix for the same Person waits.

Answer: Agree

## Q15 · answered
Owner: "what about a fix for a certified ?" Q4 said open Nominations only, before Q2 chose agreement by every Member. Proposed: any Person, certified or rejected too, with the same agreement and the same one fix per Person.

Answer: sure

## Q16 · answered
Owner: "Are any of these candidates for configs being used? there are numbers here". Today the config holds Group Settings: three nomination limits and `allow_renomination` (`internal/nominations/domain/settings.go`, `cmd/gentalent/config.go:37`); the one-minute result delay (`domain/finalize.go:9`) and the close-spelling thresholds (`delivery/chat/close_spelling.go:15`) are constants. Proposed: `allow_name_fixes` (switch, like `allow_renomination`) and `name_fix_hours` (24) become Group Settings; one fix per Person and agreement by every Member stay fixed rules.

Answer: Comments on the proposal, verbatim:
- On `name_fix_hours`: "what limit are you talking about?"
- On agreement by every Member staying fixed: "yeah, but if one person hasn't answered in 20 days, we just can't fix? Configurable.to a percentage. Also, right now we are acting like it is always going to be just 3 people. What if there is 48?"
- On `allow_name_fixes`: "I like this"
- On entry 8 (window): "reword given my other citations"
- On entry 9 (`name-fixed-only-when-group-allows`): "okay"

## Q17 · answered
The agreement percentage. Proposed: `name_fix_agreement_percent` in the config; a NameFix takes effect once that share of the roster agrees (the proposer counts); it stops once enough Members say no that the share can no longer be reached; it expires at the end of `name_fix_hours`. Default 100.

Answer: (no direct answer) The owner: "just implement the feature and do not worry about my answers.md or questions.md stuff. you already messed up and already have a complicated feature to implement". Taken as going with the recommendation: default 100.

## Q18 · answered
Group size. Fact: the config refuses any roster but exactly three (`cmd/gentalent/config.go:124`), and recorded entries say "three" (Member, votes-change-until-the-third-vote, finalized-only-when-all-votes-are-in). Proposed: name fixes count the roster, never three; making the Group any size is its own spec.

Answer: I think this should be put in the domain spec or something as so mething we are moving to. but we need to be able to do a fix today. that seems like a larger unrelated effot.

## Q19 · answered
Owner: "what is sol's 24 hour thing? do we even want it in there? i can't state it and  I'm the domain owner lol". It is how long a proposed fix waits for agreement before it lapses. Proposed: drop it, with `name_fix_hours` and entry 8. A fix waits until it has enough agreement, or enough Members say no, or its proposer withdraws it; the percentage (Q17) already answers "one person hasn't answered in 20 days".

Answer: oh i like tht

Owner, later on Q19 (reopens it): "wait i think i liked that one and then it was going to be configurable right?"
Owner, on entry 10 (`name-fix-agreement-percentage-within-range`): "cool"
Owner, on specs: "you shouldn't be saving my specs in git although you should be transferring them over there. the thing that people should really b e using is the domain. i can have embarrasing things in there"

## Q20 · declined
Specs out of git. Proposed: `.arclint/specs/` goes in `.gitignore`; the two tracked done specs are untracked on this branch (history keeps them unless the owner asks for a rewrite); "over there" read as the main checkout's `.arclint/specs/`, where this folder is copied as it changes state.

Owner reply (not yet an answer): "yo, they should move with me back to makin f rom the wortree". On untracking: "I want the specs that i work on to be on disc and at the place that i have them at. So, if i work on a spec here in the working tree, and i go to main, I expect to see it in the main branch. EVEN THOUGH it is not in the remote. I think I had it down correctly before a certain change. There are things that we should keep on the remte from there right? just not my dialogue. We have my final stuff in arclint's domain file. We can have your questions. We don't need my ramblings on remote? do we?"

Revised proposal: tracked, so they reach main and the remote: `questions.md` with questions only, `<name>.ears.md`, `<name>.feature`, `rules.arclint.yaml` and the domain copy. Untracked: the owner's words in `answers.md` beside them (ignored by `.arclint/specs/**/answers.md`); its one copy lives in the main checkout's spec folder, and the worktree's spec folder holds a symlink to it, so the same file shows in both places and moves with the folder's state. The two done specs' answers move out of their tracked `questions.md` the same way.

Owner reply on Q20 (not yet an answer): "so it is already doing this. answers is already not tracked so then we are fine"
Checked: `git grep -c "^Answer:" HEAD -- .arclint/specs` finds 11 answers in done/fuzzy-name-voting/questions.md and 37 in done/round-one/questions.md, on a commit origin/main contains; `git check-ignore` ignores neither questions.md nor answers.md in a spec folder (exit 1).

Owner: "just implement the feature and do not worry about my answers.md or questions.md stuff. you already messed up and already have a complicated feature to implement"
Taken as approval of entries 1 to 8, 11 and 12 in the wording last shown (9 and 10 approved earlier), announced to the owner before recording. The spec folder stays untracked.

## Owner, after the first commits (verbatim)
- On the withdrawal rule: "the member that proposes a name fix can withdraw it with their own no"
- On the library's clock read: "i think a new version of go got uuid in the standard library"
- On "the Group has no list of Persons. Each Nomination holds its own Person{ID, Name} value": "this is just wrong"
- On asking ArcLint for a rule: "you fool. arclint allows you to write your own rules"
- On writing the convention in AGENTS.md: "what the hell. write the rulefor it before you refactor! what the hell"
- "record exactly what I said."
- "Where is your repositories?"
