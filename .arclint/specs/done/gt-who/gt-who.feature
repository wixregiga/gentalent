Feature: gt-who

  @nominations_Group_who-searches-within-daily-limit
  Scenario: A Member at the who search daily limit is refused
    Given the Group's who search daily limit is 2
    And Ada started 2 who searches in the last 24 hours
    When Ada sends "!!gtwho the woman who wrote the first computer program"
    Then no WhoSearch starts and Ada is told when the next one is allowed

  @nominations_Group_who-searches-within-daily-limit
  Scenario: A who search daily limit of 0 allows none
    Given the Group's who search daily limit is 0
    When Ada sends "!!gtwho the woman who wrote the first computer program"
    Then no WhoSearch starts

  @nominations_Group_public-figure-spelled-as-a-person-is-that-person
  Scenario: The public figure is already a Person
    Given Ada Lovelace was certified in Nomination 4
    When a WhoSearch's AI answer names "Ada Lovelace"
    Then the reply says she is already in, certified, Nomination 4

  @nominations_Group_WhoSearch
  Scenario: The public figure is not in the Group yet
    When a WhoSearch's AI answer names "Grace Hopper" with a blurb, a photo and a source link
    Then the reply shows her name, the blurb and the photo, says she is not in yet, and shows no source link
