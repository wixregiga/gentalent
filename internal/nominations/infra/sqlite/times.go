package sqlite

import (
	"database/sql"
	"fmt"
	"time"
)

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(text string) (time.Time, error) {
	if text == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("read time %q: %w", text, err)
	}
	return t, nil
}

func nullable(text string) sql.NullString {
	return sql.NullString{String: text, Valid: text != ""}
}
