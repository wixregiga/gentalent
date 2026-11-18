package sqlite

import (
	"testing"
	"time"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

func TestInboxAndCursorCommitTogether(t *testing.T) {
	ctx := t.Context()
	s := openStore(t)
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	armed, err := s.Arm(ctx, port.Baseline{Cursor: "before", ArmedAt: stamp})
	must(t, err)
	if !armed {
		t.Fatal("could not arm fresh database")
	}
	later := port.InboxItem{Message: domain.Incoming{ID: "later", SentAt: stamp}, SortKey: "2"}
	first := port.InboxItem{Message: domain.Incoming{ID: "first", SentAt: stamp}, SortKey: "1"}
	invalid := port.InboxItem{Message: domain.Incoming{ID: "bad", SentAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}
	if err := s.SavePage(ctx, "before", "bad", []port.InboxItem{later, invalid}); err == nil {
		t.Fatal("invalid page committed")
	}
	baseline, err := s.Baseline(ctx)
	must(t, err)
	next, err := s.Next(ctx)
	must(t, err)
	if baseline.Cursor != "before" || next != nil {
		t.Fatalf("failed page committed: cursor %q, next %+v", baseline.Cursor, next)
	}
	must(t, s.SavePage(ctx, "before", "after", []port.InboxItem{later, first}))
	if err := s.SavePage(ctx, "before", "stale", []port.InboxItem{first}); err == nil {
		t.Fatal("stale poll advanced cursor")
	}
	next, err = s.Next(ctx)
	must(t, err)
	if next == nil || next.ID != "first" {
		t.Fatalf("unordered inbox: %+v", next)
	}
	restarted := New(s.db)
	next, err = restarted.Next(ctx)
	must(t, err)
	if next == nil || next.ID != "first" {
		t.Fatalf("unhandled message did not survive restart: %+v", next)
	}
	must(t, restarted.Handled(ctx, "first"))
	next, err = restarted.Next(ctx)
	must(t, err)
	if next == nil || next.ID != "later" {
		t.Fatalf("acknowledgement did not advance inbox: %+v", next)
	}
}
