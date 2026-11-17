Feature: round-one

  @nominations_Group_open-nominations-within-group-limit
  Scenario: A nomination beyond the Group's open nomination limit is refused
    Given the Group's open nomination limit is 2
    And Nominations for "Ada Lovelace" and "Grace Hopper" are open
    When Alex sends "!gt Katherine Johnson. Orbital math"
    Then no Nomination is opened for "Katherine Johnson"
    And the reply lists the open Nominations and how to finish them

  @nominations_Group_open-nominations-within-group-limit
  Scenario: Lowering the limit below the open count refuses only new nominations
    Given Nominations for "Ada Lovelace" and "Grace Hopper" are open
    When the Group's open nomination limit becomes 1
    Then "!gt Katherine Johnson. Orbital math" is refused
    And "!!Y 1" still counts as Alex's vote on Nomination 1

  @nominations_Group_open-nominations-within-member-limit
  Scenario: A Member at the member open nomination limit cannot nominate again
    Given the member open nomination limit is 1
    And Alex's Nomination for "Ada Lovelace" is open
    When Alex sends "!gt Grace Hopper. Compilers"
    Then no Nomination is opened for "Grace Hopper"
    And Blair can still nominate "Grace Hopper"

  @nominations_Group_nominations-per-message-within-limit
  Scenario: A message over the single message nomination limit nominates nobody
    Given the single message nomination limit is 2
    When Alex sends "!gt Ada Lovelace. Engines !gt Grace Hopper. Compilers !gt Katherine Johnson. Orbits"
    Then no Nomination is opened
    And the reply says at most 2 people can be nominated in one message

  @nominations_Group_vote-names-its-nomination
  Scenario: An unnamed vote with several open Nominations counts nothing
    Given Nominations 1 "Ada Lovelace" and 2 "Grace Hopper" are open
    When Blair sends "!!Y"
    Then no vote is recorded
    And the reply lists Nomination 1 "Ada Lovelace" and Nomination 2 "Grace Hopper"

  @nominations_Group_vote-names-its-nomination
  Scenario: With one open Nomination a vote needs no name
    Given only Nomination 1 "Ada Lovelace" is open
    When Blair sends "!!Y"
    Then Blair's yes is recorded on Nomination 1

  @nominations_Group_one-open-nomination-per-person
  Scenario: A Person with an open Nomination cannot be nominated again
    Given Nomination 1 "Ada Lovelace" is open
    When Blair sends "!gt Ada Lovelace. Engines"
    Then no Nomination is opened
    And the reply points to Nomination 1

  @nominations_Group_renominated-only-through-renomination
  Scenario: Nominating a previously rejected Person is refused
    Given "Ada Lovelace" was rejected on Nomination 1
    When Blair sends "!gt Ada Lovelace. Engines"
    Then no Nomination is opened

  @nominations_Group_renominated-only-when-rejected
  Scenario: A certified Person cannot be renominated
    Given "Grace Hopper" was certified on Nomination 2
    And the Group allows renomination
    When Blair renominates "Grace Hopper"
    Then no Renomination is opened

  @nominations_Group_renominated-only-when-group-allows
  Scenario: Renomination is refused while the Group does not allow it
    Given "Ada Lovelace" was rejected on Nomination 1
    And the Group does not allow renomination
    When Blair renominates "Ada Lovelace"
    Then no Renomination is opened

  @nominations_Group_nomination-limit-not-less-than-one
  Scenario: A group open nomination limit of zero is refused at startup
    Given the group open nomination limit is configured as 0
    Then GenTalent does not start and names that setting
