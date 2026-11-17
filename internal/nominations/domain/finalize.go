package domain

import (
	"errors"
	"slices"
	"time"
)

const ResultDelay = time.Minute

func (g *Group) Finalize(by MemberID, refs []Ref, at time.Time) ([]Nomination, error) {
	next := g.clone()
	if err := next.requireMember(by); err != nil {
		return nil, err
	}
	chosen, err := next.finalizing(by, refs)
	if err != nil || len(chosen) == 0 {
		return nil, err
	}
	if err := next.AssertFinalizedOnlyWhenAllVotesAreIn(chosen); err != nil {
		return nil, err
	}
	if err := next.AssertFinalizedEarlyOnlyByNominator(by, chosen); err != nil {
		return nil, err
	}
	finalized := next.finalize(chosen, at)
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes()); err != nil {
		return nil, err
	}
	*g = *next
	return finalized, nil
}

func (g *Group) FinalizeDue(now time.Time) ([]Nomination, error) {
	next := g.clone()
	var due []Nomination
	for _, nomination := range next.open() {
		if nomination.Locked() && next.AssertFinalizedAMinuteAfterTheLastVote(nomination, now) == nil {
			due = append(due, nomination)
		}
	}
	finalized := next.finalize(due, now)
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes()); err != nil {
		return nil, err
	}
	*g = *next
	return finalized, nil
}

func (g *Group) AssertFinalizedOnlyWhenAllVotesAreIn(nominations []Nomination) error {
	var waiting []NominationNumber
	for _, nomination := range nominations {
		if !nomination.Locked() {
			waiting = append(waiting, nomination.Number)
		}
	}
	if len(waiting) > 0 {
		return Refusal{Rule: FinalizedOnlyWhenAllVotesAreIn, Numbers: waiting}
	}
	return nil
}

func (g *Group) AssertFinalizedEarlyOnlyByNominator(member MemberID, nominations []Nomination) error {
	var others []NominationNumber
	for _, nomination := range nominations {
		if nomination.Nominator != member {
			others = append(others, nomination.Number)
		}
	}
	if len(others) > 0 {
		return Refusal{Rule: FinalizedEarlyOnlyByNominator, Numbers: others}
	}
	return nil
}

func (g *Group) AssertFinalizedAMinuteAfterTheLastVote(nomination Nomination, now time.Time) error {
	if !nomination.Locked() || now.Sub(nomination.LockedAt) < ResultDelay {
		return Refusal{Rule: FinalizedAMinuteAfterTheLastVote, Numbers: []NominationNumber{nomination.Number}}
	}
	return nil
}

func (g *Group) finalizing(by MemberID, refs []Ref) ([]Nomination, error) {
	if len(refs) == 0 {
		return g.finalizedByDefault(by), nil
	}
	var chosen []Nomination
	for _, ref := range refs {
		i, _, err := g.openFor(ref)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(chosen, func(n Nomination) bool { return n.Number == g.nominations[i].Number }) {
			chosen = append(chosen, g.nominations[i])
		}
	}
	return chosen, nil
}

func (g *Group) finalizedByDefault(by MemberID) []Nomination {
	mine := g.openOf(by)
	if len(mine) == 0 {
		if locked := lockedAmong(g.open()); len(locked) > 0 {
			return locked
		}
		return g.open()
	}
	if locked := lockedAmong(mine); len(locked) > 0 {
		return locked
	}
	return mine
}

func (g *Group) finalize(nominations []Nomination, at time.Time) []Nomination {
	finalized := make([]Nomination, 0, len(nominations))
	for _, chosen := range nominations {
		i, _ := g.index(chosen.Number)
		nomination := &g.nominations[i]
		nomination.Status = Rejected
		if g.unanimous(*nomination) {
			nomination.Status = Certified
		}
		nomination.FinalizedAt = at
		finalized = append(finalized, nomination.clone())
	}
	slices.SortFunc(finalized, func(a, b Nomination) int { return int(a.Number - b.Number) })
	return finalized
}

func lockedAmong(nominations []Nomination) []Nomination {
	var locked []Nomination
	for _, nomination := range nominations {
		if nomination.Locked() {
			locked = append(locked, nomination)
		}
	}
	return locked
}
