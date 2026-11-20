package domain

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

type WhoSearchNumber int

type WhoSearchStatus string

const (
	WhoRequested WhoSearchStatus = "requested"
	WhoSearching WhoSearchStatus = "searching"
	WhoFound     WhoSearchStatus = "found"
	WhoNoMatch   WhoSearchStatus = "no-match"
	WhoFailed    WhoSearchStatus = "failed"
)

const WhoSearchDay = 24 * time.Hour

type PublicFigure struct {
	Name     string
	Spelling Spelling
	Blurb    string
	Source   string
	Photo    string
	Provider string
	Person   PersonID
}

type WhoSearch struct {
	Number      WhoSearchNumber
	By          MemberID
	Description string
	StartedAt   time.Time
	Status      WhoSearchStatus
	Figure      PublicFigure
}

func (g *Group) WhoSearches() []WhoSearch {
	return slices.Clone(g.whoSearches)
}

func (g *Group) WhoSearch(number WhoSearchNumber) (WhoSearch, bool) {
	if i, ok := g.whoIndex(number); ok {
		return g.whoSearches[i], true
	}
	return WhoSearch{}, false
}

func (g *Group) SearchWho(by MemberID, description string, at time.Time) (WhoSearch, error) {
	next := g.clone()
	if err := next.requireMember(by); err != nil {
		return WhoSearch{}, err
	}
	if description == "" {
		return WhoSearch{}, errors.New("a who search needs a description")
	}
	if err := next.AssertWhoSearchesWithinDailyLimit(by, at); err != nil {
		return WhoSearch{}, err
	}
	search := WhoSearch{Number: next.nextWhoNumber(), By: by, Description: description, StartedAt: at, Status: WhoRequested}
	next.whoSearches = append(next.whoSearches, search)
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return WhoSearch{}, err
	}
	*g = *next
	return search, nil
}

func (g *Group) ClaimWhoSearches() ([]WhoSearch, error) {
	next := g.clone()
	var claimed []WhoSearch
	for i := range next.whoSearches {
		if search := &next.whoSearches[i]; search.Status == WhoRequested {
			search.Status = WhoSearching
			claimed = append(claimed, *search)
		}
	}
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return nil, err
	}
	*g = *next
	return claimed, nil
}

func (g *Group) ReceivePublicFigure(number WhoSearchNumber, figure PublicFigure) (WhoSearch, error) {
	next, i, err := g.whoSearching(number)
	if err != nil {
		return WhoSearch{}, err
	}
	if figure.Name == "" || figure.Spelling == "" {
		return WhoSearch{}, fmt.Errorf("who search %d received a public figure with no name", number)
	}
	figure.Person, _ = next.spelled(figure.Spelling)
	if err := next.AssertPublicFigureSpelledAsAPersonIsThatPerson(figure); err != nil {
		return WhoSearch{}, err
	}
	search := &next.whoSearches[i]
	search.Status = WhoFound
	search.Figure = figure
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return WhoSearch{}, err
	}
	*g = *next
	return *search, nil
}

func (g *Group) FindNoOne(number WhoSearchNumber) (WhoSearch, error) {
	next, i, err := g.whoSearching(number)
	if err != nil {
		return WhoSearch{}, err
	}
	search := &next.whoSearches[i]
	search.Status = WhoNoMatch
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return WhoSearch{}, err
	}
	*g = *next
	return *search, nil
}

func (g *Group) FailWhoSearch(number WhoSearchNumber) (WhoSearch, error) {
	next, i, err := g.whoSearching(number)
	if err != nil {
		return WhoSearch{}, err
	}
	search := &next.whoSearches[i]
	search.Status = WhoFailed
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return WhoSearch{}, err
	}
	*g = *next
	return *search, nil
}

func (g *Group) AbandonInterruptedWhoSearches() ([]WhoSearch, error) {
	next := g.clone()
	var abandoned []WhoSearch
	for i := range next.whoSearches {
		if search := &next.whoSearches[i]; search.Status == WhoSearching {
			search.Status = WhoFailed
			abandoned = append(abandoned, *search)
		}
	}
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return nil, err
	}
	*g = *next
	return abandoned, nil
}

func (g *Group) AssertWhoSearchesWithinDailyLimit(by MemberID, at time.Time) error {
	var counted []time.Time
	for _, search := range g.whoSearches {
		if search.By == by && at.Sub(search.StartedAt) < WhoSearchDay {
			counted = append(counted, search.StartedAt)
		}
	}
	limit := g.settings.WhoSearchDailyLimit
	if len(counted) < limit {
		return nil
	}
	refusal := Refusal{Rule: WhoSearchesWithinDailyLimit, Limit: limit}
	if limit > 0 {
		slices.SortFunc(counted, time.Time.Compare)
		refusal.Next = counted[len(counted)-limit].Add(WhoSearchDay)
	}
	return refusal
}

func (g *Group) AssertPublicFigureSpelledAsAPersonIsThatPerson(figure PublicFigure) error {
	if named, _ := g.spelled(figure.Spelling); named != figure.Person {
		return Refusal{Rule: PublicFigureSpelledAsAPersonIsThatPerson, Ref: Ref{Person: named, Spelling: figure.Spelling}}
	}
	return nil
}

func (g *Group) whoSearching(number WhoSearchNumber) (*Group, int, error) {
	next := g.clone()
	i, ok := next.whoIndex(number)
	if !ok || next.whoSearches[i].Status != WhoSearching {
		return nil, 0, fmt.Errorf("who search %d is not running", number)
	}
	return next, i, nil
}

func (g *Group) whoIndex(number WhoSearchNumber) (int, bool) {
	return slices.BinarySearchFunc(g.whoSearches, number, func(s WhoSearch, target WhoSearchNumber) int {
		return int(s.Number - target)
	})
}

func (g *Group) nextWhoNumber() WhoSearchNumber {
	if len(g.whoSearches) == 0 {
		return 1
	}
	return g.whoSearches[len(g.whoSearches)-1].Number + 1
}
