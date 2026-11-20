package command

import (
	"context"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type ClaimedWhoSearch struct {
	Number      domain.WhoSearchNumber
	Description string
}

type ClaimWhoSearchesHandler struct {
	Groups port.GroupStore
}

func (h ClaimWhoSearchesHandler) Handle(ctx context.Context) ([]ClaimedWhoSearch, error) {
	var searches []ClaimedWhoSearch
	err := h.Groups.Apply(ctx, "", func(group *domain.Group) ([]port.Reply, error) {
		claimed, err := group.ClaimWhoSearches()
		if err != nil {
			return nil, err
		}
		searches = make([]ClaimedWhoSearch, len(claimed))
		for i, search := range claimed {
			searches[i] = ClaimedWhoSearch{Number: search.Number, Description: search.Description}
		}
		return nil, nil
	})
	return searches, err
}
