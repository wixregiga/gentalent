# multiple-gentalent-groups: requirements

Each line names the domain key it refers to. The rule text lives only in the domain recording.

- When a Group is created or changed, GenTalent shall enforce its Member roster requirement. Domain: nominations / Group / invariant / group-has-at-least-two-members
- While an open Nomination waits for votes, GenTalent shall allow voting, changing, or clearing votes according to the lock rule. Domain: nominations / Group / assertion / votes-change-until-the-last-vote
- When a nominator sends `!final`, GenTalent shall enforce early finalization readiness. Domain: nominations / Group / assertion / finalized-only-when-all-votes-are-in
- For any Group roster of two or more people, GenTalent shall model each participant as a Member. Domain: nominations / Group / entity / Member
