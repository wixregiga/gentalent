# round-one: requirements

Each line names the domain key it refers to. The rule text lives only in the domain recording.

- **R1** When a Member nominates a person while the Group has no room under its open nomination limit, GenTalent shall refuse the nomination and list the open Nominations with how to finish them.
  Domain: nominations / Group / assertion / open-nominations-within-group-limit
- **R2** When a Member nominates a person while that Member has no room under the member open nomination limit, GenTalent shall refuse the nomination and list that Member's open Nominations.
  Domain: nominations / Group / assertion / open-nominations-within-member-limit
- **R3** When one message nominates more people than the single message nomination limit, or than the Group or Member limits have room for, GenTalent shall nominate none of them and reply with how many can be nominated.
  Domain: nominations / Group / assertion / nominations-per-message-within-limit
- **R4** When a Member votes without naming a Nomination while more than one Nomination is open, GenTalent shall count no vote from that message and list the open Nominations by number and name.
  Domain: nominations / Group / assertion / vote-names-its-nomination
- **R5** When a Member nominates a Person who already has an open Nomination, GenTalent shall refuse it and point to that open Nomination by number.
  Domain: nominations / Group / invariant / one-open-nomination-per-person
- **R6** When a Member nominates a Person who was nominated before, GenTalent shall refuse it and say how to renominate when the Group allows renomination.
  Domain: nominations / Group / assertion / renominated-only-through-renomination
- **R7** When a Member renominates a Person whose certification status is not rejected, GenTalent shall refuse it.
  Domain: nominations / Group / assertion / renominated-only-when-rejected
- **R8** When a Member renominates a Person while the Group does not allow renomination, GenTalent shall refuse it.
  Domain: nominations / Group / assertion / renominated-only-when-group-allows
- **R9** When a nomination limit below one is configured, GenTalent shall refuse to start and name the setting.
  Domain: nominations / Group / invariant / nomination-limit-not-less-than-one
