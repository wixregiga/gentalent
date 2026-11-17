package domain

import (
	"errors"
	"time"
)

type Candidate struct {
	Person     PersonID
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
	if err := namedOnce(candidates); err != nil {
		return nil, err
	}
	nominated := make([]NominateOutcome, len(candidates))
	var fresh []int
	for k, candidate := range candidates {
		i, open := next.openIndexOf(candidate.Person)
		if !open {
			if err := next.AssertRenominatedOnlyThroughRenomination(candidate.Person); err != nil {
				return nil, err
			}
			fresh = append(fresh, k)
			continue
		}
		existing := &next.nominations[i]
		if !candidate.WithPhotos {
			return nil, Refusal{Rule: OneOpenNominationPerPerson, Numbers: []NominationNumber{existing.Number}}
		}
		if err := next.AssertAiRunsAtMostOncePerNominee(*existing); err != nil {
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
		nominated[k] = NominateOutcome{Number: next.openNomination(by, Person{ID: candidate.Person, Name: candidate.Name}, candidate, at).Number}
	}
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes()); err != nil {
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
	if err := namedOnce(candidates); err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		if err := next.AssertRenominatedOnlyWhenRejected(candidate.Person); err != nil {
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
	for _, candidate := range candidates {
		i, _ := next.latestOf(candidate.Person)
		renominated = append(renominated, next.openNomination(by, next.nominations[i].Person, candidate, at))
	}
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes()); err != nil {
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

func (g *Group) AssertRenominatedOnlyWhenRejected(person PersonID) error {
	i, ok := g.latestOf(person)
	if !ok {
		return Refusal{Rule: RenominatedOnlyWhenRejected, Ref: Ref{Person: person}}
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

func (g *Group) AssertAiRunsAtMostOncePerNominee(nomination Nomination) error {
	if nomination.Photos != NoPhotos {
		return Refusal{Rule: AiRunsAtMostOncePerNominee, Numbers: []NominationNumber{nomination.Number}}
	}
	return nil
}

func (g *Group) openNomination(by MemberID, person Person, candidate Candidate, at time.Time) Nomination {
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

func namedOnce(candidates []Candidate) error {
	seen := map[PersonID]bool{}
	for _, candidate := range candidates {
		if seen[candidate.Person] {
			return Refusal{Rule: OneOpenNominationPerPerson, Ref: Ref{Person: candidate.Person}}
		}
		seen[candidate.Person] = true
	}
	return nil
}
