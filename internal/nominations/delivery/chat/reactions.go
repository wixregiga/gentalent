package chat

import (
	"context"
	"fmt"
	"strings"

	"gentalent/internal/nominations/app/command"
	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

var reactionVotes = map[string]domain.Vote{"👍": domain.Yes, "👎": domain.No}

var unstyled = strings.NewReplacer("\U0001F3FB", "", "\U0001F3FC", "", "\U0001F3FD", "", "\U0001F3FE", "", "\U0001F3FF", "", "\uFE0E", "", "\uFE0F", "")

func (b *Bot) CountReactions(ctx context.Context, nomination int, byMember map[string][]string) error {
	votes := map[domain.MemberID]domain.Vote{}
	for sender, keys := range byMember {
		member, ok := b.member(sender)
		if !ok {
			continue
		}
		if vote, single := reactionVote(keys); single {
			votes[member.ID] = vote
		}
	}
	at := b.config.Now()
	cmd := command.CountReactions{Nomination: domain.NominationNumber(nomination), Reactions: votes, At: at}
	key := eventKey(fmt.Sprintf("reactions/%d", nomination), at)
	return b.reactions.Handle(ctx, cmd, present(b, key, nil, func(w wording, outcomes []domain.VoteOutcome) []port.Reply {
		if !b.config.AnnounceReactionVotes {
			return nil
		}
		return w.voteReplies(outcomes)
	}))
}

func (b *Bot) OpenAnnouncements(ctx context.Context) ([]int, error) {
	standing, err := b.standing.Handle(ctx)
	if err != nil {
		return nil, err
	}
	numbers := make([]int, len(standing.Open))
	for i, open := range standing.Open {
		numbers[i] = int(open.Number)
	}
	return numbers, nil
}

func reactionVote(keys []string) (domain.Vote, bool) {
	found := domain.NotVoted
	for _, key := range keys {
		vote := reactionVotes[unstyled.Replace(key)]
		if vote == domain.NotVoted {
			continue
		}
		if found != domain.NotVoted && found != vote {
			return domain.NotVoted, false
		}
		found = vote
	}
	return found, found != domain.NotVoted
}
