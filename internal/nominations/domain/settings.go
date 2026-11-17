package domain

import "fmt"

type Settings struct {
	GroupOpenNominationLimit     int
	MemberOpenNominationLimit    int
	SingleMessageNominationLimit int
	AllowRenomination            bool
}

type Limit string

const (
	GroupOpenNominationLimit     Limit = "group open nomination limit"
	MemberOpenNominationLimit    Limit = "member open nomination limit"
	SingleMessageNominationLimit Limit = "single message nomination limit"
)

type LimitBelowOne struct {
	Limit Limit
	Value int
}

func (e LimitBelowOne) Error() string {
	return fmt.Sprintf("the %s is %d; it must be at least 1", e.Limit, e.Value)
}
