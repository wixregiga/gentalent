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

func (s *Store) KnownReplyID(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM outbox WHERE (pending_id=? AND pending_id<>'') OR (final_id=? AND final_id<>''))
	`, id, id).Scan(&exists)
	return exists, err
}

func (s *Store) KnownAttachment(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM outbox WHERE attachment_name=? AND attachment_name<>'')
	`, name).Scan(&exists)
	return exists, err
}
