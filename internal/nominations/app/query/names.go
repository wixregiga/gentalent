package query

import (
	"context"

	"gentalent/internal/nominations/port"
)

type NamesHandler struct {
	Reader port.NameReader
}

func (h NamesHandler) Handle(ctx context.Context) ([]port.Name, error) {
	return h.Reader.Names(ctx)
}
