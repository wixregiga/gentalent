package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"gentalent/internal/nominations/domain"
)

func loadPersons(ctx context.Context, tx *sql.Tx) ([]domain.Person, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id, name FROM persons ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("load persons: %w", err)
	}
	defer rows.Close()
	var persons []domain.Person
	index := map[domain.PersonID]int{}
	for rows.Next() {
		var person domain.Person
		if err := rows.Scan(&person.ID, &person.Name); err != nil {
			return nil, fmt.Errorf("load persons: %w", err)
		}
		index[person.ID] = len(persons)
		persons = append(persons, person)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load persons: %w", err)
	}
	spellings, err := tx.QueryContext(ctx, "SELECT alias, person_id FROM aliases ORDER BY alias")
	if err != nil {
		return nil, fmt.Errorf("load spellings: %w", err)
	}
	defer spellings.Close()
	for spellings.Next() {
		var spelling domain.Spelling
		var person domain.PersonID
		if err := spellings.Scan(&spelling, &person); err != nil {
			return nil, fmt.Errorf("load spellings: %w", err)
		}
		i, known := index[person]
		if !known {
			return nil, fmt.Errorf("the spelling %q names Person %s, who has no row", spelling, person)
		}
		persons[i].Spellings = append(persons[i].Spellings, spelling)
	}
	return persons, spellings.Err()
}

func savePersons(ctx context.Context, tx *sql.Tx, before, after []domain.Person) error {
	was := map[domain.PersonID]domain.Person{}
	for _, person := range before {
		was[person.ID] = person
	}
	for _, person := range after {
		old, existed := was[person.ID]
		switch {
		case !existed:
			if _, err := tx.ExecContext(ctx, "INSERT INTO persons(id, name) VALUES(?, ?)", person.ID, person.Name); err != nil {
				return fmt.Errorf("save Person %s: %w", person.ID, err)
			}
		case old.Name != person.Name:
			if _, err := tx.ExecContext(ctx, "UPDATE persons SET name = ? WHERE id = ?", person.Name, person.ID); err != nil {
				return fmt.Errorf("save the name of Person %s: %w", person.ID, err)
			}
		}
		for _, spelling := range old.Spellings {
			if !person.SpelledAs(spelling) {
				return fmt.Errorf("the spelling %q of Person %s cannot be dropped", spelling, person.ID)
			}
		}
		for _, spelling := range person.Spellings {
			if slices.Contains(old.Spellings, spelling) {
				continue
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO aliases(alias, person_id) VALUES(?, ?)", spelling, person.ID); err != nil {
				return fmt.Errorf("save the spelling %q: %w", spelling, err)
			}
		}
	}
	return nil
}
