# fuzzy-name-voting: requirements

Each line names the domain key it refers to. The rule text lives only in the domain recording.

- **R1** While more than one Nomination is open, when a Member votes with a close spelling of one open Nomination's Person's name, GenTalent shall count the vote on that Nomination and reply "I think you meant <name> (#<number>)." above the vote line.
  Domain: nominations / Group / assertion / close-spelling-names-only-an-open-nomination
- **R2** When a Member votes with a close spelling that matches only a Person without an open Nomination, GenTalent shall not count it on any Nomination.
  Domain: nominations / Group / assertion / close-spelling-names-only-an-open-nomination
- **R3** When a Member votes with a close spelling that matches more than one open Nomination, GenTalent shall count nothing and list the open Nominations.
  Domain: nominations / Group / assertion / close-spelling-names-one-nomination
- **R4** When a Member votes with a number, GenTalent shall name the Nomination by that number only, and never by a close spelling.
  Domain: nominations / Group / assertion / vote-names-its-nomination
