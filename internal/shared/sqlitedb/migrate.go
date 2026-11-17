package sqlitedb

import (
	"context"
	"database/sql"
	"fmt"
)

type Migration = func(ctx context.Context, tx *sql.Tx) error

func Version(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	return version, err
}

func Migrate(ctx context.Context, db *sql.DB, migrations []Migration) error {
	version, err := Version(ctx, db)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this build knows (%d)", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		if err := apply(ctx, db, i+1, migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	return nil
}

func apply(ctx context.Context, db *sql.DB, version int, migration Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := migration(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	return tx.Commit()
}
