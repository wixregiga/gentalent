# gt-who questions

Base: b0e94ae (last commit of domain.arclint.yaml)
Worktree base: 3c44a7b (local main, the commit the live bot runs)
Source: the owner's request, dictated, verbatim:

> Hey, so I want a feature where it is a GT who? Um, so it'll be GT, who? And it'll give a short blurb about, like, somebody that I'm thinking of so. Your job is to come back with a GT command for that person or to come back with that person that's already inside. Of the application. So, if I say. Who is that girl from Monster Ball and it's Halle Berry, but Halle Berry is already inside of the? Uh nominations, or inside of. She's already a person in there. That has already been reviewed. Say that she's in there. If she's not, we give a little blur about her life or his life. Whatever person you come back with and I get to decide which one I want. And then. I get to make that person a nominee if I want to make that person nominee. I also get to say no to that person as well if I don't.

Facts found (not decisions):

- A Person is "someone who has been nominated at least once by a Member" (`domain.arclint.yaml`, nominations / Group / Person). The Group holds each Person once with a PersonID (UUIDv7), a Name and earlier Spellings (`internal/nominations/domain/person.go:15`).
- Nominating resolves a Person by exact Spelling only (`Group.people`, `internal/nominations/domain/nominate.go`); close spelling is used only for votes on open Nominations (`internal/nominations/delivery/chat/close_spelling.go:21`, assertions `close-spelling-names-only-an-open-nomination`, `close-spelling-names-one-nomination`). So a misspelled `!gt` makes a second Person: the known open bug.
- `!!` marks commands that run AI (`README.md`, `!!gt`; `doc/plans/next-features.md`, `!gtshame` "no AI, so no `!!`").
- AI runs beside the ordered lane, never holding the database: claimed after the command commits, run by `cmd/gentalent/photo_searches.go`, result handed back to the lane as a command (`doc/architecture.md`, step 6). The `ai` context today only finds a Person's photos (`domain.arclint.yaml`, context `ai` and its relation to `nominations`). Programs are the `codex`, `claude`, `agy` CLIs on subscription logins (`internal/ai/infra/programs/program.go`), with a 180 s timeout per provider.
- `!!gtinfo` is planned, not built: facts about a nominee, each with a source link or dropped; "no reliable public info" for a name that is not clearly a public figure; three open questions (AI failure, pitch-less `!gt`, ethnicity) (`doc/plans/next-features.md:58`).

## Round one

## Q1 · answered
The command. Proposed `!!gtwho <description>` (two `!` because AI runs), e.g. `!!gtwho that actress from Monster's Ball`. Any Member may send it.

Answer: Any

## Q2 · answered
How many people come back? (a) one best guess; (b) up to three numbered guesses, best first, so you "decide which one I want"; (c) a different number.

Answer: Yes

## Q3 · answered
"Already inside of the application": which Persons count? (a) every Person, whatever their status (open, certified, rejected), and the reply says where they stand and their Nomination number; (b) only those already finalized ("already been reviewed"); an open one gets something else.

Answer: Yes. Of course, if that person's not in there then they're not in there. I wanted to search for the right person first and then check to see if that person's in there.

## Q4 · answered
How a guess is matched to a Person already in the Group. (a) the guessed name is one of a Person's Spellings (exact or an earlier spelling after a fix), the same rule `!gt` uses today; (b) also a close spelling, shown as "may already be in: Hallie Berry (#4)" without deciding. The known bug (a misspelled `!gt` makes a second Person) stays its own spec either way.

Answer: I don't know what you're talking about here. Just keep in mind the overall goal. I don't know something and I want to find it out. So if I'm like who's that white girl who is inside of scary movie she played alongside it. Regina Hall. I want them to come back with the right person. That's not making more complicated than that.

## Q5 · answered
Does the blurb follow the planned `!!gtinfo` rules? Proposed: yes, for both: only clear public figures; each blurb carries a source link or is dropped; a description that fits no clear public figure gets "no reliable public match" instead of a guess. `!!gtinfo` itself stays unbuilt.

Answer: Sure.

## Q6 · answered
How "make that person a nominee" or "say no" works. (a) The reply carries a ready command per guess (`!gt Halle Berry.`); sending it nominates through the existing `!gt` path with all its rules, and not sending it is saying no; nothing is stored. (b) The bot keeps the guesses; a Member answers `!gtwho yes 2` or `!gtwho no`, and yes nominates through the `!gt` rules with that Member as nominator.

Answer: B. But it should have the effect of a gt call of it's a yes and the person isn't in

## Q7 · answered
An AI lookup fails (every program errors or times out). Proposed: reply "I couldn't work out who that is. Try again with more detail." and record nothing; no automatic retry.

Answer: Agree

Owner, after round one: "Let's not over complicate this so I don't really see the need for many more rounds of this"

Q4 decided on that authority: a guess is checked against the Group the way `!gt` checks a name (a Person's Spellings); no close-spelling hint.

## Round two (final, the owner asked for few rounds)

## Q8 · answered
Answer form. Each WhoSearch has a number shown in its reply. `!gtwho 7 yes 2` nominates guess 2 like `!gt`; `!!gtwho 7 yes 2` like `!!gt` (with photos); text after the guess number is the pitch; `!gtwho 7 no` says no. The number is always required, so a yes never lands on someone else's search.

Answer: I don't see the point in the number being necessary for the yes or no side because if a person is only allowed to have one who guess out there that is not answered yet then the pics inside of that who search or their answer. So why do I have to now mention the who search ID number in order to pick? Especially if there's one answer that comes back, why would I have to pick the number first instead of just saying yes without even having a number?

## Q9 · answered
Unanswered or already-in guesses. A WhoSearch waits for an answer with no time limit, and any Member answers it. A guess already in the Group is shown as "already in: certified, #4" and gets no yes command.

Answer: A search should be answerable or not answerable because imagine if I'm searching for Halle Berry and she's already in there? I don't want to make them respond to that.

Every member should have only five who searches per day. And, they should be either just results returned back or answerable. Because some of them are just not going to have an action that should be taken because maybe they're already rejected or maybe they're already in there already nominated

## Q10 · declined
Approve the domain entries as written (entity WhoSearch, six Group assertions, the ai context definition and its relation).

Owner gave no answer to Q8 or Q10 in this reply.

Superseded by Q12, Q13, Q14, Q16 and Q19; never answered on its own.

## Round three

## Q11 · answered
"Five who searches per day": (a) any 24 hours, rolling; (b) the calendar day in one time zone. Does a search whose AI failed count? Proposed: (a); it counts; a search refused by the limit costs nothing. Five is a fixed rule, not a config setting.

Answer: Yes

## Q12 · declined
Approve Q8 as written and Q10 revised: the WhoSearch definition ends at "a source link" (the answerable rule is an assertion, not definition text); new assertions `answerable-only-with-a-guess-not-in-the-group` (on AnswerWhoSearch) and `five-who-searches-per-member-per-day` (on SearchWho).

Superseded by Q13 and later rounds; never answered on its own.

## Q13 · answered
Read from the Q8 answer: a Member has at most one unanswered WhoSearch, and only that Member answers it. `!gtwho yes`, `!gtwho yes 2 <pitch>`, `!!gtwho yes 2` (photos), `!gtwho no`; no search number; the guess number only when more than one guess can be picked. A new `!!gtwho` while one is still running or unanswered is refused with how to answer it. A results-only search never blocks.

Answer (profanity removed at the owner's request): "A member has at most one unanswered whose search" needs to protect against another who search being launched for that member without the first one being answered. But again, it's you putting these numbers in my rules all the time where they should be configurable.

Don't put in numbers in my rules. It's a config. The default should be five but don't say five who's that's so stupid. I hate that. Don't record my profanities either.

I don't think that's the source link. Is something that needs to be returned in the text message. It's more like metadata right? I want this search to be very quick and snappy. I'm not trying to spin my wheels or spend a lot of tokens per search here.

Oh yes names. It's guess no because the guess isn't the issue. It's the person inside of the guess so if a guest has three people then it can name one of those three peoples as the yes. If a guest only has one person in it, then of course it's only yes that's needed without a number. You need to make sure that the sum of these rules that you think I'm saying actually makes sense too.

Yes, nominates is GT is supposed to be an AI GT not regular GT. So that means it's a photo search as well. The only time that it's not going to get to AIS if it's already in there meaning that there's no yes to answer to. But if it's already in there and it's a part of a list and they say yes to it, then it shouldn't go to AI meaning that if they say yes to a person that is already either nominated rejected or certified and that person was part of a list that included a non-certified or nominated or rejected person. Then then person would not kick off an AI GT flow

## Round four

## Q14 · answered
Revised whole: limits are config (`who_search_daily_limit` 5, `who_search_guess_limit` 3), never numbers in rules; the source link is kept as metadata, not shown; one fast AI call per search; a yes is a `!!gt` (with photos) for a public figure not in the Group, and nominates no one for a Person already in it; the yes names one public figure whenever the guess holds more than one. Approve the 13 entries as shown in round four.

Answer: You know what? Let's make this even easier. Only return with one person. You don't get multiple guesses. You only get one person back from a GT who

## Q15 · declined
A yes that the nomination rules refuse (for example the Group's open nomination limit is full). Proposed: the search stays unanswered, so the Member can say yes later or say no.

## Q11 (restated) · answered
"A day": rolling 24 hours (proposed) or the calendar day. A failed search counts (proposed).

Answer: Yes (the owner's "11. Yes", saved under the first Q11 above)

## Round five

## Q16 · answered
Revised for one public figure per WhoSearch: the guess limit, its config and the guess-number rule go; `!!gtwho yes [pitch]` and `!!gtwho no`; a search whose public figure is already a Person is results only. Approve the 11 entries as shown in round five.

Answer: Should we even have it come back with something where you can say yes or no to? We already have GT. We already have the amount limit per person. How about we just do that? We don't actually provide a yes or a no. So it just fires off. The search brings back the answer and that's it. No action upon it. The answer needs to come back with at least one photo of the person. Not the three photos. Just one photo

Q15 no longer applies: there is no yes (Q16).

## Round six

## Q17 · answered
"We already have the amount limit per person": (a) keep the who search daily limit (config, default 5) on `!!gtwho`; (b) no limit of its own.

Answer: Yes

## Q18 · answered
The AI finds the person but no photo. (a) Send the blurb and say no photo was found; (b) treat it as a failed search.

Answer: A

## Q19 · answered
Approve the entries as shown in round six (WhoSearch, who-search-daily-limit-not-less-than-one, who-searches-within-daily-limit, who-searched-at-most-once, public-figure-has-a-source, public-figure-spelled-as-a-person-is-that-person) and the ai context and relation wording.

Answer: (profanity removed at the owner's request)
- You don't need that first who search daily limit being less than one thing. That's stupid. Just set the limit. Who search per member is not greater than the daily limit. If it's 0 then it's 0.
- Why are we saying the AI has to run at least once?
- just try to grab the source link. I don't think this is hard.

Taken from Q19 as approval of the remaining entries with the owner's corrections, announced to the owner before recording:
- `who-search-daily-limit-not-less-than-one` dropped; a limit of 0 allows no who searches.
- `who-searches-within-daily-limit` restated in the owner's words.
- `who-searched-at-most-once` dropped as a domain rule (librarian `not-a-domain-rule`: a promise about delivery); the engine still claims each search once, as for photos, verified by a test.
- `public-figure-has-a-source` dropped; the source link is kept when the AI gives one.
