package domain

import (
	"fmt"
	"time"
)

type Settings struct {
	GroupOpenNominationLimit     int
	MemberOpenNominationLimit    int
	SingleMessageNominationLimit int
	AllowRenomination            bool
	AllowNameFixes               bool
	NameFixAgreementPercentage   int
	NameFixWindow                time.Duration
	WhoSearchDailyLimit          int
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

type AgreementPercentageOutOfRange struct {
	Value int
}

func (e AgreementPercentageOutOfRange) Error() string {
	return fmt.Sprintf("the name fix agreement percentage is %d; it must be more than 0 and at most 100", e.Value)
}

type NameFixWindowBelowAnHour struct {
	Value time.Duration
}

func (e NameFixWindowBelowAnHour) Error() string {
	return fmt.Sprintf("the name fix window is %s; it must be at least one hour", e.Value)
}
