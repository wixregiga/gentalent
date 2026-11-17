package query

import (
	"context"

	"gentalent/internal/nominations/port"
)

type UnrelayedHandler struct {
	Reader port.OutboxReader
}

func (h UnrelayedHandler) Handle(ctx context.Context) ([]port.Reply, error) {
	return h.Reader.Unrelayed(ctx)
}
