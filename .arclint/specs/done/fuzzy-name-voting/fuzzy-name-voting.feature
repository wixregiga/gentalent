Feature: fuzzy-name-voting

  @nominations_Group_close-spelling-names-only-an-open-nomination
  Scenario: A close spelling counts on the one open Nomination it fits
    Given Nominations 6 "Sofía Vergara" and 7 "Hachikō" are open
    When Blair sends "!!Y Sofia "
    Then Blair's yes counts on Nomination 6
    And the reply starts "I think you meant Sofía Vergara (#6)."

  @nominations_Group_close-spelling-names-only-an-open-nomination
  Scenario Outline: Close spellings of open names
    Given Nominations for "Sofía Vergara" and "Ada Lovelace" are open
    When Blair sends "!!Y <typed>"
    Then Blair's yes counts on the Nomination for "<nominee>"

    Examples:
      | typed         | nominee       |
      | sofria        | Sofía Vergara |
      | sofia vergera | Sofía Vergara |
      | vergara       | Sofía Vergara |
      | love          | Ada Lovelace  |
      | lovelace      | Ada Lovelace  |
      | ada           | Ada Lovelace  |
      | addaa         | Ada Lovelace  |

  @nominations_Group_close-spelling-names-only-an-open-nomination
  Scenario: A close spelling of a closed Person counts nowhere
    Given Nominations for "Sofía Vergara" and "Hachikō" are open
    And "Ada Lovelace" was rejected
    When Blair sends "!!Y addaa"
    Then no vote is counted

  @nominations_Group_close-spelling-names-one-nomination
  Scenario: A close spelling that fits two open Nominations names none
    Given Nominations for "Ada Lovelace" and "Adam Driver" are open
    When Blair sends "!!Y ada"
    Then no vote is counted
    And the reply lists the open Nominations

  @nominations_Group_vote-names-its-nomination
  Scenario: A number never becomes a close spelling
    Given Nominations 6 "Sofía Vergara" and 7 "Hachikō" are open and Nomination 1 is finished
    When Blair sends "!!Y 1"
    Then the reply says Nomination 1 is finished
