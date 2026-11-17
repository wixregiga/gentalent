package command

import (
	"context"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type Finalize struct {
	MessageID string
	By        domain.MemberID
	Named     []domain.Ref
	At        time.Time
}

type FinalizeHandler struct {
	Groups port.GroupStore
}

func (h FinalizeHandler) Handle(ctx context.Context, cmd Finalize, present port.Present[[]domain.Nomination]) error {
	return h.Groups.Apply(ctx, cmd.MessageID, func(group *domain.Group) ([]port.Reply, error) {
		result, err := group.Finalize(cmd.By, cmd.Named, cmd.At)
		return present(group, result, err)
	})
}
