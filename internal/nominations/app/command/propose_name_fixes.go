package command

import (
	"context"
	"errors"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type ProposeNameFixes struct {
	MessageID string
	By        domain.MemberID
	Proposals []domain.NameFixProposal
	At        time.Time
}

type NameFixAttempt struct {
	Proposal domain.NameFixProposal
	Fix      domain.NameFixNumber
	Refusal  *domain.Refusal
}

type ProposeNameFixesHandler struct {
	Groups port.GroupStore
}

func (h ProposeNameFixesHandler) Handle(ctx context.Context, cmd ProposeNameFixes, present port.Present[[]NameFixAttempt]) error {
	return h.Groups.Apply(ctx, cmd.MessageID, func(group *domain.Group) ([]port.Reply, error) {
		attempts := make([]NameFixAttempt, 0, len(cmd.Proposals))
		for _, proposal := range cmd.Proposals {
			fix, err := group.ProposeNameFix(cmd.By, proposal, cmd.At)
			var refusal domain.Refusal
			switch {
			case errors.As(err, &refusal):
				attempts = append(attempts, NameFixAttempt{Proposal: proposal, Refusal: &refusal})
			case err != nil:
				return present(group, nil, err)
			default:
				attempts = append(attempts, NameFixAttempt{Proposal: proposal, Fix: fix.Number})
			}
		}
		return present(group, attempts, nil)
	})
}
