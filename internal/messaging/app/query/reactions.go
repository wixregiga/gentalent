package query

import (
	"context"
	"fmt"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type ReactionReader struct {
	Replies  port.TaggedReplyReader
	Platform port.MessageReader
	Group    domain.Group
}

func (r ReactionReader) Reactions(ctx context.Context, tag string) (map[string][]string, error) {
	id, err := r.Replies.TaggedMessage(ctx, tag)
	if err != nil || id == "" {
		return nil, err
	}
	message, err := r.Platform.Retrieve(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reaction check skipped: %w", err)
	}
	if message.ID != id {
		return nil, fmt.Errorf("reaction check returned a different message")
	}
	bySender := map[string][]string{}
	for _, reaction := range message.Reactions {
		if reaction.Key != "" && r.Group.HasMember(reaction.Sender) {
			bySender[reaction.Sender] = append(bySender[reaction.Sender], reaction.Key)
		}
	}
	return bySender, nil
}
