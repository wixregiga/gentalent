package command

import (
	"context"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type ClaimedPhotoSearch struct {
	Nomination domain.NominationNumber
	Person     string
	Pitch      string
}

type ClaimPhotoSearchesHandler struct {
	Groups port.GroupStore
}

func (h ClaimPhotoSearchesHandler) Handle(ctx context.Context) ([]ClaimedPhotoSearch, error) {
	var searches []ClaimedPhotoSearch
	err := h.Groups.Apply(ctx, "", func(group *domain.Group) ([]port.Reply, error) {
		claimed, err := group.ClaimPhotoSearches()
		if err != nil {
			return nil, err
		}
		searches = make([]ClaimedPhotoSearch, len(claimed))
		for i, nomination := range claimed {
			person, _ := group.Person(nomination.Person)
			searches[i] = ClaimedPhotoSearch{Nomination: nomination.Number, Person: person.Name, Pitch: nomination.Pitch}
		}
		return nil, nil
	})
	return searches, err
}
