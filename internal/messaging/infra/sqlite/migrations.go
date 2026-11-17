package sqlite

import (
	"context"
	"database/sql"
)

const schema = `
CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE processed(message_id TEXT PRIMARY KEY);
CREATE TABLE inbox(id TEXT PRIMARY KEY, sent_at INTEGER NOT NULL, sort_key TEXT NOT NULL, payload TEXT NOT NULL);
CREATE INDEX inbox_order ON inbox(sent_at, sort_key);
CREATE TABLE outbox(
 key TEXT PRIMARY KEY, text TEXT NOT NULL, attachment TEXT NOT NULL, attachment_name TEXT NOT NULL,
 tag TEXT NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL
 CHECK(status IN ('queued','sending','pending','sent','failed','uncertain','preview')),
 pending_id TEXT NOT NULL DEFAULT '', final_id TEXT NOT NULL DEFAULT ''
);
CREATE INDEX outbox_pending ON outbox(pending_id) WHERE pending_id <> '';
CREATE INDEX outbox_final ON outbox(final_id) WHERE final_id <> '';
CREATE UNIQUE INDEX outbox_tag ON outbox(tag) WHERE tag <> '';
CREATE INDEX outbox_attachment ON outbox(attachment_name) WHERE attachment_name <> '';
`

func Migrations(pythonDatabase string) []func(context.Context, *sql.Tx) error {
	return []func(context.Context, *sql.Tx) error{
		func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, schema); err != nil {
				return err
			}
			return importHistory(ctx, tx, pythonDatabase)
		},
	}
}
