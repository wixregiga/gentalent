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
	}
}
