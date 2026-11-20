package command

import (
	"context"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type AbandonInterruptedWhoSearchesHandler struct {
	Groups port.GroupStore
}

func (h AbandonInterruptedWhoSearchesHandler) Handle(ctx context.Context, present port.Present[[]domain.WhoSearch]) error {
	return h.Groups.Apply(ctx, "", func(group *domain.Group) ([]port.Reply, error) {
		abandoned, err := group.AbandonInterruptedWhoSearches()
		return present(group, abandoned, err)
	})
}
