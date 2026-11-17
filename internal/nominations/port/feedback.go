package port

import (
	"context"
	"time"

	"gentalent/internal/nominations/domain"
)

type Feedback struct {
	MessageID string
	Sender    domain.MemberID
	Kind      string
	Text      string
	SaidAt    time.Time
}

type FeedbackStore interface {
	Log(ctx context.Context, entry Feedback, replies []Reply) error
}

type FeedbackReader interface {
	Feedback(ctx context.Context) ([]Feedback, error)
}
