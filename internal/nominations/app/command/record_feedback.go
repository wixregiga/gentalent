package command

import (
	"context"

	"gentalent/internal/nominations/port"
)

type RecordFeedbackHandler struct {
	Feedback port.FeedbackStore
}

func (h RecordFeedbackHandler) Handle(ctx context.Context, entry port.Feedback, replies []port.Reply) error {
	return h.Feedback.Log(ctx, entry, replies)
}
