package sqlitedb

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"modernc.org/sqlite"
)

func Backup(ctx context.Context, src *sql.DB, destination string) error {
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("backup %s: file already exists", destination)
	}
	conn, err := src.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Raw(func(driverConn any) error {
		source, ok := driverConn.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("backup %s: driver connection %T has no online backup", destination, driverConn)
		}
		backup, err := source.NewBackup(destination)
		if err != nil {
			return fmt.Errorf("backup %s: %w", destination, err)
		}
		for {
			more, err := backup.Step(-1)
			if err != nil {
				backup.Finish()
				return fmt.Errorf("backup %s: %w", destination, err)
			}
			if !more {
				break
			}
		}
		if err := backup.Finish(); err != nil {
			return fmt.Errorf("backup %s: %w", destination, err)
		}
		return os.Chmod(destination, 0o600)
	})
}
