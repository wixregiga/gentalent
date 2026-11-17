package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"

	"gentalent/internal/nominations/domain"
)

func loadGroupRows(ctx context.Context, tx *sql.Tx) ([]domain.Nomination, []domain.Shame, error) {
	nominations, err := loadNominations(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	shames, err := loadShames(ctx, tx)
	return nominations, shames, err
}

func loadNominations(ctx context.Context, tx *sql.Tx) ([]domain.Nomination, error) {
	rows, err := tx.QueryContext(ctx, `SELECT n.number, n.person_id, p.name, n.nominator, n.status, n.nominated_at, n.locked_at,
		n.finalized_at, n.pitch, n.photos, n.strip FROM nominations n JOIN persons p ON p.id = n.person_id ORDER BY n.number`)
	if err != nil {
		return nil, fmt.Errorf("load nominations: %w", err)
	}
	defer rows.Close()
	var nominations []domain.Nomination
	index := map[domain.NominationNumber]int{}
	for rows.Next() {
		var n domain.Nomination
		var nominatedAt, lockedAt, finalizedAt string
		if err := rows.Scan(&n.Number, &n.Person.ID, &n.Person.Name, &n.Nominator, &n.Status, &nominatedAt, &lockedAt,
			&finalizedAt, &n.Pitch, &n.Photos, &n.Strip); err != nil {
			return nil, fmt.Errorf("load nominations: %w", err)
		}
		if n.NominatedAt, err = parseTime(nominatedAt); err != nil {
			return nil, err
		}
		if n.LockedAt, err = parseTime(lockedAt); err != nil {
			return nil, err
		}
		if n.FinalizedAt, err = parseTime(finalizedAt); err != nil {
			return nil, err
		}
		index[n.Number] = len(nominations)
		nominations = append(nominations, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load nominations: %w", err)
	}
	if err := eachVote(ctx, tx, "votes", func(number domain.NominationNumber, member domain.MemberID, vote domain.Vote) {
		put(&nominations[index[number]].Votes, member, vote)
	}); err != nil {
		return nil, err
	}
	if err := eachVote(ctx, tx, "reactions", func(number domain.NominationNumber, member domain.MemberID, vote domain.Vote) {
		put(&nominations[index[number]].Reactions, member, vote)
	}); err != nil {
		return nil, err
	}
	exhibits, err := tx.QueryContext(ctx, "SELECT number, text FROM exhibits ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("load exhibits: %w", err)
	}
	defer exhibits.Close()
	for exhibits.Next() {
		var number domain.NominationNumber
		var text string
		if err := exhibits.Scan(&number, &text); err != nil {
			return nil, fmt.Errorf("load exhibits: %w", err)
		}
		n := &nominations[index[number]]
		n.Exhibits = append(n.Exhibits, text)
	}
	return nominations, exhibits.Err()
}

func eachVote(ctx context.Context, tx *sql.Tx, table string, use func(domain.NominationNumber, domain.MemberID, domain.Vote)) error {
	rows, err := tx.QueryContext(ctx, "SELECT number, member, vote FROM "+table)
	if err != nil {
		return fmt.Errorf("load %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var number domain.NominationNumber
		var member domain.MemberID
		var vote domain.Vote
		if err := rows.Scan(&number, &member, &vote); err != nil {
			return fmt.Errorf("load %s: %w", table, err)
		}
		use(number, member, vote)
	}
	return rows.Err()
}

func put(votes *map[domain.MemberID]domain.Vote, member domain.MemberID, vote domain.Vote) {
	if *votes == nil {
		*votes = map[domain.MemberID]domain.Vote{}
	}
	(*votes)[member] = vote
}

func loadShames(ctx context.Context, tx *sql.Tx) ([]domain.Shame, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, number, shamer, target, reason, COALESCE(target_vote, ''),
		COALESCE(shamed_message_id, ''), COALESCE(shamed_text, ''), COALESCE(shamed_at, ''), gif, message_id, created_at
		FROM shames ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("load shames: %w", err)
	}
	defer rows.Close()
	var shames []domain.Shame
	for rows.Next() {
		var s domain.Shame
		var shamedAt, createdAt string
		if err := rows.Scan(&s.ID, &s.Nomination, &s.By, &s.Target, &s.Reason, &s.TargetVote,
			&s.Shamed.ID, &s.Shamed.Text, &shamedAt, &s.GIF, &s.MessageID, &createdAt); err != nil {
			return nil, fmt.Errorf("load shames: %w", err)
		}
		if s.Shamed.SentAt, err = parseTime(shamedAt); err != nil {
			return nil, err
		}
		if s.At, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		shames = append(shames, s)
	}
	return shames, rows.Err()
}

func saveGroupRows(ctx context.Context, tx *sql.Tx, nominations []domain.Nomination, shames []domain.Shame, group *domain.Group) error {
	known := map[domain.NominationNumber]domain.Nomination{}
	for _, n := range nominations {
		known[n.Number] = n
	}
	for _, after := range group.Nominations() {
		was, existed := known[after.Number]
		if err := saveNomination(ctx, tx, was, existed, after); err != nil {
			return fmt.Errorf("save nomination %d: %w", after.Number, err)
		}
	}
	given := map[domain.ShameID]bool{}
	for _, s := range shames {
		given[s.ID] = true
	}
	for _, s := range group.Shames() {
		if given[s.ID] {
			continue
		}
		if err := insertShame(ctx, tx, s); err != nil {
			return fmt.Errorf("save shame %d: %w", s.ID, err)
		}
	}
	return nil
}

func saveNomination(ctx context.Context, tx *sql.Tx, was domain.Nomination, existed bool, after domain.Nomination) error {
	if !existed {
		if err := insertNomination(ctx, tx, after); err != nil {
			return err
		}
	} else if err := updateNomination(ctx, tx, was, after); err != nil {
		return err
	}
	if !existed || !maps.Equal(was.Votes, after.Votes) {
		if err := replaceVotes(ctx, tx, "votes", after.Number, after.Votes); err != nil {
			return err
		}
	}
	if !existed || !maps.Equal(was.Reactions, after.Reactions) {
		if err := replaceVotes(ctx, tx, "reactions", after.Number, after.Reactions); err != nil {
			return err
		}
	}
	for _, text := range after.Exhibits[min(len(was.Exhibits), len(after.Exhibits)):] {
		if _, err := tx.ExecContext(ctx, "INSERT INTO exhibits(number, text) VALUES(?, ?)", after.Number, text); err != nil {
			return err
		}
	}
	return nil
}

func insertNomination(ctx context.Context, tx *sql.Tx, n domain.Nomination) error {
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO persons(id, name) VALUES(?, ?)", n.Person.ID, n.Person.Name); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO aliases(alias, person_id) VALUES(?, ?)", n.Person.ID, n.Person.ID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO nominations(number, person_id, nominator, status, nominated_at, locked_at,
		finalized_at, pitch, photos, strip) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.Number, n.Person.ID, n.Nominator, n.Status, formatTime(n.NominatedAt), formatTime(n.LockedAt),
		formatTime(n.FinalizedAt), n.Pitch, n.Photos, n.Strip)
	return err
}

func updateNomination(ctx context.Context, tx *sql.Tx, was, after domain.Nomination) error {
	if was.Status != after.Status || !was.FinalizedAt.Equal(after.FinalizedAt) {
		if _, err := tx.ExecContext(ctx, "UPDATE nominations SET status = ?, finalized_at = ? WHERE number = ?",
			after.Status, formatTime(after.FinalizedAt), after.Number); err != nil {
			return err
		}
	}
	if was.LockedAt.Equal(after.LockedAt) && was.Pitch == after.Pitch && was.Photos == after.Photos && was.Strip == after.Strip {
		return nil
	}
	_, err := tx.ExecContext(ctx, "UPDATE nominations SET locked_at = ?, pitch = ?, photos = ?, strip = ? WHERE number = ?",
		formatTime(after.LockedAt), after.Pitch, after.Photos, after.Strip, after.Number)
	return err
}

func replaceVotes(ctx context.Context, tx *sql.Tx, table string, number domain.NominationNumber, votes map[domain.MemberID]domain.Vote) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE number = ?", number); err != nil {
		return err
	}
	for _, member := range slices.Sorted(maps.Keys(votes)) {
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+table+"(number, member, vote) VALUES(?, ?, ?)", number, member, votes[member]); err != nil {
			return err
		}
	}
	return nil
}

func insertShame(ctx context.Context, tx *sql.Tx, s domain.Shame) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO shames(id, number, shamer, target, reason, target_vote, shamed_message_id,
		shamed_text, shamed_at, gif, message_id, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Nomination, s.By, s.Target, s.Reason, nullable(string(s.TargetVote)), nullable(s.Shamed.ID),
		nullable(s.Shamed.Text), nullable(formatTime(s.Shamed.SentAt)), s.GIF, s.MessageID, formatTime(s.At))
	return err
}
