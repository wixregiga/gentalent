package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

func importHistory(ctx context.Context, target *sql.Tx, path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat source database: %w", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	dsn := (&url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=ro&_pragma=busy_timeout%2810000%29"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	source, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("open source snapshot: %w", err)
	}
	defer source.Rollback()
	for _, table := range []struct {
		read, write string
		columns     int
	}{
		{"SELECT key,value FROM meta", "INSERT INTO meta VALUES (?,?)", 2},
		{"SELECT message_id FROM processed", "INSERT INTO processed VALUES (?)", 1},
	} {
		rows, err := source.QueryContext(ctx, table.read)
		if err != nil {
			return err
		}
		err = importRows(ctx, target, rows, table.write, table.columns)
		rows.Close()
		if err != nil {
			return err
		}
	}
	rows, err := source.QueryContext(ctx, `
		SELECT id,text,status,COALESCE(pending_id,''),COALESCE(final_id,''),kind,attachment_path,
		CASE WHEN case_id IS NOT NULL AND rowid=(SELECT MIN(first.rowid) FROM outbox first
		WHERE first.case_id=reply.case_id) THEN 'announces/' || case_id ELSE '' END
		FROM outbox reply ORDER BY rowid
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key, text, state, pending, final, kind, attachment, tag string
		if err := rows.Scan(&key, &text, &state, &pending, &final, &kind, &attachment, &tag); err != nil {
			return err
		}
		if _, err := target.ExecContext(ctx, `
			INSERT INTO outbox(key,text,status,pending_id,final_id,kind,attachment,attachment_name,tag)
			VALUES (?,?,?,?,?,?,?,?,?)
		`, key, text, state, pending, final, kind, attachment, attachmentName(attachment), tag); err != nil {
			return fmt.Errorf("import reply %s: %w", key, err)
		}
	}
	return rows.Err()
}

func importRows(ctx context.Context, target *sql.Tx, rows *sql.Rows, statement string, columns int) error {
	values := make([]any, columns)
	destinations := make([]any, columns)
	for i := range values {
		destinations[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(destinations...); err != nil {
			return err
		}
		if _, err := target.ExecContext(ctx, statement, values...); err != nil {
			return err
		}
	}
	return rows.Err()
}
