package sqlitedb

import (
	"context"
	"database/sql"
	"fmt"
)

type Migration struct {
	Name string
	Up   func(ctx context.Context, tx *sql.Tx) error
}

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
		migration := migrations[i]
		err := InTx(ctx, db, func(tx *sql.Tx) error {
			if err := migration.Up(ctx, tx); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1))
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %d (%s): %w", i+1, migration.Name, err)
		}
	}
	return nil
}
