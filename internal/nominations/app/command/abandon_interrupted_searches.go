package command

import (
	"context"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type AbandonInterruptedSearchesHandler struct {
	Groups port.GroupStore
}

func (h AbandonInterruptedSearchesHandler) Handle(ctx context.Context, present port.Present[[]domain.Nomination]) error {
	return h.Groups.Apply(ctx, "", func(group *domain.Group) ([]port.Reply, error) {
		abandoned, err := group.AbandonInterruptedSearches()
		return present(group, abandoned, err)
	})
}
