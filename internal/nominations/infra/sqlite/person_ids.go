package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

const outcomesTrigger = "keep_outcomes_update"

func newPersonID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("make a Person ID: %w", err)
	}
	return id.String(), nil
}

func giveEveryPersonAnOpaqueID(ctx context.Context, tx *sql.Tx) error {
	var trigger string
	switch err := tx.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = ?", outcomesTrigger).Scan(&trigger); {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("the %s trigger is missing", outcomesTrigger)
	case err != nil:
		return fmt.Errorf("read the %s trigger: %w", outcomesTrigger, err)
	}
	if _, err := tx.ExecContext(ctx, "DROP TRIGGER "+outcomesTrigger); err != nil {
		return err
	}
	persons, err := personsInNominationOrder(ctx, tx)
	if err != nil {
		return err
	}
	for _, old := range persons {
		id, err := newPersonID()
		if err != nil {
			return err
		}
		if err := rekeyPerson(ctx, tx, old, id); err != nil {
			return fmt.Errorf("re-key Person %q: %w", old, err)
		}
	}
	if _, err := tx.ExecContext(ctx, trigger); err != nil {
		return fmt.Errorf("recreate the %s trigger: %w", outcomesTrigger, err)
	}
	return nil
}

func personsInNominationOrder(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT p.id FROM persons p LEFT JOIN nominations n ON n.person_id = p.id
		GROUP BY p.id ORDER BY MIN(n.number) IS NULL, MIN(n.number), p.id`)
	if err != nil {
		return nil, fmt.Errorf("list Persons: %w", err)
	}
	defer rows.Close()
	var persons []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list Persons: %w", err)
		}
		persons = append(persons, id)
	}
	return persons, rows.Err()
}

func rekeyPerson(ctx context.Context, tx *sql.Tx, old, id string) error {
	for _, statement := range []string{
		"INSERT INTO persons(id, name) SELECT ?2, name FROM persons WHERE id = ?1",
		"UPDATE nominations SET person_id = ?2 WHERE person_id = ?1",
		"UPDATE aliases SET person_id = ?2 WHERE person_id = ?1",
		"DELETE FROM persons WHERE id = ?1",
	} {
		if _, err := tx.ExecContext(ctx, statement, old, id); err != nil {
			return err
		}
	}
	return nil
}
