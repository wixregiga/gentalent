package domain_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"gentalent/internal/nominations/domain"
)

var (
	alex    = domain.Member{ID: "member-alex", Name: "Alex"}
	blair   = domain.Member{ID: "member-blair", Name: "Blair"}
	casey   = domain.Member{ID: "member-casey", Name: "Casey"}
	members = []domain.Member{alex, blair, casey}

	ada       = domain.Person{ID: "adalovelace", Name: "Ada Lovelace"}
	grace     = domain.Person{ID: "gracehopper", Name: "Grace Hopper"}
	katherine = domain.Person{ID: "katherinejohnson", Name: "Katherine Johnson"}
	dorothy   = domain.Person{ID: "dorothyvaughan", Name: "Dorothy Vaughan"}

	monday = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	today  = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
)

func newGroup(t *testing.T, settings domain.Settings, history ...domain.Nomination) *domain.Group {
	t.Helper()
	group, err := domain.NewGroup(domain.GroupSnapshot{ID: "group-test", Nominations: history}, members, settings)
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	return group
}

func open(number domain.NominationNumber, person domain.Person, nominator domain.Member) domain.Nomination {
	return domain.Nomination{
		Number:      number,
		Person:      person,
		Nominator:   nominator.ID,
		Status:      domain.Nominated,
		Votes:       map[domain.MemberID]domain.Vote{nominator.ID: domain.Yes},
		NominatedAt: monday,
	}
}

func finalized(number domain.NominationNumber, person domain.Person, status domain.CertificationStatus) domain.Nomination {
	last := domain.Yes
	if status == domain.Rejected {
		last = domain.No
	}
	return domain.Nomination{
		Number:      number,
		Person:      person,
		Nominator:   alex.ID,
		Status:      status,
		Votes:       map[domain.MemberID]domain.Vote{alex.ID: domain.Yes, blair.ID: domain.Yes, casey.ID: last},
		LockedAt:    monday.Add(time.Minute),
		NominatedAt: monday,
	}
}

func candidates(people ...domain.Person) []domain.Candidate {
	list := make([]domain.Candidate, 0, len(people))
	for _, person := range people {
		list = append(list, domain.Candidate{Person: person, Pitch: "A pitch for " + person.Name})
	}
	return list
}

func refusal(t *testing.T, err error, rule domain.Rule) domain.Refusal {
	t.Helper()
	var refused domain.Refusal
	if !errors.As(err, &refused) {
		t.Fatalf("want a refusal by %v, got %v", rule, err)
	}
	if refused.Rule != rule {
		t.Fatalf("want a refusal by %v, got one by %v", rule, refused.Rule)
	}
	return refused
}

func numbers(nominations []domain.Nomination) []domain.NominationNumber {
	list := make([]domain.NominationNumber, 0, len(nominations))
	for _, nomination := range nominations {
		list = append(list, nomination.Number)
	}
	return list
}

func wantNumbers(t *testing.T, got []domain.Nomination, want ...domain.NominationNumber) {
	t.Helper()
	if !slices.Equal(numbers(got), want) {
		t.Fatalf("want Nominations %v, got %v", want, numbers(got))
	}
}

func wantUnchanged(t *testing.T, group *domain.Group, before domain.GroupSnapshot) {
	t.Helper()
	if after := group.Snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatalf("a refused command changed the Group:\nbefore %+v\nafter  %+v", before, after)
	}
}

func nomination(t *testing.T, group *domain.Group, number domain.NominationNumber) domain.Nomination {
	t.Helper()
	found, ok := group.Nomination(number)
	if !ok {
		t.Fatalf("Nomination %d does not exist", number)
	}
	return found
}

func TestNominationLimitNotLessThanOne(t *testing.T) {
	valid := domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 1}
	if _, err := domain.NewGroup(domain.GroupSnapshot{ID: "group-test"}, members, valid); err != nil {
		t.Fatalf("limits of one must be accepted: %v", err)
	}
	cases := []struct {
		name   string
		change func(*domain.Settings)
		limit  domain.Limit
		value  int
	}{
		{"group open limit of zero", func(s *domain.Settings) { s.GroupOpenNominationLimit = 0 }, domain.GroupOpenNominationLimit, 0},
		{"member open limit below zero", func(s *domain.Settings) { s.MemberOpenNominationLimit = -1 }, domain.MemberOpenNominationLimit, -1},
		{"single message limit of zero", func(s *domain.Settings) { s.SingleMessageNominationLimit = 0 }, domain.SingleMessageNominationLimit, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			settings := valid
			c.change(&settings)
			group, err := domain.NewGroup(domain.GroupSnapshot{ID: "group-test", Nominations: []domain.Nomination{open(1, ada, alex)}}, members, settings)
			var below domain.LimitBelowOne
			if !errors.As(err, &below) {
				t.Fatalf("want LimitBelowOne, got group %v and error %v", group, err)
			}
			if below.Limit != c.limit || below.Value != c.value {
				t.Fatalf("want %v at %d named, got %v at %d", c.limit, c.value, below.Limit, below.Value)
			}
		})
	}
}

func TestOneOpenNominationPerPerson(t *testing.T) {
	settings := domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 3}
	group := newGroup(t, settings, open(1, ada, alex))
	before := group.Snapshot()

	_, err := group.Nominate(blair.ID, candidates(ada), today)
	refused := refusal(t, err, domain.OneOpenNominationPerPerson)
	wantNumbers(t, refused.Nominations, 1)
	wantUnchanged(t, group, before)

	_, err = group.Nominate(blair.ID, candidates(grace, grace), today)
	refused = refusal(t, err, domain.OneOpenNominationPerPerson)
	if refused.Ref != (domain.Ref{Person: grace.ID}) {
		t.Fatalf("a message naming Grace twice must name her, got %+v", refused.Ref)
	}
	wantUnchanged(t, group, before)

	_, err = domain.NewGroup(domain.GroupSnapshot{ID: "group-test", Nominations: []domain.Nomination{open(1, ada, alex), open(2, ada, blair)}}, members, settings)
	refused = refusal(t, err, domain.OneOpenNominationPerPerson)
	wantNumbers(t, refused.Nominations, 1, 2)
}

func TestOpenNominationsWithinGroupLimit(t *testing.T) {
	settings := domain.Settings{GroupOpenNominationLimit: 2, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 3, AllowRenomination: true}
	group := newGroup(t, settings, finalized(1, dorothy, domain.Rejected), open(2, ada, blair), open(3, grace, casey))
	before := group.Snapshot()

	_, err := group.Nominate(alex.ID, candidates(katherine), today)
	refused := refusal(t, err, domain.OpenNominationsWithinGroupLimit)
	wantNumbers(t, refused.Nominations, 2, 3)
	if refused.Limit != 2 {
		t.Fatalf("the refusal must carry the limit 2, got %d", refused.Limit)
	}
	wantUnchanged(t, group, before)

	_, err = group.Renominate(alex.ID, candidates(dorothy), today)
	refused = refusal(t, err, domain.OpenNominationsWithinGroupLimit)
	wantNumbers(t, refused.Nominations, 2, 3)
	wantUnchanged(t, group, before)

	settings.GroupOpenNominationLimit = 3
	group = newGroup(t, settings, finalized(1, dorothy, domain.Rejected), open(2, ada, blair), open(3, grace, casey))
	opened, err := group.Nominate(alex.ID, candidates(katherine), today)
	if err != nil {
		t.Fatalf("a Group below its limit must accept the nomination: %v", err)
	}
	if len(opened) != 1 || opened[0].Nomination.Person.ID != katherine.ID {
		t.Fatalf("want Katherine opened, got %+v", opened)
	}
}

func TestLoweringTheGroupLimitRefusesOnlyNewNominations(t *testing.T) {
	history := []domain.Nomination{open(1, ada, blair), open(2, grace, casey)}
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3}, history...)

	_, err := group.Nominate(alex.ID, candidates(katherine), today)
	refusal(t, err, domain.OpenNominationsWithinGroupLimit)

	if _, err := group.CastVote(alex.ID, []domain.Ballot{{Nomination: domain.Ref{Number: 1}, Vote: domain.Yes}}, today); err != nil {
		t.Fatalf("a vote must still count after the limit is lowered: %v", err)
	}
	if got := nomination(t, group, 1).Votes[alex.ID]; got != domain.Yes {
		t.Fatalf("want Alex's yes on Nomination 1, got %v", got)
	}
}

func TestOpenNominationsWithinMemberLimit(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3}, open(1, ada, alex))
	before := group.Snapshot()

	_, err := group.Nominate(alex.ID, candidates(grace), today)
	refused := refusal(t, err, domain.OpenNominationsWithinMemberLimit)
	wantNumbers(t, refused.Nominations, 1)
	if refused.Limit != 1 {
		t.Fatalf("the refusal must carry the limit 1, got %d", refused.Limit)
	}
	wantUnchanged(t, group, before)

	opened, err := group.Nominate(blair.ID, candidates(grace), today)
	if err != nil {
		t.Fatalf("Blair has room under the member limit: %v", err)
	}
	got := nomination(t, group, opened[0].Nomination.Number)
	if got.Person.ID != grace.ID || got.Nominator != blair.ID || got.Votes[blair.ID] != domain.Yes {
		t.Fatalf("want Grace nominated by Blair with Blair's yes, got %+v", got)
	}
}

func TestNominationsPerMessageWithinLimit(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 2})
	before := group.Snapshot()

	_, err := group.Nominate(alex.ID, candidates(ada, grace, katherine), today)
	refused := refusal(t, err, domain.NominationsPerMessageWithinLimit)
	if refused.Limit != 2 {
		t.Fatalf("the refusal must say at most 2, got %d", refused.Limit)
	}
	wantUnchanged(t, group, before)

	opened, err := group.Nominate(alex.ID, candidates(ada, grace), today)
	if err != nil {
		t.Fatalf("two people fit the single message limit: %v", err)
	}
	if len(opened) != 2 {
		t.Fatalf("want two Nominations opened, got %d", len(opened))
	}
	for _, each := range opened {
		if each.Nomination.Votes[alex.ID] != domain.Yes || len(each.Nomination.Votes) != 1 {
			t.Fatalf("each Nomination opens with only the nominator's yes, got %+v", each.Nomination.Votes)
		}
	}
}

func TestMessageNominatesAllOrNone(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 3}, open(1, ada, blair))
	before := group.Snapshot()
	_, err := group.Nominate(alex.ID, candidates(grace, katherine, dorothy), today)
	refused := refusal(t, err, domain.MessageNominatesAllOrNone)
	if refused.Limit != 2 {
		t.Fatalf("the Group has room for 2, got %d", refused.Limit)
	}
	wantUnchanged(t, group, before)

	group = newGroup(t, domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 2, SingleMessageNominationLimit: 3})
	before = group.Snapshot()
	_, err = group.Nominate(alex.ID, candidates(grace, katherine, dorothy), today)
	refused = refusal(t, err, domain.MessageNominatesAllOrNone)
	if refused.Limit != 2 {
		t.Fatalf("Alex has room for 2, got %d", refused.Limit)
	}
	wantUnchanged(t, group, before)
}

func TestVoteNamesItsNomination(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 2, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3}, open(1, ada, alex), open(2, grace, casey))
	before := group.Snapshot()

	_, err := group.CastVote(blair.ID, []domain.Ballot{{Vote: domain.Yes}}, today)
	refused := refusal(t, err, domain.VoteNamesItsNomination)
	wantNumbers(t, refused.Nominations, 1, 2)
	wantUnchanged(t, group, before)

	_, err = group.CastVote(blair.ID, []domain.Ballot{{Nomination: domain.Ref{Number: 1}, Vote: domain.Yes}, {Vote: domain.No}}, today)
	refusal(t, err, domain.VoteNamesItsNomination)
	wantUnchanged(t, group, before)

	if _, err := group.CastVote(blair.ID, []domain.Ballot{
		{Nomination: domain.Ref{Number: 2}, Vote: domain.Yes},
		{Nomination: domain.Ref{Person: ada.ID}, Vote: domain.No},
	}, today); err != nil {
		t.Fatalf("votes naming their Nominations must count: %v", err)
	}
	if got := nomination(t, group, 2).Votes[blair.ID]; got != domain.Yes {
		t.Fatalf("want Blair's yes on Nomination 2 by number, got %v", got)
	}
	if got := nomination(t, group, 1).Votes[blair.ID]; got != domain.No {
		t.Fatalf("want Blair's no on Nomination 1 by name, got %v", got)
	}
}

func TestUnnamedVoteCountsWithOneOpenNomination(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3}, open(1, ada, alex))

	if _, err := group.CastVote(blair.ID, []domain.Ballot{{Vote: domain.Yes}}, today); err != nil {
		t.Fatalf("with one open Nomination a vote needs no name: %v", err)
	}
	if got := nomination(t, group, 1).Votes[blair.ID]; got != domain.Yes {
		t.Fatalf("want Blair's yes on Nomination 1, got %v", got)
	}
}

func TestRenominatedOnlyThroughRenomination(t *testing.T) {
	settings := domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3}
	group := newGroup(t, settings, finalized(1, ada, domain.Rejected))
	before := group.Snapshot()

	_, err := group.Nominate(blair.ID, candidates(ada), today)
	refused := refusal(t, err, domain.RenominatedOnlyThroughRenomination)
	wantNumbers(t, refused.Nominations, 1)
	if refused.Renominable {
		t.Fatal("a Group that does not allow renomination must not offer it")
	}
	wantUnchanged(t, group, before)

	settings.AllowRenomination = true
	group = newGroup(t, settings, finalized(1, ada, domain.Rejected), finalized(2, grace, domain.Certified))

	_, err = group.Nominate(blair.ID, candidates(grace), today)
	refused = refusal(t, err, domain.RenominatedOnlyThroughRenomination)
	wantNumbers(t, refused.Nominations, 2)
	if refused.Renominable {
		t.Fatal("a certified Person can never be renominated")
	}

	_, err = group.Nominate(blair.ID, candidates(ada), today)
	refused = refusal(t, err, domain.RenominatedOnlyThroughRenomination)
	if !refused.Renominable {
		t.Fatal("a rejected Person in a Group that allows renomination must be offered it")
	}

	renominated, err := group.Renominate(blair.ID, []domain.Candidate{{Person: domain.Person{ID: ada.ID, Name: "ADA LOVELACE"}, Pitch: "Engines"}}, today)
	if err != nil {
		t.Fatalf("renomination must open a Nomination: %v", err)
	}
	got := nomination(t, group, renominated[0].Number)
	if got.Number != 3 || got.Person != ada || got.Nominator != blair.ID || got.Status != domain.Nominated {
		t.Fatalf("want Nomination 3 of Ada as first named, by Blair and open, got %+v", got)
	}
}

func TestRenominatedOnlyWhenRejected(t *testing.T) {
	settings := domain.Settings{GroupOpenNominationLimit: 2, MemberOpenNominationLimit: 2, SingleMessageNominationLimit: 3, AllowRenomination: true}
	group := newGroup(t, settings, open(1, ada, alex), finalized(2, grace, domain.Certified))
	before := group.Snapshot()

	_, err := group.Renominate(blair.ID, candidates(grace), today)
	refused := refusal(t, err, domain.RenominatedOnlyWhenRejected)
	wantNumbers(t, refused.Nominations, 2)
	wantUnchanged(t, group, before)

	_, err = group.Renominate(blair.ID, candidates(ada), today)
	refused = refusal(t, err, domain.RenominatedOnlyWhenRejected)
	wantNumbers(t, refused.Nominations, 1)
	wantUnchanged(t, group, before)

	_, err = group.Renominate(blair.ID, candidates(katherine), today)
	refused = refusal(t, err, domain.RenominatedOnlyWhenRejected)
	if refused.Ref != (domain.Ref{Person: katherine.ID}) || len(refused.Nominations) != 0 {
		t.Fatalf("a Person never nominated must be named with no Nomination, got %+v", refused)
	}
	wantUnchanged(t, group, before)
}

func TestRenominatedOnlyWhenGroupAllows(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3}, finalized(1, ada, domain.Rejected))
	before := group.Snapshot()

	_, err := group.Renominate(blair.ID, candidates(ada), today)
	refusal(t, err, domain.RenominatedOnlyWhenGroupAllows)
	wantUnchanged(t, group, before)
}

func TestNominationNumbersContinueAfterTheHighest(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3},
		finalized(5, ada, domain.Rejected), finalized(9, grace, domain.Certified))

	if _, err := group.Nominate(alex.ID, candidates(ada), today); err == nil {
		t.Fatal("nominating a rejected Person must be refused")
	}
	opened, err := group.Nominate(alex.ID, candidates(katherine), today)
	if err != nil {
		t.Fatalf("Nominate: %v", err)
	}
	if opened[0].Nomination.Number != 10 {
		t.Fatalf("want Nomination 10 after the highest 9, a refusal using no number, got %d", opened[0].Nomination.Number)
	}
}
