package command

import (
	"context"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type CastVote struct {
	MessageID string
	By        domain.MemberID
	Ballots   []domain.Ballot
	At        time.Time
}

type CastVoteHandler struct {
	Groups port.GroupStore
}

func (h CastVoteHandler) Handle(ctx context.Context, cmd CastVote, present port.Present[[]domain.VoteOutcome]) error {
	return h.Groups.Apply(ctx, cmd.MessageID, func(group *domain.Group) ([]port.Reply, error) {
		result, err := group.CastVote(cmd.By, cmd.Ballots, cmd.At)
		return present(group, result, err)
	})
}
