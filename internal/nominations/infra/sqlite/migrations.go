package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

func Migrations() []func(context.Context, *sql.Tx) error {
	return []func(context.Context, *sql.Tx) error{
		func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, schema); err != nil {
				return fmt.Errorf("create the nominations schema: %w", err)
			}
			return nil
		},
		func(ctx context.Context, tx *sql.Tx) error {
			if err := giveEveryPersonAnOpaqueID(ctx, tx); err != nil {
				return fmt.Errorf("give every Person an opaque ID: %w", err)
			}
			if _, err := tx.ExecContext(ctx, nameFixSchema); err != nil {
				return fmt.Errorf("create the name fix schema: %w", err)
			}
			return nil
		},
	}
}
