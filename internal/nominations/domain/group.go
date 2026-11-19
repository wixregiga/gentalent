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
	spellings   map[Spelling]PersonID
	nameFixes   []NameFix
}

func NewGroup(id GroupID, members []MemberID, settings Settings, nominations []Nomination, shames []Shame,
	spellings map[Spelling]PersonID, nameFixes []NameFix) (*Group, error) {
	group := &Group{
		id:          id,
		members:     roster(members),
		settings:    settings,
		nominations: cloneAll(nominations),
		shames:      slices.Clone(shames),
		spellings:   maps.Clone(spellings),
		nameFixes:   cloneFixes(nameFixes),
	}
	if group.spellings == nil {
		group.spellings = map[Spelling]PersonID{}
	}
	slices.SortFunc(group.nominations, func(a, b Nomination) int { return int(a.Number - b.Number) })
	slices.SortFunc(group.nameFixes, func(a, b NameFix) int { return int(a.Number - b.Number) })
	if err := errors.Join(group.EnsureNominationLimitNotLessThanOne(), group.EnsureOneOpenNominationPerPerson(), group.EnsureCertifiedByUnanimousYes(),
		group.EnsureOnePendingNameFixPerPerson(), group.EnsureNameFixAgreementPercentageWithinRange(), group.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return nil, err
	}
	return group, nil
}

func (g *Group) Spellings() map[Spelling]PersonID {
	return maps.Clone(g.spellings)
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
		open[nomination.Person.ID] = append(open[nomination.Person.ID], nomination)
	}
	for _, nomination := range g.open() {
		if twice := open[nomination.Person.ID]; len(twice) > 1 {
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
	copied.spellings = maps.Clone(g.spellings)
	copied.nameFixes = cloneFixes(g.nameFixes)
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
		if g.nominations[i].Person.ID == person {
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
