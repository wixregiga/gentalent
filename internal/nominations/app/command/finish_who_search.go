package command

import (
	"context"
	"fmt"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type FinishWhoSearch struct {
	Number domain.WhoSearchNumber
	Status domain.WhoSearchStatus
	Figure domain.PublicFigure
}

type FinishWhoSearchHandler struct {
	Groups port.GroupStore
}

func (h FinishWhoSearchHandler) Handle(ctx context.Context, cmd FinishWhoSearch, present port.Present[domain.WhoSearch]) error {
	return h.Groups.Apply(ctx, "", func(group *domain.Group) ([]port.Reply, error) {
		switch cmd.Status {
		case domain.WhoFound:
			found, err := group.ReceivePublicFigure(cmd.Number, cmd.Figure)
			return present(group, found, err)
		case domain.WhoNoMatch:
			none, err := group.FindNoOne(cmd.Number)
			return present(group, none, err)
		case domain.WhoFailed:
			failed, err := group.FailWhoSearch(cmd.Number)
			return present(group, failed, err)
		}
		return nil, fmt.Errorf("who search %d cannot finish as %q", cmd.Number, cmd.Status)
	})
}
