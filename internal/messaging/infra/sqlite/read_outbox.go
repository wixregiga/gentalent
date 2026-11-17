package sqlite

import (
	"context"
	"database/sql"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

func (s *Store) TaggedMessage(ctx context.Context, tag string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
		SELECT final_id FROM outbox WHERE tag=? AND tag<>'' AND status='sent' AND final_id<>''
	`, tag).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (s *Store) RecentSent(ctx context.Context, limit int) ([]domain.Reply, error) {
	return s.replies(ctx, "SELECT "+replyColumns+" FROM outbox WHERE status='sent' AND final_id<>'' ORDER BY rowid DESC LIMIT ?", limit)
}

func (s *Store) UncertainReplies(ctx context.Context) (port.ReplyStatus, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return port.ReplyStatus{}, err
	}
	defer tx.Rollback()
	report := port.ReplyStatus{Counts: map[domain.State]int{}, Attention: []domain.Reply{}}
	if err := readCounts(ctx, tx, report.Counts); err != nil {
		return port.ReplyStatus{}, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT key,substr(text,1,80),attachment,tag,status,pending_id,final_id,kind FROM outbox
		WHERE status IN ('queued','sending','pending','uncertain') ORDER BY rowid
	`)
	if err != nil {
		return port.ReplyStatus{}, err
	}
	defer rows.Close()
	if report.Attention, err = scanReplies(rows); err != nil {
		return port.ReplyStatus{}, err
	}
	return report, tx.Commit()
}

func readCounts(ctx context.Context, tx *sql.Tx, counts map[domain.State]int) error {
	rows, err := tx.QueryContext(ctx, "SELECT status,count(*) FROM outbox GROUP BY status")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var state domain.State
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return err
		}
		counts[state] = count
	}
	return rows.Err()
}
