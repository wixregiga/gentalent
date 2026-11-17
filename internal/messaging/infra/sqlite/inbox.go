package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"gentalent/internal/messaging/port"
)

func (s *Store) SavePage(ctx context.Context, before, after string, items []port.InboxItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE meta SET value=? WHERE key='cursor' AND value=?", after, before)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("poll cursor changed while fetching")
	}
	for _, item := range items {
		payload, err := json.Marshal(item.Message)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO inbox(id,sent_at,sort_key,payload)
			SELECT ?,?,?,? WHERE NOT EXISTS (SELECT 1 FROM processed WHERE message_id=?)
			ON CONFLICT(id) DO NOTHING
		`, item.Message.ID, item.Message.SentAt.UnixNano(), item.SortKey, string(payload), item.Message.ID)
		if err != nil {
			return fmt.Errorf("store inbox message %s: %w", item.Message.ID, err)
		}
	}
	return tx.Commit()
}

func (s *Store) Handled(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO processed(message_id) SELECT id FROM inbox WHERE id=?
		ON CONFLICT(message_id) DO NOTHING
	`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM inbox WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}
