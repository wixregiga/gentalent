package query

import (
	"context"

	"gentalent/internal/nominations/port"
)

type FeedbackHandler struct {
	Reader port.FeedbackReader
}

func (h FeedbackHandler) Handle(ctx context.Context) ([]port.Feedback, error) {
	return h.Reader.Feedback(ctx)
}
