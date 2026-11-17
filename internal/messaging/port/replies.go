package port

import (
	"context"

	"gentalent/internal/messaging/domain"
)

type ReplyWriter interface {
	Enqueue(context.Context, []domain.Outgoing) error
}

type DeliveryStore interface {
	ActiveReplies(context.Context) ([]domain.Reply, error)
	Transition(context.Context, string, domain.State, domain.State, string) (bool, error)
}

type ResolutionStore interface {
	Reply(context.Context, string) (domain.Reply, error)
	FinalIDUsed(context.Context, string, string) (bool, error)
	Transition(context.Context, string, domain.State, domain.State, string) (bool, error)
}

type TaggedReplyReader interface {
	TaggedMessage(context.Context, string) (string, error)
}

type SentReplyReader interface {
	RecentSent(context.Context, int) ([]domain.Reply, error)
}

type ReplyStatusReader interface {
	UncertainReplies(context.Context) (ReplyStatus, error)
}

type ReplyStatus struct {
	Counts    map[domain.State]int
	Attention []domain.Reply
}
