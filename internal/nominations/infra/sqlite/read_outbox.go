package sqlite

import (
	"context"
	"fmt"

	"gentalent/internal/nominations/port"
)

func (r Reader) Unrelayed(ctx context.Context) ([]port.Reply, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT key, text, attachment, announces FROM outbox WHERE relayed = 0 ORDER BY seq")
	if err != nil {
		return nil, fmt.Errorf("read unrelayed replies: %w", err)
	}
	defer rows.Close()
	var replies []port.Reply
	for rows.Next() {
		var reply port.Reply
		if err := rows.Scan(&reply.Key, &reply.Text, &reply.Attachment, &reply.Announces); err != nil {
			return nil, fmt.Errorf("read unrelayed replies: %w", err)
		}
		replies = append(replies, reply)
	}
	return replies, rows.Err()
}
