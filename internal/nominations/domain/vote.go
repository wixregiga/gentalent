package domain

import (
	"errors"
	"slices"
	"time"
)

type Vote string

const (
	NotVoted Vote = ""
	Yes      Vote = "yes"
	No       Vote = "no"
)

type Ballot struct {
	Nomination Ref
	Vote       Vote
}

type VoteChange string

const (
	VoteRecorded VoteChange = "recorded"
	VoteChanged  VoteChange = "changed"
	VoteCleared  VoteChange = "cleared"
)

type VoteOutcome struct {
	Number          NominationNumber
	Member          MemberID
	Change          VoteChange
	Locks           bool
	ByCloseSpelling bool
}

func (g *Group) CastVote(by MemberID, ballots []Ballot, at time.Time) ([]VoteOutcome, error) {
	next := g.clone()
	if err := next.requireMember(by); err != nil {
		return nil, err
	}
	var outcomes []VoteOutcome
	for _, ballot := range ballots {
		if err := next.AssertVoteNamesItsNomination(ballot.Nomination); err != nil {
			return nil, err
		}
		if err := next.AssertCloseSpellingNamesOnlyAnOpenNomination(ballot.Nomination); err != nil {
			return nil, err
		}
		if err := next.AssertCloseSpellingNamesOneNomination(ballot.Nomination); err != nil {
			return nil, err
		}
		i, found, err := next.openFor(ballot.Nomination)
		if err != nil {
			return nil, err
		}
		if !found || next.nominations[i].Votes[by] == ballot.Vote {
			continue
		}
		if err := next.AssertVotesChangeUntilTheThirdVote(next.nominations[i]); err != nil {
			return nil, err
		}
		outcome := next.vote(i, by, ballot.Vote, at)
		outcome.ByCloseSpelling = byCloseSpelling(ballot.Nomination)
		outcomes = append(outcomes, outcome)
	}
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return nil, err
	}
	*g = *next
	return outcomes, nil
}

func (g *Group) CountReactions(number NominationNumber, reactions map[MemberID]Vote, at time.Time) ([]VoteOutcome, error) {
	next := g.clone()
	i, ok := next.index(number)
	if !ok || !next.nominations[i].Open() {
		return nil, nil
	}
	var outcomes []VoteOutcome
	for _, member := range next.members {
		nomination := &next.nominations[i]
		before, after := nomination.Reactions[member.ID], reactions[member.ID]
		if before == after {
			continue
		}
		record(&nomination.Reactions, member.ID, after)
		if after == NotVoted && next.AssertRemovedReactionClearsOnlyItsVote(*nomination, member.ID, before) != nil {
			continue
		}
		if nomination.Votes[member.ID] == after || next.AssertVotesChangeUntilTheThirdVote(*nomination) != nil {
			continue
		}
		outcomes = append(outcomes, next.vote(i, member.ID, after, at))
	}
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return nil, err
	}
	*g = *next
	return outcomes, nil
}

func (g *Group) AssertVoteNamesItsNomination(ref Ref) error {
	if open := g.open(); ref.Unnamed() && len(open) > 1 {
		return Refusal{Rule: VoteNamesItsNomination, Numbers: numbers(open)}
	}
	return nil
}

func (g *Group) AssertCloseSpellingNamesOnlyAnOpenNomination(ref Ref) error {
	if byCloseSpelling(ref) && len(g.closeMatches(ref)) == 0 {
		return Refusal{Rule: CloseSpellingNamesOnlyAnOpenNomination, Ref: ref}
	}
	return nil
}

func (g *Group) AssertCloseSpellingNamesOneNomination(ref Ref) error {
	if matches := g.closeMatches(ref); byCloseSpelling(ref) && len(matches) > 1 {
		fits := make([]NominationNumber, len(matches))
		for k, i := range matches {
			fits[k] = g.nominations[i].Number
		}
		return Refusal{Rule: CloseSpellingNamesOneNomination, Numbers: fits}
	}
	return nil
}

func (g *Group) AssertVotesChangeUntilTheThirdVote(nomination Nomination) error {
	if nomination.Locked() {
		return Refusal{Rule: VotesChangeUntilTheThirdVote, Numbers: []NominationNumber{nomination.Number}}
	}
	return nil
}

func (g *Group) AssertRemovedReactionClearsOnlyItsVote(nomination Nomination, member MemberID, removed Vote) error {
	if nomination.Votes[member] != removed {
		return Refusal{Rule: RemovedReactionClearsOnlyItsVote, Numbers: []NominationNumber{nomination.Number}}
	}
	return nil
}

func (g *Group) vote(i int, member MemberID, vote Vote, at time.Time) VoteOutcome {
	nomination := &g.nominations[i]
	change := VoteRecorded
	switch {
	case vote == NotVoted:
		change = VoteCleared
	case nomination.Votes[member] != NotVoted:
		change = VoteChanged
	}
	record(&nomination.Votes, member, vote)
	locks := change == VoteRecorded && len(nomination.Votes) == len(g.members)
	if locks {
		nomination.LockedAt = at
	}
	return VoteOutcome{Number: nomination.Number, Member: member, Change: change, Locks: locks}
}

func (g *Group) openFor(ref Ref) (int, bool, error) {
	switch {
	case ref.Number != 0:
		i, ok := g.index(ref.Number)
		if !ok {
			return 0, false, Refusal{Rule: UnknownNomination, Ref: ref}
		}
		if !g.nominations[i].Open() {
			return 0, false, Refusal{Rule: NominationClosed, Numbers: []NominationNumber{ref.Number}, Ref: ref}
		}
		return i, true, nil
	case ref.Person != "":
		i, ok := g.openIndexOf(ref.Person)
		if !ok {
			return 0, false, Refusal{Rule: NoOpenNomination, Ref: ref}
		}
		return i, true, nil
	case len(ref.Close) > 0:
		matches := g.closeMatches(ref)
		if len(matches) != 1 {
			return 0, false, errors.Join(g.AssertCloseSpellingNamesOnlyAnOpenNomination(ref), g.AssertCloseSpellingNamesOneNomination(ref))
		}
		return matches[0], true, nil
	}
	open := g.open()
	if len(open) == 0 {
		return 0, false, nil
	}
	i, _ := g.index(open[0].Number)
	return i, true, nil
}

func byCloseSpelling(ref Ref) bool {
	return ref.Number == 0 && ref.Person == "" && len(ref.Close) > 0
}

func (g *Group) closeMatches(ref Ref) []int {
	var matches []int
	for _, person := range ref.Close {
		if i, ok := g.openIndexOf(person); ok {
			matches = append(matches, i)
		}
	}
	slices.Sort(matches)
	return slices.Compact(matches)
}
