package domain

import (
	"errors"
	"fmt"
)

func (g *Group) ClaimPhotoSearches() ([]Nomination, error) {
	next := g.clone()
	var claimed []Nomination
	for i := range next.nominations {
		if nomination := &next.nominations[i]; nomination.Photos == PhotosRequested {
			nomination.Photos = PhotosSearching
			claimed = append(claimed, nomination.clone())
		}
	}
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return nil, err
	}
	*g = *next
	return claimed, nil
}

func (g *Group) AttachPhotos(number NominationNumber, strip, exhibit string) (Nomination, error) {
	next, i, err := g.searching(number)
	if err != nil {
		return Nomination{}, err
	}
	nomination := &next.nominations[i]
	nomination.Photos = PhotosFound
	nomination.Strip = strip
	nomination.exhibit(exhibit)
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return Nomination{}, err
	}
	*g = *next
	return nomination.clone(), nil
}

func (g *Group) FailPhotoSearch(number NominationNumber) (Nomination, error) {
	next, i, err := g.searching(number)
	if err != nil {
		return Nomination{}, err
	}
	nomination := &next.nominations[i]
	nomination.Photos = PhotosFailed
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return Nomination{}, err
	}
	*g = *next
	return nomination.clone(), nil
}

func (g *Group) AbandonInterruptedSearches() ([]Nomination, error) {
	next := g.clone()
	var abandoned []Nomination
	for i := range next.nominations {
		if nomination := &next.nominations[i]; nomination.Photos == PhotosSearching {
			nomination.Photos = PhotosFailed
			abandoned = append(abandoned, nomination.clone())
		}
	}
	if err := errors.Join(next.EnsureNominationLimitNotLessThanOne(), next.EnsureOneOpenNominationPerPerson(), next.EnsureCertifiedByUnanimousYes(),
		next.EnsureOnePendingNameFixPerPerson(), next.EnsureNameFixAgreementPercentageWithinRange(), next.EnsureNameFixWindowAtLeastOneHour()); err != nil {
		return nil, err
	}
	*g = *next
	return abandoned, nil
}

func (g *Group) searching(number NominationNumber) (*Group, int, error) {
	next := g.clone()
	i, ok := next.index(number)
	if !ok || next.nominations[i].Photos != PhotosSearching {
		return nil, 0, fmt.Errorf("nomination %d has no photo search running", number)
	}
	return next, i, nil
}
