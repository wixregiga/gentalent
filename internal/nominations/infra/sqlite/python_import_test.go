package sqlite_test

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/infra/sqlite"
	"gentalent/internal/nominations/port"
)

const pythonSchema = `
CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS nominees(id TEXT PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS aliases(alias TEXT PRIMARY KEY, nominee_id TEXT REFERENCES nominees(id));
CREATE TABLE IF NOT EXISTS denials(nominee_id TEXT PRIMARY KEY REFERENCES nominees(id), recorded_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TRIGGER IF NOT EXISTS keep_denials_delete BEFORE DELETE ON denials BEGIN SELECT RAISE(ABORT,'Denials are permanent'); END;
CREATE TRIGGER IF NOT EXISTS keep_denials_update BEFORE UPDATE ON denials BEGIN SELECT RAISE(ABORT,'Denials are permanent'); END;
CREATE TABLE IF NOT EXISTS cases(
  id INTEGER PRIMARY KEY, nominee_id TEXT NOT NULL UNIQUE REFERENCES nominees(id),
  nominator TEXT NOT NULL, phase TEXT NOT NULL,
  ai_invoked INTEGER NOT NULL DEFAULT 0,
  ai_artifact_dir TEXT NOT NULL DEFAULT '',
  all_votes_at REAL,
  messages_sent INTEGER NOT NULL DEFAULT 0);
CREATE UNIQUE INDEX IF NOT EXISTS one_active_case ON cases((1)) WHERE phase='voting';
CREATE TABLE IF NOT EXISTS votes(
  case_id INTEGER REFERENCES cases(id), sender_id TEXT, vote TEXT NOT NULL,
  PRIMARY KEY(case_id,sender_id));
CREATE TABLE IF NOT EXISTS exhibits(
  id INTEGER PRIMARY KEY, case_id INTEGER REFERENCES cases(id),
  text TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS processed(message_id TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS outbox(
  id TEXT PRIMARY KEY, text TEXT NOT NULL, status TEXT NOT NULL,
  pending_id TEXT, final_id TEXT, kind TEXT NOT NULL,
  attachment_path TEXT NOT NULL DEFAULT '',
  case_id INTEGER);
CREATE TABLE IF NOT EXISTS shames(
  id INTEGER PRIMARY KEY, case_id INTEGER NOT NULL REFERENCES cases(id),
  shamer_id TEXT NOT NULL, target_id TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '',
  target_vote TEXT, shamed_message_id TEXT, shamed_text TEXT, shamed_at TEXT,
  gif TEXT NOT NULL DEFAULT '', message_id TEXT NOT NULL, created_at TEXT NOT NULL,
  UNIQUE(case_id, shamer_id));
CREATE TRIGGER IF NOT EXISTS keep_shames_delete BEFORE DELETE ON shames BEGIN SELECT RAISE(ABORT,'Shames are permanent'); END;
CREATE TRIGGER IF NOT EXISTS keep_shames_update BEFORE UPDATE ON shames BEGIN SELECT RAISE(ABORT,'Shames are permanent'); END;
CREATE TABLE IF NOT EXISTS reaction_votes(
  case_id INTEGER NOT NULL REFERENCES cases(id), member_id TEXT NOT NULL, vote TEXT,
  PRIMARY KEY(case_id, member_id));
CREATE TABLE IF NOT EXISTS feedback(
  id INTEGER PRIMARY KEY, message_id TEXT NOT NULL UNIQUE, sender_id TEXT NOT NULL,
  kind TEXT NOT NULL, text TEXT NOT NULL, said_at TEXT NOT NULL);
`

const pythonRows = `
INSERT INTO meta VALUES('binding', 'binding-hash'), ('cursor', 'cursor-1'), ('armed', '2026-11-01T00:00:00+00:00');
INSERT INTO nominees VALUES('nominee:adalovelace', 'Ada Lovelace'), ('nominee:gracehopper', 'Grace Hopper'),
  ('nominee:katherinejohnson', 'Katherine Johnson');
INSERT INTO aliases VALUES('adalovelace', 'nominee:adalovelace'), ('gracehopper', 'nominee:gracehopper'),
  ('katherinejohnson', 'nominee:katherinejohnson');
INSERT INTO cases VALUES(1, 'nominee:adalovelace', 'member-alex', 'denied', 0, '', 1763380800.5, 4);
INSERT INTO cases VALUES(2, 'nominee:gracehopper', 'member-blair', 'certified', 1, '/media/old/grace', 1763381000, 5);
INSERT INTO cases VALUES(7, 'nominee:katherinejohnson', 'member-casey', 'voting', 1, '', NULL, 2);
INSERT INTO denials VALUES('nominee:adalovelace', '2026-11-17 12:01:00');
INSERT INTO votes VALUES(1, 'member-alex', 'yes'), (1, 'member-blair', 'yes'), (1, 'member-casey', 'no'),
  (2, 'member-alex', 'yes'), (2, 'member-blair', 'yes'), (2, 'member-casey', 'yes'),
  (7, 'member-casey', 'yes'), (7, 'member-alex', 'no');
INSERT INTO exhibits VALUES(1, 1, 'Engines'), (2, 2, 'Compilers'), (5, 2, 'AI media (codex): three photos [sources: /media/old/grace/SOURCES.txt]');
INSERT INTO processed VALUES('message-1'), ('message-2');
INSERT INTO outbox VALUES('a1b2c3d4e5f6', '@gentalent-bot says: hi', 'sent', 'pending-1', 'final-1', 'reply', '', 1);
INSERT INTO shames VALUES(1, 1, 'member-blair', 'member-casey', 'the no', 'no', 'message-9', 'nope',
  '2026-11-17T12:00:30.000Z', 'bell.gif', 'message-10', '2026-11-17T12:02:00.000Z');
INSERT INTO shames VALUES(2, 2, 'member-alex', 'member-blair', '', NULL, NULL, NULL, NULL, '', 'message-11', '');
INSERT INTO reaction_votes VALUES(7, 'member-alex', 'no'), (7, 'member-blair', NULL);
INSERT INTO feedback VALUES(1, 'message-12', 'member-blair', 'suggestion', 'show who voted last', '2026-11-17T13:00:00Z'),
  (2, 'message-13', 'member-casey', 'feedback', 'more GIFs', '');
`

func pythonDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gentalent.sqlite3")
	db := openDatabase(t, path)
	for _, script := range []string{pythonSchema, pythonRows} {
		if _, err := db.Exec(script); err != nil {
			t.Fatalf("build the Python database: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func fingerprint(t *testing.T, path string) [32]byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(content)
}

func TestImportKeepsTheWholePythonHistory(t *testing.T) {
	python := pythonDatabase(t)
	before := fingerprint(t, python)
	db := migrated(t, python)
	if fingerprint(t, python) != before {
		t.Fatal("D5 the Python database is only read")
	}
	store := newStore(t, db)
	group := loaded(t, store)

	at := func(text string) time.Time {
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	want := []domain.Nomination{
		{
			Number: 1, Person: domain.Person{ID: "adalovelace", Name: "Ada Lovelace"}, Nominator: alex, Status: domain.Rejected,
			Votes:    map[domain.MemberID]domain.Vote{alex: domain.Yes, blair: domain.Yes, casey: domain.No},
			LockedAt: time.Unix(1763380800, 500000000), FinalizedAt: at("2026-11-17T12:01:00Z"), Exhibits: []string{"Engines"},
		},
		{
			Number: 2, Person: domain.Person{ID: "gracehopper", Name: "Grace Hopper"}, Nominator: blair, Status: domain.Certified,
			Votes:    map[domain.MemberID]domain.Vote{alex: domain.Yes, blair: domain.Yes, casey: domain.Yes},
			LockedAt: time.Unix(1763381000, 0), Photos: domain.PhotosFound, Strip: "/media/old/grace/strip.jpg",
			Exhibits: []string{"Compilers", "AI media (codex): three photos [sources: /media/old/grace/SOURCES.txt]"},
		},
		{
			Number: 7, Person: domain.Person{ID: "katherinejohnson", Name: "Katherine Johnson"}, Nominator: casey, Status: domain.Nominated,
			Votes:     map[domain.MemberID]domain.Vote{casey: domain.Yes, alex: domain.No},
			Reactions: map[domain.MemberID]domain.Vote{alex: domain.No},
			Photos:    domain.PhotosFailed,
		},
	}
	got := group.Nominations()
	for i := range got {
		got[i].LockedAt, want[i].LockedAt = got[i].LockedAt.UTC(), want[i].LockedAt.UTC()
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("D15 D23 B174 to B184 every case keeps its number, phase, votes, photos and exhibits:\ngot  %+v\nwant %+v", got, want)
	}
	wantShames := []domain.Shame{
		{ID: 1, Nomination: 1, By: blair, Target: casey, TargetVote: domain.No, Reason: "the no", GIF: "bell.gif", MessageID: "message-10",
			Shamed: domain.ShamedMessage{ID: "message-9", Text: "nope", SentAt: at("2026-11-17T12:00:30Z")}, At: at("2026-11-17T12:02:00Z")},
		{ID: 2, Nomination: 2, By: alex, Target: blair, MessageID: "message-11"},
	}
	if shames := group.Shames(); !reflect.DeepEqual(shames, wantShames) {
		t.Fatalf("B183 every shame column survives:\ngot  %+v\nwant %+v", shames, wantShames)
	}
	if uses := group.GifUses(); uses["bell.gif"] != 1 {
		t.Fatalf("B107 GIF usage survives, got %v", uses)
	}

	reader := sqlite.NewReader(db)
	entries, err := reader.Feedback(context.Background())
	wantFeedback := []port.Feedback{
		{MessageID: "message-12", Sender: blair, Kind: "suggestion", Text: "show who voted last", SaidAt: at("2026-11-17T13:00:00Z")},
		{MessageID: "message-13", Sender: casey, Kind: "feedback", Text: "more GIFs"},
	}
	if err != nil || !reflect.DeepEqual(entries, wantFeedback) {
		t.Fatalf("B185 feedback survives, got %+v, %v", entries, err)
	}
	names, err := reader.Names(context.Background())
	if err != nil || len(names) != 3 || names[0] != (port.Name{Alias: "katherinejohnson", Person: "katherinejohnson", Display: "Katherine Johnson", Open: true}) {
		t.Fatalf("B175 every alias points at its Person, got %+v, %v", names, err)
	}

	apply(t, store, "message-new", func(g *domain.Group) error {
		opened, err := g.Nominate(blair, []domain.Candidate{{Person: "dorothyvaughan", Name: "Dorothy Vaughan"}}, time.Now())
		if err == nil && opened[0].Number != 8 {
			t.Fatalf("Q17 new numbers continue after the highest Python case, got %d", opened[0].Number)
		}
		return err
	})
}

func TestImportRefusesADenialWithoutItsCase(t *testing.T) {
	python := pythonDatabase(t)
	db := openDatabase(t, python)
	if _, err := db.Exec("INSERT INTO nominees VALUES('nominee:dorothyvaughan', 'Dorothy Vaughan'); INSERT INTO denials VALUES('nominee:dorothyvaughan', '2026-11-17 12:00:00')"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	target := openDatabase(t, filepath.Join(t.TempDir(), "nominations.sqlite3"))
	tx, err := target.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := sqlite.Migrations(python)[0](context.Background(), tx); err == nil {
		t.Fatal("an import that would lose a denial must fail")
	}
}

func TestImportWithoutAPythonDatabaseStartsEmpty(t *testing.T) {
	store := newStore(t, migrated(t, filepath.Join(t.TempDir(), "missing.sqlite3")))
	if got := loaded(t, store).Nominations(); len(got) != 0 {
		t.Fatalf("no Python database means no history, got %+v", got)
	}
}
