package command

import (
	"context"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type AnswerNameFix struct {
	MessageID string
	By        domain.MemberID
	On        domain.NominationNumber
	Agrees    bool
	At        time.Time
}

type AnswerNameFixHandler struct {
	Groups port.GroupStore
}

func (h AnswerNameFixHandler) Handle(ctx context.Context, cmd AnswerNameFix, present port.Present[domain.NameFix]) error {
	return h.Groups.Apply(ctx, cmd.MessageID, func(group *domain.Group) ([]port.Reply, error) {
		if cmd.Agrees {
			fix, err := group.AgreeToNameFix(cmd.By, cmd.On, cmd.At)
			return present(group, fix, err)
		}
		fix, err := group.StopNameFix(cmd.By, cmd.On, cmd.At)
		return present(group, fix, err)
	})
}
