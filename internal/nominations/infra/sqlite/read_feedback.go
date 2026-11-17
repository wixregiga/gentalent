package sqlite

import (
	"context"
	"fmt"

	"gentalent/internal/nominations/port"
)

func (r Reader) Feedback(ctx context.Context) ([]port.Feedback, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT message_id, sender, kind, text, said_at FROM feedback ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("read feedback: %w", err)
	}
	defer rows.Close()
	var entries []port.Feedback
	for rows.Next() {
		var entry port.Feedback
		var saidAt string
		if err := rows.Scan(&entry.MessageID, &entry.Sender, &entry.Kind, &entry.Text, &saidAt); err != nil {
			return nil, fmt.Errorf("read feedback: %w", err)
		}
		if entry.SaidAt, err = parseTime(saidAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
