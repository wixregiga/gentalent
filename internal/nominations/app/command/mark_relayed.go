package command

import (
	"context"

	"gentalent/internal/nominations/port"
)

type MarkRelayedHandler struct {
	Outbox port.Outbox
}

func (h MarkRelayedHandler) Handle(ctx context.Context, keys []string) error {
	return h.Outbox.MarkRelayed(ctx, keys)
}
