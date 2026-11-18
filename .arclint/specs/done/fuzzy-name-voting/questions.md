# fuzzy-name-voting questions

Base: cf6d499 (last commit of domain.arclint.yaml)
Source: the owner's request in this thread (Fuzzy name voting, items 1.1 to 1.5.4)
Evidence: live message 1326 `!!Y Sofia ` was refused with "Several Nominations are open. Name one, e.g. !!Y 6" while Nominations 6 (Sofía Vergara) and 7 (Hachikō) were open; the Member then sent `!!Y 6` (1328), which counted. Read from a backup-API copy of nominations.sqlite3 and a read-only GET of the chat's messages.

Settled in the request (owner's words):

- "When voting has more than one person being nominated then a sufficiently close fuzzy search should be enough to match the intended person."
- "No ai usage for fuzzy search"
- "Fuzzy search voting must be limited to the people being voted upon"
- "If the vote for 1 is done, then the workflow for sending that should still trigger and it shouldn't get to fuzzy search."
- Examples: "Sofía Vergara": "Sofia", "sofria", "sofia vergera", "vergara"; "Ada Lovelace": "love", "lovelace", "ada", "addaa"
- "If fuzzy search was used, include something like `I think you meant <nominee>` when announcing the update."

## Q1 · answered
Item 1.5 says a wrong number must not be a reason to fuzzy search; item 1.5.2 says "If the vote number is wrong then it go to fuzzy search". Which holds for `!!Y 9` when no Nomination 9 is open?

Answer: Agree

## Q2 · answered
Last resort or ranked? Proposed order: a number first, then an exact name, then a close spelling among the open Nominations only when the first two name nothing.

Answer: Agree

## Q3 · answered
Two open Nominations equally close (`!!Y ada` with Ada Lovelace and Adam Driver open). Proposed: refuse with the existing "Several Nominations are open" list; never pick one.

Answer: Agree

## Q4 · answered
Words after a vote that were not meant as a name (`!!Y love it` while Ada Lovelace is open) would count for Ada Lovelace. Proposed: accept it; the "I think you meant" line shows what counted and `!!C` or a new vote fixes it.

Answer: Agree? Up to you, depending on what you think is just right.. No need to ask me again.

## Q5 · answered
How close is close. Proposed, ignoring case and accents: a typed word of 3 or more letters that begins any word of the nominee's name, or typed words with similarity of at least 0.75 to one word of the name or to the whole name.

Answer: Agree. I do not know or care too much. You can decide. Ask fable 5.2 if you need quick advice if needed. If not, no need to waste my tokens then. Thank you!

## Q6 · answered
Names that are not open. Proposed: an exact name of a closed or rejected Person keeps today's "no open Nomination" refusal, and a near miss of one keeps today's "Did you mean" refusal when no open name is close.

Answer: Agree

## Q7 · answered
Scope. Proposed: votes only (`!!Y`, `!!N`, `!!C`); `!final <name>`, `!gtshame @Name` and reactions unchanged.

Answer: Agree

## Q8 · answered
Reply wording. Proposed: a first line "I think you meant Sofía Vergara (#6)." above the usual vote line.

Answer: Agree

## Q9 · answered
Approve assertion `nominations / Group / assertion / close-spelling-names-only-an-open-nomination`, on CastVote: "A vote that spells a Person's name closely, not exactly, counts only on an open Nomination."

Answer: Sure. 
   1. future or now: something about a threshold if you want or if that makes sense (do not ask me about it again and if implemented then make the threshold be configurable. I don't think you have to do this)

## Q10 · answered
Approve assertion `nominations / Group / assertion / close-spelling-names-one-nomination`, on CastVote: "A close spelling that fits more than one open Nomination names none of them."

Answer: Sure
    1. Again, if you want to have a threshold if  fable 5.2 thinks there's an empirical way of doing this then by all means do it. i just want it to work.

## Q11 · declined
Approve the nominations context definition, adding one sentence: "The nomination process: Members nominate a Person, the Group votes on each Nomination, and finalizing it certifies or rejects the Person as a Generational Talent. A vote names its Nomination by number, by its Person's name, or by a close spelling of that name."

Answer: No. It stays how it's defined currently because the functionality hasn't changed.
