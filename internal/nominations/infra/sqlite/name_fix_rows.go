package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"

	"gentalent/internal/nominations/domain"
)

func loadSpellings(ctx context.Context, tx *sql.Tx) (map[domain.Spelling]domain.PersonID, error) {
	rows, err := tx.QueryContext(ctx, "SELECT alias, person_id FROM aliases")
	if err != nil {
		return nil, fmt.Errorf("load spellings: %w", err)
	}
	defer rows.Close()
	spellings := map[domain.Spelling]domain.PersonID{}
	for rows.Next() {
		var spelling domain.Spelling
		var person domain.PersonID
		if err := rows.Scan(&spelling, &person); err != nil {
			return nil, fmt.Errorf("load spellings: %w", err)
		}
		spellings[spelling] = person
	}
	return spellings, rows.Err()
}

func saveSpellings(ctx context.Context, tx *sql.Tx, before, after map[domain.Spelling]domain.PersonID) error {
	for _, spelling := range slices.Sorted(maps.Keys(after)) {
		person := after[spelling]
		was, known := before[spelling]
		if known && was == person {
			continue
		}
		if known {
			return fmt.Errorf("the spelling %q names Person %s and cannot move to Person %s", spelling, was, person)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO aliases(alias, person_id) VALUES(?, ?)", spelling, person); err != nil {
			return fmt.Errorf("save the spelling %q: %w", spelling, err)
		}
	}
	return nil
}

func loadNameFixes(ctx context.Context, tx *sql.Tx) ([]domain.NameFix, error) {
	rows, err := tx.QueryContext(ctx, `SELECT number, nomination, person_id, from_name, to_name, spelling, proposer, with_photos,
		proposed_at, status, ended_at FROM name_fixes ORDER BY number`)
	if err != nil {
		return nil, fmt.Errorf("load name fixes: %w", err)
	}
	defer rows.Close()
	var fixes []domain.NameFix
	index := map[domain.NameFixNumber]int{}
	for rows.Next() {
		fix := domain.NameFix{Answers: map[domain.MemberID]domain.Vote{}}
		var proposedAt, endedAt string
		if err := rows.Scan(&fix.Number, &fix.Nomination, &fix.Person, &fix.From, &fix.To, &fix.Spelling, &fix.Proposer,
			&fix.WithPhotos, &proposedAt, &fix.Status, &endedAt); err != nil {
			return nil, fmt.Errorf("load name fixes: %w", err)
		}
		if fix.ProposedAt, err = parseTime(proposedAt); err != nil {
			return nil, err
		}
		if fix.EndedAt, err = parseTime(endedAt); err != nil {
			return nil, err
		}
		index[fix.Number] = len(fixes)
		fixes = append(fixes, fix)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load name fixes: %w", err)
	}
	answers, err := tx.QueryContext(ctx, "SELECT number, member, answer FROM name_fix_answers")
	if err != nil {
		return nil, fmt.Errorf("load name fix answers: %w", err)
	}
	defer answers.Close()
	for answers.Next() {
		var number domain.NameFixNumber
		var member domain.MemberID
		var answer domain.Vote
		if err := answers.Scan(&number, &member, &answer); err != nil {
			return nil, fmt.Errorf("load name fix answers: %w", err)
		}
		fixes[index[number]].Answers[member] = answer
	}
	return fixes, answers.Err()
}

func saveNameFixes(ctx context.Context, tx *sql.Tx, before, after []domain.NameFix) error {
	known := map[domain.NameFixNumber]domain.NameFix{}
	for _, fix := range before {
		known[fix.Number] = fix
	}
	for _, fix := range after {
		was, existed := known[fix.Number]
		if err := saveNameFix(ctx, tx, was, existed, fix); err != nil {
			return fmt.Errorf("save name fix %d: %w", fix.Number, err)
		}
	}
	return nil
}

func saveNameFix(ctx context.Context, tx *sql.Tx, was domain.NameFix, existed bool, after domain.NameFix) error {
	if !existed {
		if err := insertNameFix(ctx, tx, after); err != nil {
			return err
		}
		was = domain.NameFix{Status: domain.NameFixPending}
	} else if !sameProposal(was, after) {
		return fmt.Errorf("a name fix keeps what it proposed")
	}
	if !maps.Equal(was.Answers, after.Answers) {
		if err := replaceAnswers(ctx, tx, after.Number, after.Answers); err != nil {
			return err
		}
	}
	if was.Status == after.Status && was.EndedAt.Equal(after.EndedAt) {
		return nil
	}
	_, err := tx.ExecContext(ctx, "UPDATE name_fixes SET status = ?, ended_at = ? WHERE number = ?",
		after.Status, formatTime(after.EndedAt), after.Number)
	return err
}

func sameProposal(was, after domain.NameFix) bool {
	return was.Nomination == after.Nomination && was.Person == after.Person && was.From == after.From && was.To == after.To &&
		was.Spelling == after.Spelling && was.Proposer == after.Proposer && was.WithPhotos == after.WithPhotos &&
		was.ProposedAt.Equal(after.ProposedAt)
}

func insertNameFix(ctx context.Context, tx *sql.Tx, fix domain.NameFix) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO name_fixes(number, nomination, person_id, from_name, to_name, spelling, proposer,
		with_photos, proposed_at, status) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fix.Number, fix.Nomination, fix.Person, fix.From, fix.To, fix.Spelling, fix.Proposer, fix.WithPhotos,
		formatTime(fix.ProposedAt), domain.NameFixPending)
	return err
}

func replaceAnswers(ctx context.Context, tx *sql.Tx, number domain.NameFixNumber, answers map[domain.MemberID]domain.Vote) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM name_fix_answers WHERE number = ?", number); err != nil {
		return err
	}
	for _, member := range slices.Sorted(maps.Keys(answers)) {
		if _, err := tx.ExecContext(ctx, "INSERT INTO name_fix_answers(number, member, answer) VALUES(?, ?, ?)",
			number, member, answers[member]); err != nil {
			return err
		}
	}
	return nil
}
