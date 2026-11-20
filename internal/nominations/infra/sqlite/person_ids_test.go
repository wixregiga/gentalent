package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"uuid"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/infra/sqlite"
	"gentalent/internal/nominations/port"
)

const firstSchemaBallot = `
INSERT INTO meta(key, value) VALUES('group', 'group-test');
INSERT INTO persons(id, name) VALUES
  ('adalovelace', 'Ada Lovelace'), ('gracehopper', 'Grace Hopper'),
  ('zoëcastañeda', 'Zoë Castañeda'), ('rinaokafor', '🦄 Rina Okafor ✨'), ('mirabelle', 'Mirabelle Østergaard');
INSERT INTO aliases(alias, person_id) VALUES
  ('adalovelace', 'adalovelace'), ('countessoflovelace', 'adalovelace'), ('gracehopper', 'gracehopper'),
  ('zoëcastañeda', 'zoëcastañeda'), ('rinaokafor', 'rinaokafor');
INSERT INTO nominations(number, person_id, nominator, status, nominated_at, locked_at, finalized_at, pitch, photos, strip) VALUES
  (1, 'adalovelace', 'member-alex', 'rejected', '2026-11-01T10:00:00Z', '2026-11-01T10:05:00Z', '2026-11-01T10:06:00Z', 'Engines', 'found', '/media/1/strip.jpg'),
  (2, 'gracehopper', 'member-blair', 'certified', '2026-11-02T10:00:00Z', '2026-11-02T10:05:00Z', '2026-11-02T10:06:00Z', '', '', ''),
  (3, 'rinaokafor', 'member-casey', 'rejected', '2026-11-03T10:00:00Z', '2026-11-03T10:05:00Z', '2026-11-03T10:06:00Z', '', 'failed', ''),
  (4, 'adalovelace', 'member-blair', 'nominated', '2026-11-04T10:00:00Z', '', '', 'Again', 'requested', ''),
  (5, 'zoëcastañeda', 'member-alex', 'nominated', '2026-11-05T10:00:00Z', '', '', '', '', '');
INSERT INTO votes(number, member, vote) VALUES
  (1, 'member-alex', 'yes'), (1, 'member-blair', 'no'), (1, 'member-casey', 'no'),
  (2, 'member-alex', 'yes'), (2, 'member-blair', 'yes'), (2, 'member-casey', 'yes'),
  (3, 'member-alex', 'no'), (3, 'member-blair', 'no'), (3, 'member-casey', 'yes'),
  (4, 'member-blair', 'yes');
INSERT INTO reactions(number, member, vote) VALUES(1, 'member-casey', 'no');
INSERT INTO exhibits(number, text) VALUES(1, 'Engines'), (4, 'Again');
INSERT INTO shames(number, shamer, target, message_id, created_at) VALUES(1, 'member-alex', 'member-casey', 'message-9', '2026-11-01T10:07:00Z');
`

const everyNomination = `SELECT group_concat(line, char(10)) FROM (SELECT n.number || ' ' || p.name || ' ' || n.status || ' ' || n.nominator || ' ' ||
	n.nominated_at || ' ' || n.locked_at || ' ' || n.finalized_at || ' ' || n.pitch || ' ' || n.photos || ' ' || n.strip || ' [' ||
	COALESCE((SELECT group_concat(member || '=' || vote, ',') FROM (SELECT member, vote FROM votes v WHERE v.number = n.number ORDER BY member)), '') || '] [' ||
	COALESCE((SELECT group_concat(member || '=' || vote, ',') FROM (SELECT member, vote FROM reactions r WHERE r.number = n.number ORDER BY member)), '') || '] ' ||
	(SELECT COUNT(*) FROM exhibits e WHERE e.number = n.number) || ' ' || (SELECT COUNT(*) FROM shames s WHERE s.number = n.number) || ' ' ||
	(SELECT COUNT(*) FROM nominations m WHERE m.person_id = n.person_id) AS line
	FROM nominations n JOIN persons p ON p.id = n.person_id ORDER BY n.number)`

const everyAlias = `SELECT group_concat(line, char(10)) FROM (SELECT a.alias || ' ' || p.name AS line
	FROM aliases a JOIN persons p ON p.id = a.person_id ORDER BY a.alias)`

const outcomeRules = `SELECT group_concat(name || ': ' || sql, char(10)) FROM (SELECT name, sql FROM sqlite_master
	WHERE name IN ('keep_outcomes_update', 'keep_outcomes_delete', 'one_open_nomination_per_person') ORDER BY name)`

func runMigration(t *testing.T, db *sql.DB, version int) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Migrations()[version-1](context.Background(), tx); err != nil {
		tx.Rollback()
		t.Fatalf("migration %d: %v", version, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func text(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	var result sql.NullString
	if err := db.QueryRow(query).Scan(&result); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return result.String
}

func TestEveryPersonGetsAnOpaqueIDAndKeepsTheirBallot(t *testing.T) {
	db := openDatabase(t, filepath.Join(t.TempDir(), "nominations.sqlite3"))
	runMigration(t, db, 1)
	if _, err := db.Exec(firstSchemaBallot); err != nil {
		t.Fatal(err)
	}
	nominationsBefore, aliasesBefore, rulesBefore := text(t, db, everyNomination), text(t, db, everyAlias), text(t, db, outcomeRules)

	runMigration(t, db, 2)

	if got := text(t, db, everyNomination); got != nominationsBefore {
		t.Fatalf("every Nomination keeps its Person's name, status, votes and history:\ngot\n%s\nwant\n%s", got, nominationsBefore)
	}
	if got := text(t, db, everyAlias); got != aliasesBefore {
		t.Fatalf("every alias still names its Person:\ngot\n%s\nwant\n%s", got, aliasesBefore)
	}
	if got := text(t, db, outcomeRules); got != rulesBefore {
		t.Fatalf("the outcome rules are recreated as migration 1 made them:\ngot\n%s\nwant\n%s", got, rulesBefore)
	}
	if got := text(t, db, "SELECT COUNT(*) FROM pragma_foreign_key_check"); got != "0" {
		t.Fatalf("no row refers to a missing Person, got %s violations", got)
	}

	ids := map[string]string{}
	rows, err := db.Query("SELECT id, name FROM persons")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		parsed, err := uuid.Parse(id)
		if err != nil || parsed[6]>>4 != 7 || parsed[8]>>6 != 0b10 || parsed.String() != id {
			t.Fatalf("Person %q has id %q, which is not a UUIDv7: %v", name, id, err)
		}
		ids[name] = id
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 5 {
		t.Fatalf("every Person keeps one row, got %v", ids)
	}
	if !(ids["Ada Lovelace"] < ids["Grace Hopper"] && ids["Grace Hopper"] < ids["🦄 Rina Okafor ✨"] && ids["🦄 Rina Okafor ✨"] < ids["Zoë Castañeda"]) {
		t.Fatalf("ids follow the order Persons were first nominated, got %v", ids)
	}

	for version := 3; version <= len(sqlite.Migrations()); version++ {
		runMigration(t, db, version)
	}
	if got := text(t, db, everyNomination); got != nominationsBefore {
		t.Fatalf("the later migrations keep every Nomination:\ngot\n%s\nwant\n%s", got, nominationsBefore)
	}
	store := newStore(t, db)
	group := loaded(t, store)
	for spelling, name := range map[domain.Spelling]string{
		"adalovelace": "Ada Lovelace", "countessoflovelace": "Ada Lovelace", "gracehopper": "Grace Hopper",
		"zoëcastañeda": "Zoë Castañeda", "rinaokafor": "🦄 Rina Okafor ✨",
	} {
		if got := group.Spellings()[spelling]; string(got) != ids[name] {
			t.Fatalf("the spelling %q names %s (%s), got %q", spelling, name, ids[name], got)
		}
	}
	first, _ := group.Nomination(1)
	again, _ := group.Nomination(4)
	if first.Person != again.Person || first.Person != domain.PersonID(ids["Ada Lovelace"]) || len(first.Votes) != 3 {
		t.Fatalf("a renominated Person is one Person with their finalized history, got %+v and %+v", first, again)
	}

	err = store.Apply(context.Background(), "m1", func(g *domain.Group) ([]port.Reply, error) {
		_, err := g.Nominate(alex, []domain.Candidate{{Spelling: "rinaokafor", Name: "Rina Okafor"}}, today)
		return nil, err
	})
	var refusal domain.Refusal
	if !errors.As(err, &refusal) || refusal.Rule != domain.RenominatedOnlyThroughRenomination {
		t.Fatalf("an old spelling still finds its Person, so a new nomination of them is refused, got %v", err)
	}

	for _, statement := range []string{
		"UPDATE nominations SET person_id = '" + ids["Grace Hopper"] + "' WHERE number = 1",
		"UPDATE nominations SET status = 'certified' WHERE number = 3",
		"DELETE FROM nominations WHERE number = 2",
	} {
		if _, err := db.Exec(statement); err == nil || !strings.Contains(err.Error(), "permanent") {
			t.Fatalf("%q must be refused as permanent, got %v", statement, err)
		}
	}
	_, err = db.Exec(`INSERT INTO nominations(number, person_id, nominator, status) VALUES(6, ?, 'member-alex', 'nominated')`, ids["Zoë Castañeda"])
	if err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("a Person still has one open Nomination at a time, got %v", err)
	}
}
