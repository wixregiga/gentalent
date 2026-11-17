package command

import (
	"context"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type Renominate struct {
	MessageID  string
	By         domain.MemberID
	Candidates []domain.Candidate
	At         time.Time
}

type RenominateHandler struct {
	Groups port.GroupStore
}

func (h RenominateHandler) Handle(ctx context.Context, cmd Renominate, present port.Present[[]domain.Nomination]) error {
	return h.Groups.Apply(ctx, cmd.MessageID, func(group *domain.Group) ([]port.Reply, error) {
		result, err := group.Renominate(cmd.By, cmd.Candidates, cmd.At)
		return present(group, result, err)
	})
}
