package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"gentalent/internal/messaging/domain"
)

func (s *Store) Enqueue(ctx context.Context, replies []domain.Outgoing) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, reply := range replies {
		kind := "reply"
		if reply.Text == "" {
			kind = "photo"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO outbox(key,text,attachment,attachment_name,tag,kind,status)
			VALUES (?,?,?,?,?,?,'queued') ON CONFLICT(key) DO NOTHING
		`, reply.Key, reply.Text, reply.Attachment, attachmentName(reply.Attachment), reply.Tag, kind); err != nil {
			return fmt.Errorf("enqueue reply %s: %w", reply.Key, err)
		}
	}
	return tx.Commit()
}

func attachmentName(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

const finalIDHeld = "SELECT EXISTS(SELECT 1 FROM outbox WHERE final_id=? AND final_id<>'' AND key<>?)"

func (s *Store) Transition(ctx context.Context, key string, from, to domain.State, id string) (domain.State, error) {
	if !from.Allows(to) {
		return "", fmt.Errorf("invalid reply transition %s to %s", from, to)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if to == domain.Sent {
		var held bool
		if err := tx.QueryRowContext(ctx, finalIDHeld, id, key).Scan(&held); err != nil {
			return "", err
		}
		switch {
		case held && from.Allows(domain.Uncertain):
			to, id = domain.Uncertain, ""
		case held:
			return "", nil
		}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE outbox SET status=?,
		pending_id=CASE WHEN ?='pending' THEN ? ELSE pending_id END,
		final_id=CASE WHEN ? IN ('sent','failed') AND ?<>'' THEN ? ELSE final_id END
		WHERE key=? AND status=?
	`, to, to, id, to, id, id, key, from)
	if err != nil {
		return "", err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return "", err
	}
	return to, tx.Commit()
}

const replyColumns = "key,text,attachment,tag,status,pending_id,final_id,kind"

func (s *Store) ActiveReplies(ctx context.Context) ([]domain.Reply, error) {
	return s.replies(ctx, "SELECT "+replyColumns+" FROM outbox WHERE status IN ('queued','pending') ORDER BY rowid")
}

func (s *Store) Reply(ctx context.Context, key string) (domain.Reply, error) {
	var reply domain.Reply
	err := s.db.QueryRowContext(ctx, "SELECT "+replyColumns+" FROM outbox WHERE key=?", key).
		Scan(&reply.Key, &reply.Text, &reply.Attachment, &reply.Tag, &reply.State, &reply.PendingID, &reply.FinalID, &reply.Kind)
	if err == sql.ErrNoRows {
		return reply, nil
	}
	return reply, err
}

func (s *Store) FinalIDUsed(ctx context.Context, key, id string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, finalIDHeld, id, key).Scan(&exists)
	return exists, err
}

func (s *Store) replies(ctx context.Context, statement string, args ...any) ([]domain.Reply, error) {
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReplies(rows)
}

func scanReplies(rows *sql.Rows) ([]domain.Reply, error) {
	replies := []domain.Reply{}
	for rows.Next() {
		var reply domain.Reply
		if err := rows.Scan(&reply.Key, &reply.Text, &reply.Attachment, &reply.Tag, &reply.State, &reply.PendingID, &reply.FinalID, &reply.Kind); err != nil {
			return nil, err
		}
		replies = append(replies, reply)
	}
	return replies, rows.Err()
}
