package domain_test

import (
	"testing"
	"time"

	"gentalent/internal/nominations/domain"
)

var oneAtATime = domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}

func vote(t *testing.T, group *domain.Group, member member, choice domain.Vote) []domain.VoteOutcome {
	t.Helper()
	outcomes, err := group.CastVote(member.ID, []domain.Ballot{{Vote: choice}}, today)
	if err != nil {
		t.Fatalf("%s votes %q: %v", member.Name, choice, err)
	}
	return outcomes
}

func TestVotesChangeUntilTheLastVote(t *testing.T) {
	group := newGroup(t, oneAtATime, open(1, ada, alex))

	if got := vote(t, group, blair, domain.No); len(got) != 1 || got[0].Change != domain.VoteRecorded {
		t.Fatalf("B61 a new no is recorded, got %+v", got)
	}
	if got := vote(t, group, blair, domain.Yes); len(got) != 1 || got[0].Change != domain.VoteChanged {
		t.Fatalf("B62 a changed vote is reported as changed, got %+v", got)
	}
	if got := vote(t, group, alex, domain.NotVoted); len(got) != 1 || got[0].Change != domain.VoteCleared {
		t.Fatalf("B63 B64 the nominator may clear their own yes, got %+v", got)
	}
	if votes := nomination(t, group, 1).Votes; len(votes) != 1 {
		t.Fatalf("B63 a clear removes the vote, got %+v", votes)
	}
	vote(t, group, alex, domain.Yes)
	locking := vote(t, group, casey, domain.No)
	if len(locking) != 1 || !locking[0].Locks || nomination(t, group, 1).LockedAt != today {
		t.Fatalf("B65 the last vote locks the Nomination at its time, got %+v", locking)
	}

	before := snapshot(group)
	for _, choice := range []domain.Vote{domain.No, domain.NotVoted} {
		_, err := group.CastVote(blair.ID, []domain.Ballot{{Vote: choice}}, today)
		refused := refusal(t, err, domain.VotesChangeUntilTheLastVote)
		wantNumbers(t, refused.Numbers, 1)
	}
	if got := vote(t, group, blair, domain.Yes); len(got) != 0 {
		t.Fatalf("B58 repeating your vote after the lock is silent, got %+v", got)
	}
	wantUnchanged(t, group, before)

	four, err := domain.NewGroup("group-four", []domain.MemberID{alex.ID, blair.ID, casey.ID, devon.ID}, oneAtATime, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("four-member group: %v", err)
	}
	if _, err := four.Nominate(alex.ID, candidates(ada), today); err != nil {
		t.Fatal(err)
	}
	vote(t, four, blair, domain.Yes)
	if third := vote(t, four, casey, domain.Yes); len(third) != 1 || third[0].Locks || !nomination(t, four, 1).LockedAt.IsZero() {
		t.Fatalf("the third vote in a four-Member Group must not lock, got %+v", third)
	}
	if changed := vote(t, four, blair, domain.No); len(changed) != 1 || changed[0].Change != domain.VoteChanged {
		t.Fatalf("before the fourth vote, a Member may change their vote, got %+v", changed)
	}
	if cleared := vote(t, four, casey, domain.NotVoted); len(cleared) != 1 || cleared[0].Change != domain.VoteCleared {
		t.Fatalf("before the fourth vote, a Member may clear their vote, got %+v", cleared)
	}
	vote(t, four, casey, domain.Yes)
	if fourth := vote(t, four, devon, domain.Yes); len(fourth) != 1 || !fourth[0].Locks || nomination(t, four, 1).LockedAt != today {
		t.Fatalf("the fourth vote locks the four-Member Group, got %+v", fourth)
	}
	_, err = four.CastVote(blair.ID, []domain.Ballot{{Vote: domain.Yes}}, today)
	refusal(t, err, domain.VotesChangeUntilTheLastVote)

	two, err := domain.NewGroup("group-two", []domain.MemberID{alex.ID, blair.ID}, oneAtATime, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("two-member group: %v", err)
	}
	if _, err := two.Nominate(alex.ID, candidates(ada), today); err != nil {
		t.Fatal(err)
	}
	if second := vote(t, two, blair, domain.Yes); len(second) != 1 || !second[0].Locks {
		t.Fatalf("the second vote locks the two-Member Group, got %+v", second)
	}
	_, err = two.CastVote(blair.ID, []domain.Ballot{{Vote: domain.No}}, today)
	refusal(t, err, domain.VotesChangeUntilTheLastVote)
}

func TestRepeatedVotesAndEmptyClearsAreSilent(t *testing.T) {
	group := newGroup(t, oneAtATime, open(1, ada, alex))
	if got := vote(t, group, alex, domain.Yes); len(got) != 0 {
		t.Fatalf("B58 repeating your current vote is silent, got %+v", got)
	}
	if got := vote(t, group, blair, domain.NotVoted); len(got) != 0 {
		t.Fatalf("B57 clearing no vote is silent, got %+v", got)
	}
	empty := newGroup(t, oneAtATime, finalized(1, ada, domain.Certified))
	if got := vote(t, empty, blair, domain.Yes); len(got) != 0 {
		t.Fatalf("B56 an unnamed vote with nothing open is silent, got %+v", got)
	}
}

func TestVotesNamingAClosedOrUnknownNominationCountNothing(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 2, MemberOpenNominationLimit: 2, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour},
		finalized(1, grace, domain.Certified), open(2, ada, alex))
	before := snapshot(group)
	cases := []struct {
		ref  domain.Ref
		rule domain.Rule
	}{
		{domain.Ref{Number: 1}, domain.NominationClosed},
		{domain.Ref{Number: 7}, domain.UnknownNomination},
		{domain.Ref{Person: grace.ID}, domain.NoOpenNomination},
	}
	for _, c := range cases {
		_, err := group.CastVote(blair.ID, []domain.Ballot{{Nomination: domain.Ref{Number: 2}, Vote: domain.Yes}, {Nomination: c.ref, Vote: domain.Yes}}, today)
		refusal(t, err, c.rule)
		wantUnchanged(t, group, before)
	}
}

func TestCountReactions(t *testing.T) {
	group := newGroup(t, oneAtATime, open(1, ada, alex))
	count := func(reactions map[domain.MemberID]domain.Vote) []domain.VoteOutcome {
		t.Helper()
		outcomes, err := group.CountReactions(1, reactions, today)
		if err != nil {
			t.Fatalf("CountReactions: %v", err)
		}
		return outcomes
	}

	if got := count(map[domain.MemberID]domain.Vote{blair.ID: domain.Yes}); len(got) != 1 || nomination(t, group, 1).Votes[blair.ID] != domain.Yes {
		t.Fatalf("B70 a new thumbs up votes yes, got %+v", got)
	}
	if got := count(map[domain.MemberID]domain.Vote{blair.ID: domain.Yes}); len(got) != 0 {
		t.Fatalf("B69 an unchanged reaction is never applied twice, got %+v", got)
	}
	if got := count(map[domain.MemberID]domain.Vote{blair.ID: domain.No}); len(got) != 1 || got[0].Change != domain.VoteChanged {
		t.Fatalf("B70 switching the reaction changes the vote, got %+v", got)
	}
	count(map[domain.MemberID]domain.Vote{})
	if _, voted := nomination(t, group, 1).Votes[blair.ID]; voted {
		t.Fatal("B71 removing the reaction clears the vote it cast")
	}
	if got := count(map[domain.MemberID]domain.Vote{"stranger": domain.Yes}); len(got) != 0 {
		t.Fatalf("B67 only Members' reactions count, got %+v", got)
	}
}

func TestRemovedReactionClearsOnlyItsVote(t *testing.T) {
	group := newGroup(t, oneAtATime, open(1, ada, alex))
	if _, err := group.CountReactions(1, map[domain.MemberID]domain.Vote{blair.ID: domain.Yes}, today); err != nil {
		t.Fatal(err)
	}
	vote(t, group, blair, domain.No)
	if _, err := group.CountReactions(1, map[domain.MemberID]domain.Vote{}, today); err != nil {
		t.Fatal(err)
	}
	if got := nomination(t, group, 1).Votes[blair.ID]; got != domain.No {
		t.Fatalf("B71 a vote typed since the reaction stays, got %q", got)
	}
}

func TestReactionsAfterTheLockAreConsumedSilently(t *testing.T) {
	locked := open(1, ada, alex)
	locked.Votes = map[domain.MemberID]domain.Vote{alex.ID: domain.Yes, blair.ID: domain.Yes, casey.ID: domain.Yes}
	locked.LockedAt = today
	group := newGroup(t, oneAtATime, locked)

	outcomes, err := group.CountReactions(1, map[domain.MemberID]domain.Vote{blair.ID: domain.No}, today)
	if err != nil || len(outcomes) != 0 {
		t.Fatalf("B73 a refused reaction changes no vote and replies nothing, got %+v, %v", outcomes, err)
	}
	got := nomination(t, group, 1)
	if got.Votes[blair.ID] != domain.Yes || got.Reactions[blair.ID] != domain.No {
		t.Fatalf("B73 the reaction is still remembered so it is never retried, got %+v", got)
	}
}

func TestThirdVoteByReactionStartsTheTimer(t *testing.T) {
	two := open(1, ada, alex)
	two.Votes[blair.ID] = domain.Yes
	group := newGroup(t, oneAtATime, two)
	reactedAt := today.Add(5 * time.Second)

	outcomes, err := group.CountReactions(1, map[domain.MemberID]domain.Vote{casey.ID: domain.Yes}, reactedAt)
	if err != nil || len(outcomes) != 1 || !outcomes[0].Locks {
		t.Fatalf("B74 the third vote by reaction locks, got %+v, %v", outcomes, err)
	}
	if got := nomination(t, group, 1).LockedAt; got != reactedAt {
		t.Fatalf("B74 the timer starts at the reaction, got %v", got)
	}
}

func TestCloseSpellingNamesOnlyAnOpenNomination(t *testing.T) {
	sofia := domain.Person{ID: "sofíavergara", Name: "Sofía Vergara"}
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour},
		finalized(1, grace, domain.Rejected), open(2, sofia, alex), open(3, ada, alex))
	before := snapshot(group)

	_, err := group.CastVote(blair.ID, []domain.Ballot{{Nomination: domain.Ref{Close: []domain.PersonID{grace.ID}}, Vote: domain.Yes}}, today)
	refusal(t, err, domain.CloseSpellingNamesOnlyAnOpenNomination)
	wantUnchanged(t, group, before)

	outcomes, err := group.CastVote(blair.ID, []domain.Ballot{{Nomination: domain.Ref{Close: []domain.PersonID{grace.ID, sofia.ID}}, Vote: domain.Yes}}, today)
	if err != nil {
		t.Fatalf("a close spelling that fits one open Nomination must count: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Number != 2 || !outcomes[0].ByCloseSpelling {
		t.Fatalf("want one outcome on Nomination 2 marked as a close spelling, got %+v", outcomes)
	}
	if got := nomination(t, group, 2).Votes[blair.ID]; got != domain.Yes {
		t.Fatalf("want Blair's yes on Nomination 2, got %v", got)
	}
	if got := nomination(t, group, 3).Votes[blair.ID]; got != domain.NotVoted {
		t.Fatalf("a close spelling counts on its one Nomination only, got %v on Nomination 3", got)
	}

	exact, err := group.CastVote(casey.ID, []domain.Ballot{{Nomination: domain.Ref{Person: ada.ID}, Vote: domain.No}}, today)
	if err != nil || len(exact) != 1 || exact[0].ByCloseSpelling {
		t.Fatalf("an exact name is not a close spelling, got %+v, %v", exact, err)
	}
}

func TestCloseSpellingNamesOneNomination(t *testing.T) {
	adam := domain.Person{ID: "adamdriver", Name: "Adam Driver"}
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour},
		open(1, ada, alex), open(2, adam, alex))
	before := snapshot(group)

	_, err := group.CastVote(blair.ID, []domain.Ballot{{Nomination: domain.Ref{Close: []domain.PersonID{ada.ID, adam.ID}}, Vote: domain.Yes}}, today)
	refused := refusal(t, err, domain.CloseSpellingNamesOneNomination)
	wantNumbers(t, refused.Numbers, 1, 2)
	wantUnchanged(t, group, before)
}
