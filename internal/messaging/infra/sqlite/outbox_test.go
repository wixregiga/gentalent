package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"gentalent/internal/messaging/domain"

	_ "modernc.org/sqlite"
)

func openStore(t *testing.T, source string) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "messaging.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := Migrations(source)[0](t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return New(db)
}

func TestRestartNeverRequeuesClaimedReply(t *testing.T) {
	s := openStore(t, "")
	ctx := t.Context()
	must(t, s.StartWorker(ctx, "binding"))
	must(t, s.Enqueue(ctx, []domain.Outgoing{{Key: "a", Text: "first"}, {Key: "b", Text: "second"}}))
	change(t, ctx, s, "a", domain.Queued, domain.Sending, "")
	if changed, err := s.Transition(ctx, "a", domain.Queued, domain.Sending, ""); changed || err != nil {
		t.Fatalf("second worker claimed the reply: %v, %v", changed, err)
	}
	must(t, s.Enqueue(ctx, []domain.Outgoing{{Key: "a", Text: "duplicate"}}))
	must(t, s.StartWorker(ctx, "binding"))
	a, err := s.Reply(ctx, "a")
	must(t, err)
	active, err := s.ActiveReplies(ctx)
	must(t, err)
	if a.Text != "first" || a.State != domain.Uncertain || len(active) != 1 || active[0].Key != "b" {
		t.Fatalf("restart lost delivery state: %+v, %+v", a, active)
	}
	if err := s.StartWorker(ctx, "other binding"); err == nil {
		t.Fatal("opened another group's database")
	}
}

func change(t *testing.T, ctx context.Context, s *Store, key string, from, to domain.State, id string) {
	t.Helper()
	ok, err := s.Transition(ctx, key, from, to, id)
	if err != nil || !ok {
		t.Fatalf("transition %s %s to %s: %v, %v", key, from, to, ok, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
