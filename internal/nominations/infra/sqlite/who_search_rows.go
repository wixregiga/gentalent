package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"gentalent/internal/nominations/domain"
)

func loadWhoSearches(ctx context.Context, tx *sql.Tx) ([]domain.WhoSearch, error) {
	rows, err := tx.QueryContext(ctx, `SELECT number, searcher, description, started_at, status, figure_name, figure_spelling,
		blurb, source, photo, provider, COALESCE(person_id, '') FROM who_searches ORDER BY number`)
	if err != nil {
		return nil, fmt.Errorf("load who searches: %w", err)
	}
	defer rows.Close()
	var searches []domain.WhoSearch
	for rows.Next() {
		var search domain.WhoSearch
		var startedAt string
		figure := &search.Figure
		if err := rows.Scan(&search.Number, &search.By, &search.Description, &startedAt, &search.Status, &figure.Name, &figure.Spelling,
			&figure.Blurb, &figure.Source, &figure.Photo, &figure.Provider, &figure.Person); err != nil {
			return nil, fmt.Errorf("load who searches: %w", err)
		}
		if search.StartedAt, err = parseTime(startedAt); err != nil {
			return nil, err
		}
		searches = append(searches, search)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load who searches: %w", err)
	}
	return searches, nil
}

func saveWhoSearches(ctx context.Context, tx *sql.Tx, before, after []domain.WhoSearch) error {
	known := map[domain.WhoSearchNumber]domain.WhoSearch{}
	for _, search := range before {
		known[search.Number] = search
	}
	for _, search := range after {
		was, existed := known[search.Number]
		if err := saveWhoSearch(ctx, tx, was, existed, search); err != nil {
			return fmt.Errorf("save who search %d: %w", search.Number, err)
		}
	}
	return nil
}

func saveWhoSearch(ctx context.Context, tx *sql.Tx, was domain.WhoSearch, existed bool, after domain.WhoSearch) error {
	if !existed {
		_, err := tx.ExecContext(ctx, `INSERT INTO who_searches(number, searcher, description, started_at, status) VALUES(?, ?, ?, ?, ?)`,
			after.Number, after.By, after.Description, formatTime(after.StartedAt), after.Status)
		if err != nil {
			return err
		}
		was = domain.WhoSearch{Number: after.Number, By: after.By, Description: after.Description, StartedAt: after.StartedAt, Status: after.Status}
	} else if was.By != after.By || was.Description != after.Description || !was.StartedAt.Equal(after.StartedAt) {
		return fmt.Errorf("a who search keeps who asked, what they asked and when")
	}
	if was.Status == after.Status && was.Figure == after.Figure {
		return nil
	}
	figure := after.Figure
	_, err := tx.ExecContext(ctx, `UPDATE who_searches SET status = ?, figure_name = ?, figure_spelling = ?, blurb = ?, source = ?,
		photo = ?, provider = ?, person_id = ? WHERE number = ?`,
		after.Status, figure.Name, figure.Spelling, figure.Blurb, figure.Source, figure.Photo, figure.Provider,
		nullable(string(figure.Person)), after.Number)
	return err
}
