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

func TestStatusAndTagReadOnlyDeliveredEvidence(t *testing.T) {
	s := openStore(t, "")
	ctx := t.Context()
	must(t, s.Enqueue(ctx, []domain.Outgoing{
		{Key: "sent", Text: "sent", Tag: "sent-tag"},
		{Key: "uncertain", Text: "uncertain", Tag: "uncertain-tag"},
		{Key: "failed", Text: "failed", Tag: "failed-tag"},
		{Key: "previewed", Text: "previewed", Tag: "preview-tag"},
		{Key: "queued", Text: "queued"},
	}))
	change(t, ctx, s, "sent", domain.Queued, domain.Sending, "")
	change(t, ctx, s, "sent", domain.Sending, domain.Pending, "pending")
	change(t, ctx, s, "sent", domain.Pending, domain.Sent, "delivered")
	change(t, ctx, s, "uncertain", domain.Queued, domain.Sending, "")
	change(t, ctx, s, "uncertain", domain.Sending, domain.Uncertain, "")
	change(t, ctx, s, "failed", domain.Queued, domain.Sending, "")
	change(t, ctx, s, "failed", domain.Sending, domain.Pending, "failed-pending")
	change(t, ctx, s, "failed", domain.Pending, domain.Failed, "failed-id")
	change(t, ctx, s, "previewed", domain.Queued, domain.Sending, "")
	change(t, ctx, s, "previewed", domain.Sending, domain.Previewed, "")

	for _, tag := range []string{"sent-tag", "uncertain-tag", "failed-tag", "preview-tag", ""} {
		id, err := s.TaggedMessage(ctx, tag)
		must(t, err)
		if (id == "delivered") != (tag == "sent-tag") {
			t.Fatalf("tag %q read message %q", tag, id)
		}
	}
	for id, want := range map[string]string{"pending": "sent-tag", "delivered": "sent-tag", "failed-id": "failed-tag"} {
		tag, known, err := s.ReplyTag(ctx, id)
		must(t, err)
		if !known || tag != want {
			t.Fatalf("D35 message %s is the reply tagged %q, got %q, %v", id, want, tag, known)
		}
	}
	if tag, known, err := s.ReplyTag(ctx, "someone-else"); known || tag != "" || err != nil {
		t.Fatalf("a message the bot never sent is no reply, got %q, %v, %v", tag, known, err)
	}
	report, err := s.UncertainReplies(ctx)
	must(t, err)
	if len(report.Counts) != 5 || report.Counts[domain.Sent] != 1 || len(report.Attention) != 2 ||
		report.Attention[0].Key != "uncertain" || report.Attention[1].Key != "queued" {
		t.Fatalf("status = %+v", report)
	}
	used, err := s.FinalIDUsed(ctx, "uncertain", "delivered")
	must(t, err)
	if !used {
		t.Fatal("missing duplicate final ID")
	}
	if changed, err := s.Transition(ctx, "uncertain", domain.Uncertain, domain.Sent, "delivered"); changed || err != nil {
		t.Fatalf("duplicate resolution accepted: %v, %v", changed, err)
	}
	change(t, ctx, s, "failed", domain.Failed, domain.Sent, "failed-id")
}
