package query

import (
	"context"

	"gentalent/internal/messaging/port"
)

type Discovery struct {
	port.DiscoveryReader
}

func (d Discovery) ListGroups(ctx context.Context, account string, limit int) (port.ChatPage, error) {
	return d.DiscoveryReader.ListGroups(ctx, account, min(max(1, limit), 200))
}
