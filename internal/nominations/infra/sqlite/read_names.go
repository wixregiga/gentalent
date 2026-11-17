package sqlite

import (
	"context"
	"fmt"

	"gentalent/internal/nominations/port"
)

func (r Reader) Names(ctx context.Context) ([]port.Name, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT a.alias, p.id, p.name FROM aliases a JOIN persons p ON p.id = a.person_id
		JOIN (SELECT person_id, MAX(number) AS latest FROM nominations GROUP BY person_id) l ON l.person_id = p.id
		ORDER BY l.latest DESC, a.alias`)
	if err != nil {
		return nil, fmt.Errorf("read names: %w", err)
	}
	defer rows.Close()
	var names []port.Name
	for rows.Next() {
		var name port.Name
		if err := rows.Scan(&name.Alias, &name.Person, &name.Display); err != nil {
			return nil, fmt.Errorf("read names: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
