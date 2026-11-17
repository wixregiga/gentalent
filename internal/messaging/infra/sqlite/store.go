package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) StartWorker(ctx context.Context, binding string) error {
	if binding == "" {
		return fmt.Errorf("database binding is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO meta VALUES ('binding', ?) ON CONFLICT(key) DO NOTHING", binding); err != nil {
		return err
	}
	var saved string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='binding'").Scan(&saved); err != nil {
		return err
	}
	if saved != binding {
		return fmt.Errorf("database belongs to another chat/roster. Use a separate database")
	}
	if _, err := tx.ExecContext(ctx, "UPDATE outbox SET status='uncertain' WHERE status='sending'"); err != nil {
		return err
	}
	return tx.Commit()
}
