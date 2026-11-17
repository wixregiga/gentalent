package domain

import (
	"errors"
	"time"
)

type ShameID int

type ShamedMessage struct {
	ID     string
	Text   string
	SentAt time.Time
}

type Shame struct {
	ID         ShameID
	Nomination NominationNumber
	By         MemberID
	Target     MemberID
	TargetVote Vote
	Reason     string
	Shamed     ShamedMessage
	GIF        string
	MessageID  string
	At         time.Time
}

func (g *Group) Shame(shame Shame, about Ref) (Shame, error) {
	next := g.clone()
	if err := next.requireMember(shame.By); err != nil {
		return Shame{}, err
	}
	if !next.isMember(shame.Target) {
		return Shame{}, Refusal{Rule: ShameOnlyMembers}
	}
	if shame.Target == shame.By {
		return Shame{}, Refusal{Rule: NoSelfShame}
	}
	i, err := next.shamedOver(about)
	if err != nil {
		return Shame{}, err
	}
	nomination := next.nominations[i]
	if err := next.AssertOneShamePerMemberPerNomination(shame.By, nomination); err != nil {
		return Shame{}, err
	}
	shame.ID = next.nextShameID()
	shame.Nomination = nomination.Number
	shame.TargetVote = nomination.Votes[shame.Target]
	next.shames = append(next.shames, shame)
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes()); err != nil {
		return Shame{}, err
	}
	*g = *next
	return shame, nil
}

func (g *Group) AssertOneShamePerMemberPerNomination(member MemberID, nomination Nomination) error {
	for _, given := range g.shames {
		if given.By == member && given.Nomination == nomination.Number {
			return Refusal{Rule: OneShamePerMemberPerNomination, Numbers: []NominationNumber{nomination.Number}}
		}
	}
	return nil
}

func (g *Group) shamedOver(about Ref) (int, error) {
	switch {
	case about.Number != 0:
		if i, ok := g.index(about.Number); ok {
			return i, nil
		}
		return 0, Refusal{Rule: UnknownNomination, Ref: about}
	case about.Person != "":
		if i, ok := g.latestOf(about.Person); ok {
			return i, nil
		}
		return 0, Refusal{Rule: NothingToShame, Ref: about}
	}
	if open := g.open(); len(open) > 1 {
		return 0, Refusal{Rule: ShameNamesItsNomination, Numbers: numbers(open)}
	}
	if len(g.nominations) == 0 {
		return 0, Refusal{Rule: NothingToShame}
	}
	return len(g.nominations) - 1, nil
}

func (g *Group) nextShameID() ShameID {
	highest := ShameID(0)
	for _, shame := range g.shames {
		highest = max(highest, shame.ID)
	}
	return highest + 1
}
