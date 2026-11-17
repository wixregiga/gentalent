package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type Store struct {
	db       *sql.DB
	id       domain.GroupID
	members  []domain.MemberID
	settings domain.Settings
}

func NewStore(ctx context.Context, db *sql.DB, id domain.GroupID, members []domain.MemberID, settings domain.Settings) (*Store, error) {
	store := &Store{db: db, id: id, members: members, settings: settings}
	err := store.inTx(ctx, func(tx *sql.Tx) error {
		var recorded string
		switch err := tx.QueryRowContext(ctx, "SELECT value FROM meta WHERE key = 'group'").Scan(&recorded); {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, "INSERT INTO meta(key, value) VALUES('group', ?)", string(id)); err != nil {
				return err
			}
		case err != nil:
			return err
		case recorded != string(id):
			return errors.New("the nominations database belongs to another Group; use a separate database")
		}
		nominations, shames, err := loadGroupRows(ctx, tx)
		if err != nil {
			return err
		}
		_, err = domain.NewGroup(id, members, settings, nominations, shames)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("open the nominations store: %w", err)
	}
	return store, nil
}

func (s *Store) Apply(ctx context.Context, messageID string, change port.Change) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if handled, err := s.claim(ctx, tx, messageID); err != nil || handled {
			return err
		}
		nominations, shames, err := loadGroupRows(ctx, tx)
		if err != nil {
			return err
		}
		group, err := domain.NewGroup(s.id, s.members, s.settings, nominations, shames)
		if err != nil {
			return fmt.Errorf("load the Group: %w", err)
		}
		replies, err := change(group)
		if err != nil {
			return err
		}
		if err := saveGroupRows(ctx, tx, nominations, shames, group); err != nil {
			return err
		}
		return insertReplies(ctx, tx, replies)
	})
}

func (s *Store) Answer(ctx context.Context, messageID string, replies []port.Reply) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if handled, err := s.claim(ctx, tx, messageID); err != nil || handled {
			return err
		}
		return insertReplies(ctx, tx, replies)
	})
}

func (s *Store) Log(ctx context.Context, entry port.Feedback, replies []port.Reply) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if handled, err := s.claim(ctx, tx, entry.MessageID); err != nil || handled {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO feedback(message_id, sender, kind, text, said_at) VALUES(?, ?, ?, ?, ?)",
			entry.MessageID, entry.Sender, entry.Kind, entry.Text, formatTime(entry.SaidAt)); err != nil {
			return fmt.Errorf("log feedback: %w", err)
		}
		return insertReplies(ctx, tx, replies)
	})
}

func (s *Store) MarkRelayed(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	args := make([]any, len(keys))
	for i, key := range keys {
		args[i] = key
	}
	query := "UPDATE outbox SET relayed = 1 WHERE key IN (?" + strings.Repeat(", ?", len(keys)-1) + ")"
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("mark replies relayed: %w", err)
	}
	return nil
}

func (s *Store) claim(ctx context.Context, tx *sql.Tx, messageID string) (bool, error) {
	if messageID == "" {
		return false, nil
	}
	result, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO handled(message_id) VALUES(?)", messageID)
	if err != nil {
		return false, fmt.Errorf("record message %s handled: %w", messageID, err)
	}
	added, err := result.RowsAffected()
	return added == 0, err
}

func (s *Store) inTx(ctx context.Context, work func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := work(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func insertReplies(ctx context.Context, tx *sql.Tx, replies []port.Reply) error {
	for _, reply := range replies {
		if _, err := tx.ExecContext(ctx, "INSERT INTO outbox(key, text, attachment, announces) VALUES(?, ?, ?, ?)",
			reply.Key, reply.Text, reply.Attachment, reply.Announces); err != nil {
			return fmt.Errorf("queue reply %s: %w", reply.Key, err)
		}
	}
	return nil
}
