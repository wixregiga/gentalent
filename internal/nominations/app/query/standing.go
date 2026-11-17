package query

import (
	"context"

	"gentalent/internal/nominations/port"
)

type StandingHandler struct {
	Reader port.StandingReader
}

func (h StandingHandler) Handle(ctx context.Context) (port.Standing, error) {
	return h.Reader.Standing(ctx)
}
