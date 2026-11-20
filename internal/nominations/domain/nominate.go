package domain

import (
	"errors"
	"time"
)

type Candidate struct {
	Spelling   Spelling
	Name       string
	Pitch      string
	WithPhotos bool
}

type NominateOutcome struct {
	Number     NominationNumber
	PhotosOnly bool
}

func (g *Group) Nominate(by MemberID, candidates []Candidate, at time.Time) ([]NominateOutcome, error) {
	next := g.clone()
	if err := next.requireMember(by); err != nil {
		return nil, err
	}
	people, err := next.people(candidates, at)
	if err != nil {
		return nil, err
	}
	nominated := make([]NominateOutcome, len(candidates))
	var fresh []int
	for k, candidate := range candidates {
		i, open := next.openIndexOf(people[k])
		if !open {
			if err := next.AssertRenominatedOnlyThroughRenomination(people[k]); err != nil {
				return nil, err
			}
			fresh = append(fresh, k)
			continue
		}
		existing := &next.nominations[i]
		if !candidate.WithPhotos {
			return nil, Refusal{Rule: OneOpenNominationPerPerson, Numbers: []NominationNumber{existing.Number}}
		}
		if err := next.AssertPhotosSearchedAtMostOncePerNomination(*existing); err != nil {
			return nil, err
		}
		if existing.Locked() {
			return nil, Refusal{Rule: OneOpenNominationPerPerson, Numbers: []NominationNumber{existing.Number}}
		}
		existing.Photos = PhotosRequested
		if candidate.Pitch != "" {
			existing.Pitch = candidate.Pitch
			existing.exhibit(candidate.Pitch)
		}
		nominated[k] = NominateOutcome{Number: existing.Number, PhotosOnly: true}
	}
	if err := next.AssertNominationsPerMessageWithinLimit(len(candidates)); err != nil {
		return nil, err
	}
	if len(fresh) > 0 {
		if err := errors.Join(next.AssertOpenNominationsWithinGroupLimit(), next.AssertOpenNominationsWithinMemberLimit(by)); err != nil {
			return nil, err
		}
		if err := next.AssertMessageNominatesAllOrNone(by, len(fresh)); err != nil {
			return nil, err
		}
	}
	for _, k := range fresh {
		candidate := candidates[k]
		next.persons[people[k]] = Person{ID: people[k], Name: candidate.Name, Spellings: []Spelling{candidate.Spelling}}
		nominated[k] = NominateOutcome{Number: next.openNomination(by, people[k], candidate, at).Number}
	}
	if err := errors.Join(next.EnsureGroupHasAtLeastTwoMembers(), next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return nil, err
	}
	*g = *next
	return nominated, nil
}

func (g *Group) Renominate(by MemberID, candidates []Candidate, at time.Time) ([]Nomination, error) {
	next := g.clone()
	if err := next.requireMember(by); err != nil {
		return nil, err
	}
	if err := next.AssertRenominatedOnlyWhenGroupAllows(); err != nil {
		return nil, err
	}
	people, err := next.people(candidates, at)
	if err != nil {
		return nil, err
	}
	for k, person := range people {
		if err := next.AssertRenominatedOnlyWhenRejected(person, candidates[k].Spelling); err != nil {
			return nil, err
		}
	}
	if err := next.AssertNominationsPerMessageWithinLimit(len(candidates)); err != nil {
		return nil, err
	}
	if err := errors.Join(next.AssertOpenNominationsWithinGroupLimit(), next.AssertOpenNominationsWithinMemberLimit(by)); err != nil {
		return nil, err
	}
	if err := next.AssertMessageNominatesAllOrNone(by, len(candidates)); err != nil {
		return nil, err
	}
	renominated := make([]Nomination, 0, len(candidates))
	for k, candidate := range candidates {
		renominated = append(renominated, next.openNomination(by, people[k], candidate, at))
	}
	if err := errors.Join(next.EnsureGroupHasAtLeastTwoMembers(), next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return nil, err
	}
	*g = *next
	return renominated, nil
}

func (g *Group) AssertRenominatedOnlyThroughRenomination(person PersonID) error {
	i, ok := g.latestOf(person)
	if !ok {
		return nil
	}
	latest := g.nominations[i]
	return Refusal{
		Rule:        RenominatedOnlyThroughRenomination,
		Numbers:     []NominationNumber{latest.Number},
		Renominable: g.settings.AllowRenomination && latest.Status == Rejected,
	}
}

func (g *Group) AssertRenominatedOnlyWhenRejected(person PersonID, spelling Spelling) error {
	i, ok := g.latestOf(person)
	if !ok {
		return Refusal{Rule: RenominatedOnlyWhenRejected, Ref: Ref{Person: person, Spelling: spelling}}
	}
	if latest := g.nominations[i]; latest.Status != Rejected {
		return Refusal{Rule: RenominatedOnlyWhenRejected, Numbers: []NominationNumber{latest.Number}}
	}
	return nil
}

func (g *Group) AssertRenominatedOnlyWhenGroupAllows() error {
	if !g.settings.AllowRenomination {
		return Refusal{Rule: RenominatedOnlyWhenGroupAllows}
	}
	return nil
}

func (g *Group) AssertNominationsPerMessageWithinLimit(count int) error {
	if limit := g.settings.SingleMessageNominationLimit; count > limit {
		return Refusal{Rule: NominationsPerMessageWithinLimit, Limit: limit}
	}
	return nil
}

func (g *Group) AssertOpenNominationsWithinGroupLimit() error {
	if open, limit := g.open(), g.settings.GroupOpenNominationLimit; len(open) >= limit {
		return Refusal{Rule: OpenNominationsWithinGroupLimit, Numbers: numbers(open), Limit: limit}
	}
	return nil
}

func (g *Group) AssertOpenNominationsWithinMemberLimit(member MemberID) error {
	if open, limit := g.openOf(member), g.settings.MemberOpenNominationLimit; len(open) >= limit {
		return Refusal{Rule: OpenNominationsWithinMemberLimit, Numbers: numbers(open), Limit: limit}
	}
	return nil
}

func (g *Group) AssertMessageNominatesAllOrNone(member MemberID, count int) error {
	room := min(g.settings.GroupOpenNominationLimit-len(g.open()), g.settings.MemberOpenNominationLimit-len(g.openOf(member)))
	if count > room {
		return Refusal{Rule: MessageNominatesAllOrNone, Room: max(room, 0)}
	}
	return nil
}

func (g *Group) AssertPhotosSearchedAtMostOncePerNomination(nomination Nomination) error {
	if nomination.Photos != NoPhotos {
		return Refusal{Rule: PhotosSearchedAtMostOncePerNomination, Numbers: []NominationNumber{nomination.Number}}
	}
	return nil
}

func (g *Group) openNomination(by MemberID, person PersonID, candidate Candidate, at time.Time) Nomination {
	nomination := Nomination{
		Number:      g.nextNumber(),
		Person:      person,
		Nominator:   by,
		Status:      Nominated,
		Votes:       map[MemberID]Vote{by: Yes},
		NominatedAt: at,
		Pitch:       candidate.Pitch,
	}
	nomination.exhibit(candidate.Pitch)
	if candidate.WithPhotos {
		nomination.Photos = PhotosRequested
	}
	if len(nomination.Votes) == len(g.members) {
		nomination.LockedAt = at
	}
	g.nominations = append(g.nominations, nomination)
	return nomination.clone()
}

func (g *Group) openIndexOf(person PersonID) (int, bool) {
	i, ok := g.latestOf(person)
	return i, ok && g.nominations[i].Open()
}

func (g *Group) people(candidates []Candidate, at time.Time) ([]PersonID, error) {
	people := make([]PersonID, len(candidates))
	unknown := map[Spelling]PersonID{}
	seen := map[PersonID]bool{}
	for k, candidate := range candidates {
		person, known := g.spelled(candidate.Spelling)
		if !known {
			if person, known = unknown[candidate.Spelling]; !known {
				person = newPersonID(at)
				unknown[candidate.Spelling] = person
			}
		}
		if seen[person] {
			return nil, Refusal{Rule: OneOpenNominationPerPerson, Ref: Ref{Person: person, Spelling: candidate.Spelling}}
		}
		seen[person] = true
		people[k] = person
	}
	return people, nil
}
