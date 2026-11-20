package domain

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"
)

type GroupID string

type Group struct {
	id          GroupID
	members     []Member
	settings    Settings
	nominations []Nomination
	shames      []Shame
	persons     map[PersonID]Person
	nameFixes   []NameFix
	whoSearches []WhoSearch
}

func NewGroup(id GroupID, members []MemberID, settings Settings, persons []Person, nominations []Nomination, shames []Shame,
	nameFixes []NameFix, whoSearches []WhoSearch) (*Group, error) {
	group := &Group{
		id:          id,
		members:     roster(members),
		settings:    settings,
		nominations: cloneAll(nominations),
		shames:      slices.Clone(shames),
		persons:     map[PersonID]Person{},
		nameFixes:   cloneFixes(nameFixes),
		whoSearches: slices.Clone(whoSearches),
	}
	named := map[Spelling]PersonID{}
	for _, person := range persons {
		if _, twice := group.persons[person.ID]; twice {
			return nil, fmt.Errorf("Person %s is loaded twice", person.ID)
		}
		for _, spelling := range person.Spellings {
			if other, taken := named[spelling]; taken {
				return nil, fmt.Errorf("the spelling %q names both %s and %s", spelling, other, person.ID)
			}
			named[spelling] = person.ID
		}
		group.persons[person.ID] = person.clone()
	}
	for _, nomination := range group.nominations {
		if _, known := group.persons[nomination.Person]; !known {
			return nil, fmt.Errorf("Nomination %d names Person %s, who is not loaded", nomination.Number, nomination.Person)
		}
	}
	slices.SortFunc(group.nominations, func(a, b Nomination) int { return int(a.Number - b.Number) })
	for _, search := range group.whoSearches {
		if person := search.Figure.Person; person != "" {
			if _, known := group.persons[person]; !known {
				return nil, fmt.Errorf("who search %d names Person %s, who is not loaded", search.Number, person)
			}
		}
	}
	slices.SortFunc(group.nameFixes, func(a, b NameFix) int { return int(a.Number - b.Number) })
	slices.SortFunc(group.whoSearches, func(a, b WhoSearch) int { return int(a.Number - b.Number) })
	if err := errors.Join(group.EnsureGroupHasAtLeastTwoMembers(), group.EnsureNominationLimitNotLessThanOne(), group.EnsureOneOpenNominationPerPerson(), group.EnsureCertifiedByUnanimousYes(),
		group.EnsureOnePendingNameFixPerPerson(), group.EnsureNameFixAgreementPercentageWithinRange(), group.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return nil, err
	}
	return group, nil
}

func (g *Group) Person(id PersonID) (Person, bool) {
	person, ok := g.persons[id]
	return person.clone(), ok
}

func (g *Group) Persons() []Person {
	persons := make([]Person, 0, len(g.persons))
	for _, id := range slices.Sorted(maps.Keys(g.persons)) {
		persons = append(persons, g.persons[id].clone())
	}
	return persons
}

func (g *Group) Spellings() map[Spelling]PersonID {
	spellings := map[Spelling]PersonID{}
	for id, person := range g.persons {
		for _, spelling := range person.Spellings {
			spellings[spelling] = id
		}
	}
	return spellings
}

func (g *Group) spelled(spelling Spelling) (PersonID, bool) {
	for id, person := range g.persons {
		if person.SpelledAs(spelling) {
			return id, true
		}
	}
	return "", false
}

func (g *Group) NameFixes() []NameFix {
	return cloneFixes(g.nameFixes)
}

func (g *Group) NameFix(number NameFixNumber) (NameFix, bool) {
	if i, ok := g.fixIndex(number); ok {
		return g.nameFixes[i].clone(), true
	}
	return NameFix{}, false
}

func (g *Group) ID() GroupID {
	return g.id
}

func (g *Group) Nominations() []Nomination {
	return cloneAll(g.nominations)
}

func (g *Group) Shames() []Shame {
	return slices.Clone(g.shames)
}

func roster(ids []MemberID) []Member {
	members := make([]Member, len(ids))
	for i, id := range ids {
		members[i] = Member{ID: id}
	}
	return members
}

func (g *Group) Nomination(number NominationNumber) (Nomination, bool) {
	if i, ok := g.index(number); ok {
		return g.nominations[i].clone(), true
	}
	return Nomination{}, false
}

func (g *Group) Open() []Nomination {
	return cloneAll(g.open())
}

func (g *Group) GifUses() map[string]int {
	uses := map[string]int{}
	for _, shame := range g.shames {
		if shame.GIF != "" {
			uses[shame.GIF]++
		}
	}
	return uses
}

func (g *Group) EnsureGroupHasAtLeastTwoMembers() error {
	if len(g.members) < 2 {
		return Refusal{Rule: GroupHasAtLeastTwoMembers}
	}
	seen := make(map[MemberID]bool, len(g.members))
	for _, member := range g.members {
		if member.ID == "" || seen[member.ID] {
			return Refusal{Rule: GroupHasAtLeastTwoMembers}
		}
		seen[member.ID] = true
	}
	return nil
}

func (g *Group) EnsureNominationLimitNotLessThanOne() error {
	limits := []struct {
		limit Limit
		value int
	}{
		{GroupOpenNominationLimit, g.settings.GroupOpenNominationLimit},
		{MemberOpenNominationLimit, g.settings.MemberOpenNominationLimit},
		{SingleMessageNominationLimit, g.settings.SingleMessageNominationLimit},
	}
	for _, each := range limits {
		if each.value < 1 {
			return LimitBelowOne{Limit: each.limit, Value: each.value}
		}
	}
	return nil
}

func (g *Group) EnsureOneOpenNominationPerPerson() error {
	open := map[PersonID][]Nomination{}
	for _, nomination := range g.open() {
		open[nomination.Person] = append(open[nomination.Person], nomination)
	}
	for _, nomination := range g.open() {
		if twice := open[nomination.Person]; len(twice) > 1 {
			return Refusal{Rule: OneOpenNominationPerPerson, Numbers: numbers(twice)}
		}
	}
	return nil
}

func (g *Group) EnsureOnePendingNameFixPerPerson() error {
	pending := map[PersonID]NameFixNumber{}
	for _, fix := range g.nameFixes {
		if !fix.Pending() {
			continue
		}
		if first, twice := pending[fix.Person]; twice {
			return Refusal{Rule: OnePendingNameFixPerPerson, Fix: first}
		}
		pending[fix.Person] = fix.Number
	}
	return nil
}

func (g *Group) EnsureNameFixAgreementPercentageWithinRange() error {
	if percentage := g.settings.NameFixAgreementPercentage; percentage <= 0 || percentage > 100 {
		return AgreementPercentageOutOfRange{Value: percentage}
	}
	return nil
}

func (g *Group) EnsureNameFixWindowAtLeastOneHour() error {
	if g.settings.NameFixWindow < time.Hour {
		return NameFixWindowBelowAnHour{Value: g.settings.NameFixWindow}
	}
	return nil
}

func (g *Group) EnsureCertifiedByUnanimousYes() error {
	for _, nomination := range g.nominations {
		noes := nomination.Count(No)
		if (nomination.Status == Certified && noes > 0) || (nomination.Status == Rejected && noes == 0) {
			return Refusal{Rule: CertifiedByUnanimousYes, Numbers: []NominationNumber{nomination.Number}}
		}
	}
	return nil
}

func (g *Group) clone() *Group {
	copied := *g
	copied.nominations = cloneAll(g.nominations)
	copied.shames = slices.Clone(g.shames)
	copied.persons = make(map[PersonID]Person, len(g.persons))
	for id, person := range g.persons {
		copied.persons[id] = person.clone()
	}
	copied.nameFixes = cloneFixes(g.nameFixes)
	copied.whoSearches = slices.Clone(g.whoSearches)
	return &copied
}

func (g *Group) requireMember(id MemberID) error {
	if !g.isMember(id) {
		return fmt.Errorf("%s is not a Member of the Group", id)
	}
	return nil
}

func (g *Group) isMember(id MemberID) bool {
	return slices.ContainsFunc(g.members, func(member Member) bool { return member.ID == id })
}

func (g *Group) index(number NominationNumber) (int, bool) {
	return slices.BinarySearchFunc(g.nominations, number, func(n Nomination, target NominationNumber) int {
		return int(n.Number - target)
	})
}

func (g *Group) open() []Nomination {
	var open []Nomination
	for _, nomination := range g.nominations {
		if nomination.Open() {
			open = append(open, nomination)
		}
	}
	return open
}

func (g *Group) openOf(member MemberID) []Nomination {
	var open []Nomination
	for _, nomination := range g.open() {
		if nomination.Nominator == member {
			open = append(open, nomination)
		}
	}
	return open
}

func (g *Group) latestOf(person PersonID) (int, bool) {
	for i := len(g.nominations) - 1; i >= 0; i-- {
		if g.nominations[i].Person == person {
			return i, true
		}
	}
	return 0, false
}

func (g *Group) nextNumber() NominationNumber {
	if len(g.nominations) == 0 {
		return 1
	}
	return g.nominations[len(g.nominations)-1].Number + 1
}

func (g *Group) unanimous(nomination Nomination) bool {
	for _, member := range g.members {
		if nomination.Votes[member.ID] != Yes {
			return false
		}
	}
	return true
}

func numbers(nominations []Nomination) []NominationNumber {
	list := make([]NominationNumber, 0, len(nominations))
	for _, nomination := range nominations {
		list = append(list, nomination.Number)
	}
	return list
}
