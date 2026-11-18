package query

import (
	"context"

	"gentalent/internal/nominations/port"
)

type NominationsHandler struct {
	Reader port.NominationReader
}

func (h NominationsHandler) Handle(ctx context.Context) ([]port.NominationView, error) {
	return h.Reader.Nominations(ctx)
}
