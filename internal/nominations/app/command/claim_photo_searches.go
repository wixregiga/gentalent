package command

import (
	"context"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type ClaimPhotoSearchesHandler struct {
	Groups port.GroupStore
}

func (h ClaimPhotoSearchesHandler) Handle(ctx context.Context) ([]domain.Nomination, error) {
	var claimed []domain.Nomination
	err := h.Groups.Apply(ctx, "", func(group *domain.Group) ([]port.Reply, error) {
		var err error
		claimed, err = group.ClaimPhotoSearches()
		return nil, err
	})
	return claimed, err
}
