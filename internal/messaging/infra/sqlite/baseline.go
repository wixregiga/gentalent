package sqlite

import (
	"context"
	"fmt"
	"time"

	"gentalent/internal/messaging/port"
)

func (s *Store) Baseline(ctx context.Context) (port.Baseline, error) {
	var result port.Baseline
	var armed string
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE((SELECT value FROM meta WHERE key='cursor'),''),
		COALESCE((SELECT value FROM meta WHERE key='armed'),'')
	`).Scan(&result.Cursor, &armed)
	if err != nil || armed == "" {
		return result, err
	}
	result.ArmedAt, err = time.Parse(time.RFC3339Nano, armed)
	if err != nil {
		return result, fmt.Errorf("read armed timestamp: %w", err)
	}
	return result, nil
}

func (s *Store) Arm(ctx context.Context, baseline port.Baseline) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO meta VALUES ('armed',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value WHERE meta.value=''
	`, baseline.ArmedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO meta VALUES ('cursor',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value
	`, baseline.Cursor); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
