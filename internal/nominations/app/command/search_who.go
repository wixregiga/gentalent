package command

import (
	"context"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type SearchWho struct {
	MessageID   string
	By          domain.MemberID
	Description string
	At          time.Time
}

type SearchWhoHandler struct {
	Groups port.GroupStore
}

func (h SearchWhoHandler) Handle(ctx context.Context, cmd SearchWho, present port.Present[domain.WhoSearch]) error {
	return h.Groups.Apply(ctx, cmd.MessageID, func(group *domain.Group) ([]port.Reply, error) {
		search, err := group.SearchWho(cmd.By, cmd.Description, cmd.At)
		return present(group, search, err)
	})
}
