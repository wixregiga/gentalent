package command

import (
	"context"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type ExpireNameFixes struct {
	Now time.Time
}

type ExpireNameFixesHandler struct {
	Groups port.GroupStore
}

func (h ExpireNameFixesHandler) Handle(ctx context.Context, cmd ExpireNameFixes, present port.Present[[]domain.NameFix]) error {
	return h.Groups.Apply(ctx, "", func(group *domain.Group) ([]port.Reply, error) {
		lapsed, err := group.ExpireNameFixes(cmd.Now)
		return present(group, lapsed, err)
	})
}
