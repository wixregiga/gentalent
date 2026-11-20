Feature: multiple-gentalent-groups

  @nominations_Group_group-has-at-least-two-members
  Scenario: A Group with fewer than two distinct Members is refused
    When a Group is created with zero Members, one Member, duplicate MemberIDs, or a blank MemberID
    Then Group creation fails

  @nominations_Group_votes-change-until-the-last-vote
  Scenario: A four-Member Group locks only once the fourth vote is in
    Given a Group with four Members and one open Nomination
    When the third Member votes
    Then Members may still change or clear their votes
    When the fourth Member votes
    Then further vote changes or clears are refused

  @nominations_Group_finalized-only-when-all-votes-are-in
  Scenario: Early finalize requires every Member's vote in a four-Member Group
    Given a Group with four Members and an open Nomination with three votes in
    When the nominator runs Finalize
    Then Finalize is refused until the fourth Member votes
