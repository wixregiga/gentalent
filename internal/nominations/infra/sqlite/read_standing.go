package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type Reader struct {
	db *sql.DB
}

func NewReader(db *sql.DB) Reader {
	return Reader{db: db}
}

const latestFinalizedOfEachPerson = `SELECT p.name FROM nominations n JOIN persons p ON p.id = n.person_id
	WHERE n.number = (SELECT MAX(m.number) FROM nominations m WHERE m.person_id = n.person_id AND m.status <> 'nominated')
	AND n.status = ? ORDER BY n.number`

func (r Reader) Standing(ctx context.Context) (port.Standing, error) {
	var standing port.Standing
	open, err := r.views(ctx, "WHERE n.status = 'nominated'")
	if err != nil {
		return port.Standing{}, err
	}
	standing.Open = open
	latest, err := r.views(ctx, "WHERE n.number = (SELECT MAX(number) FROM nominations)")
	if err != nil {
		return port.Standing{}, err
	}
	if len(latest) == 1 {
		standing.Latest = latest[0]
	}
	if standing.Certified, err = r.names(ctx, domain.Certified); err != nil {
		return port.Standing{}, err
	}
	if standing.Rejected, err = r.names(ctx, domain.Rejected); err != nil {
		return port.Standing{}, err
	}
	searching, err := r.views(ctx, "WHERE n.photos = 'searching'")
	if err != nil {
		return port.Standing{}, err
	}
	for _, view := range searching {
		standing.Searching = append(standing.Searching, view.Number)
	}
	return standing, nil
}

func (r Reader) views(ctx context.Context, where string) ([]port.NominationView, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT n.number, p.name, n.nominator, n.status, n.locked_at, n.photos
		FROM nominations n JOIN persons p ON p.id = n.person_id `+where+` ORDER BY n.number`)
	if err != nil {
		return nil, fmt.Errorf("read nominations: %w", err)
	}
	defer rows.Close()
	var views []port.NominationView
	for rows.Next() {
		var view port.NominationView
		var lockedAt string
		if err := rows.Scan(&view.Number, &view.Person, &view.Nominator, &view.Status, &lockedAt, &view.Photos); err != nil {
			return nil, fmt.Errorf("read nominations: %w", err)
		}
		if view.LockedAt, err = parseTime(lockedAt); err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read nominations: %w", err)
	}
	for i := range views {
		if views[i].Votes, err = r.votes(ctx, views[i].Number); err != nil {
			return nil, err
		}
	}
	return views, nil
}

func (r Reader) votes(ctx context.Context, number domain.NominationNumber) (map[domain.MemberID]domain.Vote, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT member, vote FROM votes WHERE number = ?", number)
	if err != nil {
		return nil, fmt.Errorf("read votes: %w", err)
	}
	defer rows.Close()
	votes := map[domain.MemberID]domain.Vote{}
	for rows.Next() {
		var member domain.MemberID
		var vote domain.Vote
		if err := rows.Scan(&member, &vote); err != nil {
			return nil, fmt.Errorf("read votes: %w", err)
		}
		votes[member] = vote
	}
	return votes, rows.Err()
}

func (r Reader) names(ctx context.Context, status domain.CertificationStatus) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, latestFinalizedOfEachPerson, status)
	if err != nil {
		return nil, fmt.Errorf("read %s Persons: %w", status, err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("read %s Persons: %w", status, err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
