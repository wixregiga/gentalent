package command

import (
	"context"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type CountReactions struct {
	Nomination domain.NominationNumber
	Reactions  map[domain.MemberID]domain.Vote
	At         time.Time
}

type CountReactionsHandler struct {
	Groups port.GroupStore
}

func (h CountReactionsHandler) Handle(ctx context.Context, cmd CountReactions, present port.Present[[]domain.VoteOutcome]) error {
	return h.Groups.Apply(ctx, "", func(group *domain.Group) ([]port.Reply, error) {
		outcomes, err := group.CountReactions(cmd.Nomination, cmd.Reactions, cmd.At)
		return present(group, outcomes, err)
	})
}
