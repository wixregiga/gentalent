package domain

import (
	"errors"
	"maps"
	"slices"
	"time"
)

type NameFixNumber int

type NameFixStatus string

const (
	NameFixPending   NameFixStatus = "pending"
	NameFixed        NameFixStatus = "fixed"
	NameFixStopped   NameFixStatus = "stopped"
	NameFixWithdrawn NameFixStatus = "withdrawn"
	NameFixLapsed    NameFixStatus = "lapsed"
)

type NameFix struct {
	Number     NameFixNumber
	Nomination NominationNumber
	Person     PersonID
	From       string
	To         string
	Spelling   Spelling
	Proposer   MemberID
	Answers    map[MemberID]Vote
	WithPhotos bool
	ProposedAt time.Time
	Status     NameFixStatus
	EndedAt    time.Time
}

type NameFixProposal struct {
	Nomination NominationNumber
	Name       string
	Spelling   Spelling
	WithPhotos bool
}

func (f NameFix) Pending() bool {
	return f.Status == NameFixPending
}

func (f NameFix) Count(answer Vote) int {
	count := 0
	for _, given := range f.Answers {
		if given == answer {
			count++
		}
	}
	return count
}

func (f NameFix) clone() NameFix {
	f.Answers = maps.Clone(f.Answers)
	return f
}

func cloneFixes(fixes []NameFix) []NameFix {
	copies := make([]NameFix, len(fixes))
	for i, fix := range fixes {
		copies[i] = fix.clone()
	}
	return copies
}

func (g *Group) ProposeNameFix(by MemberID, proposal NameFixProposal, at time.Time) (NameFix, error) {
	next := g.clone()
	if err := next.requireMember(by); err != nil {
		return NameFix{}, err
	}
	if err := next.AssertNameFixedOnlyWhenGroupAllows(); err != nil {
		return NameFix{}, err
	}
	i, ok := next.index(proposal.Nomination)
	if !ok {
		return NameFix{}, Refusal{Rule: UnknownNomination, Ref: Ref{Number: proposal.Nomination}}
	}
	nomination := next.nominations[i]
	if proposal.Name == nomination.Person.Name {
		return NameFix{}, Refusal{Rule: NameUnchanged, Numbers: []NominationNumber{nomination.Number}}
	}
	if err := next.AssertNameFixedOncePerPerson(nomination.Person.ID); err != nil {
		return NameFix{}, err
	}
	if err := next.AssertNameFixNeverTakesAnotherPersonsSpelling(nomination.Person.ID, proposal.Spelling); err != nil {
		return NameFix{}, err
	}
	if proposal.WithPhotos {
		if err := next.AssertPhotosSearchedAtMostOncePerNomination(nomination); err != nil {
			return NameFix{}, err
		}
	}
	fix := NameFix{
		Number:     next.nextFixNumber(),
		Nomination: nomination.Number,
		Person:     nomination.Person.ID,
		From:       nomination.Person.Name,
		To:         proposal.Name,
		Spelling:   proposal.Spelling,
		Proposer:   by,
		Answers:    map[MemberID]Vote{},
		WithPhotos: proposal.WithPhotos,
		ProposedAt: at,
		Status:     NameFixPending,
	}
	next.nameFixes = append(next.nameFixes, fix)
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return NameFix{}, err
	}
	agreed, err := next.AgreeToNameFix(by, fix.Number, at)
	if err != nil {
		return NameFix{}, err
	}
	*g = *next
	return agreed, nil
}

func (g *Group) AgreeToNameFix(by MemberID, number NameFixNumber, at time.Time) (NameFix, error) {
	next := g.clone()
	if err := next.requireMember(by); err != nil {
		return NameFix{}, err
	}
	i, err := next.pendingFix(number)
	if err != nil {
		return NameFix{}, err
	}
	fix := &next.nameFixes[i]
	fix.Answers[by] = Yes
	if next.AssertNameFixedOnlyWhenEnoughMembersAgree(*fix) == nil {
		if err := next.AssertNameFixedOncePerPerson(fix.Person); err != nil {
			return NameFix{}, err
		}
		if err := next.AssertNameFixNeverTakesAnotherPersonsSpelling(fix.Person, fix.Spelling); err != nil {
			return NameFix{}, err
		}
		next.fixName(i, at)
		if err := next.AssertEarlierSpellingsStillNameThePerson(g.spellings, fix.Person); err != nil {
			return NameFix{}, err
		}
	}
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return NameFix{}, err
	}
	*g = *next
	return next.nameFixes[i].clone(), nil
}

func (g *Group) StopNameFix(by MemberID, number NameFixNumber, at time.Time) (NameFix, error) {
	next := g.clone()
	if err := next.requireMember(by); err != nil {
		return NameFix{}, err
	}
	i, err := next.pendingFix(number)
	if err != nil {
		return NameFix{}, err
	}
	fix := &next.nameFixes[i]
	fix.Answers[by] = No
	switch {
	case by == fix.Proposer:
		fix.Status, fix.EndedAt = NameFixWithdrawn, at
	case next.outOfReach(*fix):
		fix.Status, fix.EndedAt = NameFixStopped, at
	}
	if err := next.AssertNameFixStoppedWhenAgreementIsOutOfReach(*fix); err != nil {
		return NameFix{}, err
	}
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return NameFix{}, err
	}
	*g = *next
	return fix.clone(), nil
}

func (g *Group) ExpireNameFixes(now time.Time) ([]NameFix, error) {
	next := g.clone()
	var lapsed []NameFix
	for i := range next.nameFixes {
		fix := &next.nameFixes[i]
		if fix.Pending() && next.AssertNameFixLapsesAfterTheNameFixWindow(*fix, now) == nil {
			fix.Status, fix.EndedAt = NameFixLapsed, now
			lapsed = append(lapsed, fix.clone())
		}
	}
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return nil, err
	}
	*g = *next
	return lapsed, nil
}

func (g *Group) AssertNameFixedOnlyWhenGroupAllows() error {
	if !g.settings.AllowNameFixes {
		return Refusal{Rule: NameFixedOnlyWhenGroupAllows}
	}
	return nil
}

func (g *Group) AssertNameFixedOnlyWhenEnoughMembersAgree(fix NameFix) error {
	if g.agreed(fix) < g.AgreementNeeded() {
		return Refusal{Rule: NameFixedOnlyWhenEnoughMembersAgree, Fix: fix.Number}
	}
	return nil
}

func (g *Group) AssertNameFixedOncePerPerson(person PersonID) error {
	for _, fix := range g.nameFixes {
		if fix.Person == person && fix.Status == NameFixed {
			return Refusal{Rule: NameFixedOncePerPerson, Fix: fix.Number, Numbers: []NominationNumber{fix.Nomination}}
		}
	}
	return nil
}

func (g *Group) AssertNameFixNeverTakesAnotherPersonsSpelling(person PersonID, spelling Spelling) error {
	if other, named := g.spellings[spelling]; named && other != person {
		refusal := Refusal{Rule: NameFixNeverTakesAnotherPersonsSpelling, Ref: Ref{Person: other}}
		if i, ok := g.latestOf(other); ok {
			refusal.Numbers = []NominationNumber{g.nominations[i].Number}
		}
		return refusal
	}
	return nil
}

func (g *Group) AssertEarlierSpellingsStillNameThePerson(earlier map[Spelling]PersonID, person PersonID) error {
	for spelling, named := range earlier {
		if named == person && g.spellings[spelling] != person {
			return Refusal{Rule: EarlierSpellingsStillNameThePerson, Ref: Ref{Person: person}}
		}
	}
	return nil
}

func (g *Group) AssertNameFixStoppedWhenAgreementIsOutOfReach(fix NameFix) error {
	if fix.Pending() && g.outOfReach(fix) {
		return Refusal{Rule: NameFixStoppedWhenAgreementIsOutOfReach, Fix: fix.Number}
	}
	return nil
}

func (g *Group) AssertNameFixLapsesAfterTheNameFixWindow(fix NameFix, now time.Time) error {
	if now.Before(fix.ProposedAt.Add(g.settings.NameFixWindow)) {
		return Refusal{Rule: NameFixLapsesAfterTheNameFixWindow, Fix: fix.Number}
	}
	return nil
}

func (g *Group) AgreementNeeded() int {
	return (g.settings.NameFixAgreementPercentage*len(g.members) + 99) / 100
}

func (g *Group) NameFixWindow() time.Duration {
	return g.settings.NameFixWindow
}

func (g *Group) PendingNameFixFor(person PersonID) (NameFix, bool) {
	for _, fix := range g.nameFixes {
		if fix.Person == person && fix.Pending() {
			return fix.clone(), true
		}
	}
	return NameFix{}, false
}

func (g *Group) agreed(fix NameFix) int {
	count := 0
	for _, member := range g.members {
		if fix.Answers[member.ID] == Yes {
			count++
		}
	}
	return count
}

func (g *Group) outOfReach(fix NameFix) bool {
	noes := 0
	for _, member := range g.members {
		if fix.Answers[member.ID] == No {
			noes++
		}
	}
	return len(g.members)-noes < g.AgreementNeeded()
}

func (g *Group) fixName(i int, at time.Time) {
	fix := &g.nameFixes[i]
	for k := range g.nominations {
		if g.nominations[k].Person.ID == fix.Person {
			g.nominations[k].Person.Name = fix.To
		}
	}
	g.spellings[fix.Spelling] = fix.Person
	fix.Status, fix.EndedAt = NameFixed, at
	if !fix.WithPhotos {
		return
	}
	if k, ok := g.index(fix.Nomination); ok && g.AssertPhotosSearchedAtMostOncePerNomination(g.nominations[k]) == nil {
		g.nominations[k].Photos = PhotosRequested
	}
}

func (g *Group) pendingFix(number NameFixNumber) (int, error) {
	i, ok := g.fixIndex(number)
	if !ok {
		return 0, Refusal{Rule: UnknownNameFix, Fix: number}
	}
	if !g.nameFixes[i].Pending() {
		return 0, Refusal{Rule: NameFixOver, Fix: number}
	}
	return i, nil
}

func (g *Group) fixIndex(number NameFixNumber) (int, bool) {
	return slices.BinarySearchFunc(g.nameFixes, number, func(f NameFix, target NameFixNumber) int {
		return int(f.Number - target)
	})
}

func (g *Group) nextFixNumber() NameFixNumber {
	if len(g.nameFixes) == 0 {
		return 1
	}
	return g.nameFixes[len(g.nameFixes)-1].Number + 1
}
