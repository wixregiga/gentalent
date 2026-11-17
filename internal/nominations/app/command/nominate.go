package command

import (
	"context"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type Nominate struct {
	MessageID  string
	By         domain.MemberID
	Candidates []domain.Candidate
	At         time.Time
}

type NominateHandler struct {
	Groups port.GroupStore
}

func (h NominateHandler) Handle(ctx context.Context, cmd Nominate, present port.Present[[]domain.NominateOutcome]) error {
	return h.Groups.Apply(ctx, cmd.MessageID, func(group *domain.Group) ([]port.Reply, error) {
		result, err := group.Nominate(cmd.By, cmd.Candidates, cmd.At)
		return present(group, result, err)
	})
}
