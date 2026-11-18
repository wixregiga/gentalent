package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"gentalent/internal/messaging/domain"
)

func (s *Store) Next(ctx context.Context) (*domain.Incoming, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, "SELECT payload FROM inbox ORDER BY sent_at,sort_key,rowid LIMIT 1").Scan(&payload)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var next domain.Incoming
	if err := json.Unmarshal([]byte(payload), &next); err != nil {
		return nil, err
	}
	return &next, nil
}

func (s *Store) ReplyTag(ctx context.Context, messageID string) (string, bool, error) {
	var tag string
	err := s.db.QueryRowContext(ctx, `
		SELECT tag FROM outbox WHERE (pending_id=? AND pending_id<>'') OR (final_id=? AND final_id<>'') ORDER BY rowid LIMIT 1
	`, messageID, messageID).Scan(&tag)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return tag, err == nil, err
}

func (s *Store) KnownAttachment(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM outbox WHERE attachment_name=? AND attachment_name<>'')
	`, name).Scan(&exists)
	return exists, err
}
