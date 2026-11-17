package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	pythonPersonPrefix = "nominee:"
	pythonStripFile    = "strip.jpg"
	pythonDenialTime   = "2006-01-02 15:04:05"
)

type pythonTable struct {
	name   string
	query  string
	insert string
	adapt  func(row []any) error
}

func importPython(ctx context.Context, tx *sql.Tx, path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("find the Python database: %w", err)
	}
	dsn := (&url.URL{Scheme: "file", Opaque: path, RawQuery: "mode=ro"}).String()
	python, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open the Python database read-only: %w", err)
	}
	defer python.Close()
	if err := everyDenialHasItsCase(ctx, python); err != nil {
		return err
	}
	for _, table := range pythonTables() {
		if err := copyTable(ctx, python, tx, table); err != nil {
			return fmt.Errorf("import Python %s: %w", table.name, err)
		}
	}
	return nil
}

func pythonTables() []pythonTable {
	return []pythonTable{
		{
			name:   "nominees",
			query:  "SELECT id, name FROM nominees ORDER BY rowid",
			insert: "INSERT INTO persons(id, name) VALUES(?, ?)",
			adapt:  func(row []any) error { row[0] = personID(row[0]); return nil },
		},
		{
			name:   "aliases",
			query:  "SELECT alias, nominee_id FROM aliases ORDER BY rowid",
			insert: "INSERT INTO aliases(alias, person_id) VALUES(?, ?)",
			adapt: func(row []any) error {
				if err := required(row); err != nil {
					return err
				}
				row[1] = personID(row[1])
				return nil
			},
		},
		{
			name: "cases",
			query: `SELECT c.id, c.nominee_id, c.nominator, c.phase, c.all_votes_at, c.ai_invoked, c.ai_artifact_dir, d.recorded_at
				FROM cases c LEFT JOIN denials d ON d.nominee_id = c.nominee_id AND c.phase = 'denied' ORDER BY c.id`,
			insert: `INSERT INTO nominations(number, person_id, nominator, status, locked_at, photos, strip, finalized_at)
				VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			adapt: adaptCase,
		},
		{
			name:   "votes",
			query:  "SELECT case_id, sender_id, vote FROM votes ORDER BY rowid",
			insert: "INSERT INTO votes(number, member, vote) VALUES(?, ?, ?)",
			adapt:  required,
		},
		{
			name:   "reaction_votes",
			query:  "SELECT case_id, member_id, vote FROM reaction_votes WHERE vote IS NOT NULL ORDER BY rowid",
			insert: "INSERT INTO reactions(number, member, vote) VALUES(?, ?, ?)",
			adapt:  required,
		},
		{
			name:   "exhibits",
			query:  "SELECT id, case_id, text FROM exhibits ORDER BY id",
			insert: "INSERT INTO exhibits(id, number, text) VALUES(?, ?, ?)",
			adapt:  required,
		},
		{
			name: "shames",
			query: `SELECT id, case_id, shamer_id, target_id, reason, target_vote, shamed_message_id, shamed_text, shamed_at,
				gif, message_id, created_at FROM shames ORDER BY id`,
			insert: `INSERT INTO shames(id, number, shamer, target, reason, target_vote, shamed_message_id, shamed_text,
				shamed_at, gif, message_id, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			adapt: func(row []any) error { return retime(row, 8, 11) },
		},
		{
			name:   "feedback",
			query:  "SELECT id, message_id, sender_id, kind, text, said_at FROM feedback ORDER BY id",
			insert: "INSERT INTO feedback(id, message_id, sender, kind, text, said_at) VALUES(?, ?, ?, ?, ?, ?)",
			adapt:  func(row []any) error { return retime(row, 5) },
		},
	}
}

func everyDenialHasItsCase(ctx context.Context, python *sql.DB) error {
	var orphan string
	err := python.QueryRowContext(ctx, `SELECT d.nominee_id FROM denials d WHERE NOT EXISTS
		(SELECT 1 FROM cases c WHERE c.nominee_id = d.nominee_id AND c.phase = 'denied') LIMIT 1`).Scan(&orphan)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("read Python denials: %w", err)
	}
	return fmt.Errorf("import Python denials: %s is denied with no denied case", orphan)
}

func copyTable(ctx context.Context, python *sql.DB, tx *sql.Tx, table pythonTable) error {
	rows, err := python.QueryContext(ctx, table.query)
	if err != nil {
		return err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	for rows.Next() {
		row := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range row {
			pointers[i] = &row[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return err
		}
		if err := table.adapt(row); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, table.insert, row...); err != nil {
			return fmt.Errorf("row %v: %w", row, err)
		}
	}
	return rows.Err()
}

func adaptCase(row []any) error {
	if err := required(row[:4]); err != nil {
		return err
	}
	row[1] = personID(row[1])
	status, ok := map[string]string{"voting": "nominated", "certified": "certified", "denied": "rejected"}[text(row[3])]
	if !ok {
		return fmt.Errorf("case %v has unknown phase %v", row[0], row[3])
	}
	row[3] = status
	locked, err := epochTime(row[4])
	if err != nil {
		return fmt.Errorf("case %v: %w", row[0], err)
	}
	row[4] = formatTime(locked)
	photos, strip := "", ""
	if invoked, _ := row[5].(int64); invoked != 0 {
		photos = "failed"
		if dir := text(row[6]); dir != "" {
			photos, strip = "found", filepath.Join(dir, pythonStripFile)
		}
	}
	row[5], row[6] = photos, strip
	finalized := time.Time{}
	if recorded := text(row[7]); recorded != "" {
		if finalized, err = time.ParseInLocation(pythonDenialTime, recorded, time.UTC); err != nil {
			return fmt.Errorf("case %v has an unreadable denial time: %w", row[0], err)
		}
	}
	row[7] = formatTime(finalized)
	return nil
}

func epochTime(value any) (time.Time, error) {
	switch seconds := value.(type) {
	case nil:
		return time.Time{}, nil
	case float64:
		whole, fraction := math.Modf(seconds)
		return time.Unix(int64(whole), int64(math.Round(fraction*1e6))*1e3), nil
	case int64:
		return time.Unix(seconds, 0), nil
	}
	return time.Time{}, fmt.Errorf("unreadable all_votes_at %v", value)
}

func personID(value any) string {
	return strings.TrimPrefix(text(value), pythonPersonPrefix)
}

func required(row []any) error {
	for _, value := range row {
		if value == nil {
			return fmt.Errorf("row %v has an empty required column", row)
		}
	}
	return nil
}

func retime(row []any, columns ...int) error {
	for _, column := range columns {
		if row[column] == nil {
			continue
		}
		at, err := parseTime(text(row[column]))
		if err != nil {
			return err
		}
		row[column] = formatTime(at)
	}
	return nil
}

func text(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case nil:
		return ""
	}
	return fmt.Sprint(value)
}
