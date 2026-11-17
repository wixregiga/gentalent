package port

import "context"

type Outbox interface {
	Answer(ctx context.Context, messageID string, replies []Reply) error
	MarkRelayed(ctx context.Context, keys []string) error
}

type OutboxReader interface {
	Unrelayed(ctx context.Context) ([]Reply, error)
}
