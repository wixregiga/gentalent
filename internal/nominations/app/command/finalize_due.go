package command

import (
	"context"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type FinalizeDue struct {
	Now time.Time
}

type FinalizeDueHandler struct {
	Groups port.GroupStore
}

func (h FinalizeDueHandler) Handle(ctx context.Context, cmd FinalizeDue, present port.Present[[]domain.Nomination]) error {
	return h.Groups.Apply(ctx, "", func(group *domain.Group) ([]port.Reply, error) {
		finalized, err := group.FinalizeDue(cmd.Now)
		return present(group, finalized, err)
	})
}
