package domain

import (
	"maps"
	"slices"
	"time"
)

type NominationNumber int

type PhotoStatus string

const (
	NoPhotos        PhotoStatus = ""
	PhotosRequested PhotoStatus = "requested"
	PhotosSearching PhotoStatus = "searching"
	PhotosFound     PhotoStatus = "found"
	PhotosFailed    PhotoStatus = "failed"
)

const exhibitLimit = 2000

type Nomination struct {
	Number      NominationNumber
	Person      Person
	Nominator   MemberID
	Status      CertificationStatus
	Votes       map[MemberID]Vote
	Reactions   map[MemberID]Vote
	NominatedAt time.Time
	LockedAt    time.Time
	FinalizedAt time.Time
	Pitch       string
	Exhibits    []string
	Photos      PhotoStatus
	Strip       string
}

func (n Nomination) Open() bool {
	return n.Status == Nominated
}

func (n Nomination) Locked() bool {
	return !n.LockedAt.IsZero()
}

func (n Nomination) Count(vote Vote) int {
	count := 0
	for _, cast := range n.Votes {
		if cast == vote {
			count++
		}
	}
	return count
}

func (n Nomination) clone() Nomination {
	n.Votes = maps.Clone(n.Votes)
	n.Reactions = maps.Clone(n.Reactions)
	n.Exhibits = slices.Clone(n.Exhibits)
	return n
}

func (n *Nomination) exhibit(text string) {
	if text == "" {
		return
	}
	runes := []rune(text)
	if len(runes) > exhibitLimit {
		runes = runes[:exhibitLimit]
	}
	n.Exhibits = append(n.Exhibits, string(runes))
}

func record(votes *map[MemberID]Vote, member MemberID, vote Vote) {
	if vote == NotVoted {
		delete(*votes, member)
		return
	}
	if *votes == nil {
		*votes = map[MemberID]Vote{}
	}
	(*votes)[member] = vote
}

func cloneAll(nominations []Nomination) []Nomination {
	copies := make([]Nomination, len(nominations))
	for i, nomination := range nominations {
		copies[i] = nomination.clone()
	}
	return copies
}
