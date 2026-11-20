package port

import (
	"context"
	"time"

	"gentalent/internal/nominations/domain"
)

type NominationView struct {
	Number    domain.NominationNumber
	Person    string
	Nominator domain.MemberID
	Status    domain.CertificationStatus
	Votes     map[domain.MemberID]domain.Vote
	LockedAt  time.Time
	Photos    domain.PhotoStatus
}

type Standing struct {
	Open         []NominationView
	Latest       NominationView
	Certified    []string
	Rejected     []string
	Searching    []domain.NominationNumber
	WhoSearching []domain.WhoSearchNumber
}

type StandingReader interface {
	Standing(ctx context.Context) (Standing, error)
}

type Name struct {
	Alias   string
	Person  domain.PersonID
	Display string
	Open    bool
}

type NameReader interface {
	Names(ctx context.Context) ([]Name, error)
}
