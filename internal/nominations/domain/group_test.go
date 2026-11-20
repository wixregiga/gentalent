package domain_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"
	"unicode"

	"gentalent/internal/nominations/domain"
)

type member struct {
	ID   domain.MemberID
	Name string
}

var (
	alex      = member{ID: "member-alex", Name: "Alex"}
	blair     = member{ID: "member-blair", Name: "Blair"}
	casey     = member{ID: "member-casey", Name: "Casey"}
	devon     = member{ID: "member-devon", Name: "Devon"}
	members   = []member{alex, blair, casey}
	memberIDs = []domain.MemberID{alex.ID, blair.ID, casey.ID}

	ada       = domain.Person{ID: "adalovelace", Name: "Ada Lovelace"}
	grace     = domain.Person{ID: "gracehopper", Name: "Grace Hopper"}
	katherine = domain.Person{ID: "katherinejohnson", Name: "Katherine Johnson"}
	dorothy   = domain.Person{ID: "dorothyvaughan", Name: "Dorothy Vaughan"}

	monday = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	today  = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
)

func newGroup(t *testing.T, settings domain.Settings, history ...domain.Nomination) *domain.Group {
	t.Helper()
	group, err := loaded(settings, history...)
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	return group
}

var cast = map[domain.PersonID]domain.Person{}

func spelling(name string) domain.Spelling {
	var folded []rune
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			folded = append(folded, unicode.ToLower(r))
		}
	}
	return domain.Spelling(folded)
}

func personsOf(history []domain.Nomination) []domain.Person {
	var persons []domain.Person
	seen := map[domain.PersonID]bool{}
	for _, nomination := range history {
		if seen[nomination.Person] {
			continue
		}
		seen[nomination.Person] = true
		person := cast[nomination.Person]
		person.Spellings = []domain.Spelling{spelling(person.Name)}
		persons = append(persons, person)
	}
	return persons
}

func loaded(settings domain.Settings, history ...domain.Nomination) (*domain.Group, error) {
	return domain.NewGroup("group-test", memberIDs, settings, personsOf(history), history, nil, nil, nil)
}

func named(t *testing.T, group *domain.Group, id domain.PersonID) string {
	t.Helper()
	person, ok := group.Person(id)
	if !ok {
		t.Fatalf("no Person %s", id)
	}
	return person.Name
}

func open(number domain.NominationNumber, person domain.Person, nominator member) domain.Nomination {
	cast[person.ID] = person
	return domain.Nomination{
		Number:      number,
		Person:      person.ID,
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
	cast[person.ID] = person
	return domain.Nomination{
		Number:      number,
		Person:      person.ID,
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
		list = append(list, domain.Candidate{Spelling: spelling(person.Name), Name: person.Name, Pitch: "A pitch for " + person.Name})
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

func wantNumbers(t *testing.T, got []domain.NominationNumber, want ...domain.NominationNumber) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("want Nominations %v, got %v", want, got)
	}
}

type state struct {
	nominations []domain.Nomination
	shames      []domain.Shame
	whoSearches []domain.WhoSearch
}

func snapshot(group *domain.Group) state {
	return state{group.Nominations(), group.Shames(), group.WhoSearches()}
}

func wantUnchanged(t *testing.T, group *domain.Group, before state) {
	t.Helper()
	if after := snapshot(group); !reflect.DeepEqual(before, after) {
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

func TestGroupHasAtLeastTwoMembers(t *testing.T) {
	valid := domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 1, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}
	for _, tc := range []struct {
		name    string
		roster  []domain.MemberID
		allowed bool
	}{
		{"none", nil, false},
		{"one member", []domain.MemberID{alex.ID}, false},
		{"two identical members", []domain.MemberID{alex.ID, alex.ID}, false},
		{"duplicate among three", []domain.MemberID{alex.ID, blair.ID, alex.ID}, false},
		{"blank member ID", []domain.MemberID{alex.ID, ""}, false},
		{"two distinct members", []domain.MemberID{alex.ID, blair.ID}, true},
		{"four distinct members", []domain.MemberID{alex.ID, blair.ID, casey.ID, devon.ID}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewGroup("group-test", tc.roster, valid, nil, nil, nil, nil, nil)
			if tc.allowed {
				if err != nil {
					t.Fatalf("want Group created, got %v", err)
				}
				return
			}
			refusal(t, err, domain.GroupHasAtLeastTwoMembers)
		})
	}
}

func TestNominationLimitNotLessThanOne(t *testing.T) {
	valid := domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 1, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}
	if _, err := loaded(valid); err != nil {
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
			group, err := loaded(settings, open(1, ada, alex))
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
	settings := domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}
	group := newGroup(t, settings, open(1, ada, alex))
	before := snapshot(group)

	_, err := group.Nominate(blair.ID, candidates(ada), today)
	refused := refusal(t, err, domain.OneOpenNominationPerPerson)
	wantNumbers(t, refused.Numbers, 1)
	wantUnchanged(t, group, before)

	_, err = group.Nominate(blair.ID, candidates(grace, grace), today)
	refused = refusal(t, err, domain.OneOpenNominationPerPerson)
	if refused.Ref.Spelling != spelling(grace.Name) || refused.Ref.Person == "" {
		t.Fatalf("a message naming Grace twice must name her, got %+v", refused.Ref)
	}
	wantUnchanged(t, group, before)

	_, err = loaded(settings, open(1, ada, alex), open(2, ada, blair))
	refused = refusal(t, err, domain.OneOpenNominationPerPerson)
	wantNumbers(t, refused.Numbers, 1, 2)
}

func TestOpenNominationsWithinGroupLimit(t *testing.T) {
	settings := domain.Settings{GroupOpenNominationLimit: 2, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 3, AllowRenomination: true, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}
	group := newGroup(t, settings, finalized(1, dorothy, domain.Rejected), open(2, ada, blair), open(3, grace, casey))
	before := snapshot(group)

	_, err := group.Nominate(alex.ID, candidates(katherine), today)
	refused := refusal(t, err, domain.OpenNominationsWithinGroupLimit)
	wantNumbers(t, refused.Numbers, 2, 3)
	if refused.Limit != 2 {
		t.Fatalf("the refusal must carry the limit 2, got %d", refused.Limit)
	}
	wantUnchanged(t, group, before)

	_, err = group.Renominate(alex.ID, candidates(dorothy), today)
	refused = refusal(t, err, domain.OpenNominationsWithinGroupLimit)
	wantNumbers(t, refused.Numbers, 2, 3)
	wantUnchanged(t, group, before)

	settings.GroupOpenNominationLimit = 3
	group = newGroup(t, settings, finalized(1, dorothy, domain.Rejected), open(2, ada, blair), open(3, grace, casey))
	opened, err := group.Nominate(alex.ID, candidates(katherine), today)
	if err != nil {
		t.Fatalf("a Group below its limit must accept the nomination: %v", err)
	}
	if len(opened) != 1 || named(t, group, nomination(t, group, opened[0].Number).Person) != katherine.Name || group.Spellings()[spelling(katherine.Name)] != nomination(t, group, opened[0].Number).Person {
		t.Fatalf("want Katherine opened, got %+v", opened)
	}
}

func TestLoweringTheGroupLimitRefusesOnlyNewNominations(t *testing.T) {
	history := []domain.Nomination{open(1, ada, blair), open(2, grace, casey)}
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}, history...)

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
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}, open(1, ada, alex))
	before := snapshot(group)

	_, err := group.Nominate(alex.ID, candidates(grace), today)
	refused := refusal(t, err, domain.OpenNominationsWithinMemberLimit)
	wantNumbers(t, refused.Numbers, 1)
	if refused.Limit != 1 {
		t.Fatalf("the refusal must carry the limit 1, got %d", refused.Limit)
	}
	wantUnchanged(t, group, before)

	opened, err := group.Nominate(blair.ID, candidates(grace), today)
	if err != nil {
		t.Fatalf("Blair has room under the member limit: %v", err)
	}
	got := nomination(t, group, opened[0].Number)
	if named(t, group, got.Person) != grace.Name || got.Person != group.Spellings()[spelling(grace.Name)] || got.Nominator != blair.ID || got.Votes[blair.ID] != domain.Yes {
		t.Fatalf("want Grace nominated by Blair with Blair's yes, got %+v", got)
	}
}

func TestNominationsPerMessageWithinLimit(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 2, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour})
	before := snapshot(group)

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
		if votes := nomination(t, group, each.Number).Votes; votes[alex.ID] != domain.Yes || len(votes) != 1 {
			t.Fatalf("each Nomination opens with only the nominator's yes, got %+v", votes)
		}
	}
}

func TestMessageNominatesAllOrNone(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}, open(1, ada, blair))
	before := snapshot(group)
	_, err := group.Nominate(alex.ID, candidates(grace, katherine, dorothy), today)
	refused := refusal(t, err, domain.MessageNominatesAllOrNone)
	if refused.Room != 2 {
		t.Fatalf("the Group has room for 2, got %d", refused.Room)
	}
	wantUnchanged(t, group, before)

	group = newGroup(t, domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 2, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour})
	before = snapshot(group)
	_, err = group.Nominate(alex.ID, candidates(grace, katherine, dorothy), today)
	refused = refusal(t, err, domain.MessageNominatesAllOrNone)
	if refused.Room != 2 {
		t.Fatalf("Alex has room for 2, got %d", refused.Room)
	}
	wantUnchanged(t, group, before)
}

func TestVoteNamesItsNomination(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 2, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}, open(1, ada, alex), open(2, grace, casey))
	before := snapshot(group)

	_, err := group.CastVote(blair.ID, []domain.Ballot{{Vote: domain.Yes}}, today)
	refused := refusal(t, err, domain.VoteNamesItsNomination)
	wantNumbers(t, refused.Numbers, 1, 2)
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
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}, open(1, ada, alex))

	if _, err := group.CastVote(blair.ID, []domain.Ballot{{Vote: domain.Yes}}, today); err != nil {
		t.Fatalf("with one open Nomination a vote needs no name: %v", err)
	}
	if got := nomination(t, group, 1).Votes[blair.ID]; got != domain.Yes {
		t.Fatalf("want Blair's yes on Nomination 1, got %v", got)
	}
}

func TestRenominatedOnlyThroughRenomination(t *testing.T) {
	settings := domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}
	group := newGroup(t, settings, finalized(1, ada, domain.Rejected))
	before := snapshot(group)

	_, err := group.Nominate(blair.ID, candidates(ada), today)
	refused := refusal(t, err, domain.RenominatedOnlyThroughRenomination)
	wantNumbers(t, refused.Numbers, 1)
	if refused.Renominable {
		t.Fatal("a Group that does not allow renomination must not offer it")
	}
	wantUnchanged(t, group, before)

	settings.AllowRenomination = true
	group = newGroup(t, settings, finalized(1, ada, domain.Rejected), finalized(2, grace, domain.Certified))

	_, err = group.Nominate(blair.ID, candidates(grace), today)
	refused = refusal(t, err, domain.RenominatedOnlyThroughRenomination)
	wantNumbers(t, refused.Numbers, 2)
	if refused.Renominable {
		t.Fatal("a certified Person can never be renominated")
	}

	_, err = group.Nominate(blair.ID, candidates(ada), today)
	refused = refusal(t, err, domain.RenominatedOnlyThroughRenomination)
	if !refused.Renominable {
		t.Fatal("a rejected Person in a Group that allows renomination must be offered it")
	}

	renominated, err := group.Renominate(blair.ID, []domain.Candidate{{Spelling: domain.Spelling(ada.ID), Name: "ADA LOVELACE", Pitch: "Engines"}}, today)
	if err != nil {
		t.Fatalf("renomination must open a Nomination: %v", err)
	}
	got := nomination(t, group, renominated[0].Number)
	if got.Number != 3 || got.Person != ada.ID || got.Nominator != blair.ID || got.Status != domain.Nominated {
		t.Fatalf("want Nomination 3 of Ada as first named, by Blair and open, got %+v", got)
	}
}

func TestRenominatedOnlyWhenRejected(t *testing.T) {
	settings := domain.Settings{GroupOpenNominationLimit: 2, MemberOpenNominationLimit: 2, SingleMessageNominationLimit: 3, AllowRenomination: true, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}
	group := newGroup(t, settings, open(1, ada, alex), finalized(2, grace, domain.Certified))
	before := snapshot(group)

	_, err := group.Renominate(blair.ID, candidates(grace), today)
	refused := refusal(t, err, domain.RenominatedOnlyWhenRejected)
	wantNumbers(t, refused.Numbers, 2)
	wantUnchanged(t, group, before)

	_, err = group.Renominate(blair.ID, candidates(ada), today)
	refused = refusal(t, err, domain.RenominatedOnlyWhenRejected)
	wantNumbers(t, refused.Numbers, 1)
	wantUnchanged(t, group, before)

	_, err = group.Renominate(blair.ID, candidates(katherine), today)
	refused = refusal(t, err, domain.RenominatedOnlyWhenRejected)
	if refused.Ref.Spelling != spelling(katherine.Name) || len(refused.Numbers) != 0 {
		t.Fatalf("a Person never nominated must be named with no Nomination, got %+v", refused)
	}
	wantUnchanged(t, group, before)
}

func TestRenominatedOnlyWhenGroupAllows(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}, finalized(1, ada, domain.Rejected))
	before := snapshot(group)

	_, err := group.Renominate(blair.ID, candidates(ada), today)
	refusal(t, err, domain.RenominatedOnlyWhenGroupAllows)
	wantUnchanged(t, group, before)
}

func TestNominationNumbersContinueAfterTheHighest(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour},
		finalized(5, ada, domain.Rejected), finalized(9, grace, domain.Certified))

	if _, err := group.Nominate(alex.ID, candidates(ada), today); err == nil {
		t.Fatal("nominating a rejected Person must be refused")
	}
	opened, err := group.Nominate(alex.ID, candidates(katherine), today)
	if err != nil {
		t.Fatalf("Nominate: %v", err)
	}
	if opened[0].Number != 10 {
		t.Fatalf("want Nomination 10 after the highest 9, a refusal using no number, got %d", opened[0].Number)
	}
}

func TestGettersReturnCopiesTheCallerCannotChange(t *testing.T) {
	group := newGroup(t, domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}, open(1, ada, alex))

	got := nomination(t, group, 1)
	got.Votes[blair.ID] = domain.No
	got.Exhibits = append(got.Exhibits, "forged")
	group.Nominations()[0].Votes[casey.ID] = domain.No
	group.Open()[0].Votes[casey.ID] = domain.No

	if votes := nomination(t, group, 1).Votes; len(votes) != 1 || votes[alex.ID] != domain.Yes {
		t.Fatalf("changing a returned Nomination changed the Group: %+v", votes)
	}
}
