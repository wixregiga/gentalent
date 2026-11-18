package port

import (
	"context"
	"time"

	"gentalent/internal/messaging/domain"
)

type Baseline struct {
	Cursor  string
	ArmedAt time.Time
}

type InboxItem struct {
	Message domain.Incoming
	SortKey string
}

type BaselineStore interface {
	Baseline(context.Context) (Baseline, error)
	Arm(context.Context, Baseline) (bool, error)
}

type PollStore interface {
	Baseline(context.Context) (Baseline, error)
	SavePage(context.Context, string, string, []InboxItem) error
	ReplyTag(ctx context.Context, messageID string) (tag string, known bool, err error)
	KnownAttachment(context.Context, string) (bool, error)
}

type InboxReader interface {
	Next(context.Context) (*domain.Incoming, error)
}

type InboxWriter interface {
	Handled(context.Context, string) error
}

type HistoryReader interface {
	GroupReader
	Messages(context.Context, string) (Page, error)
}
