package sqlite

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

func TestImportPythonDatabasePreservesHistoryAndAnnouncementTags(t *testing.T) {
	source := filepath.Join(t.TempDir(), "python # ballot.sqlite3")
	python := exec.CommandContext(t.Context(), "python3", "-B", "-c", `
import sqlite3
import sys
from gentalent.state import SCHEMA
with sqlite3.connect(sys.argv[1]) as db:
    db.executescript(SCHEMA)
    db.executescript("""
        INSERT INTO meta VALUES ('binding','binding'),('cursor','opaque/?'),('armed','2026-01-02T03:04:05.067+00:00'),('other','keep');
        INSERT INTO nominees VALUES ('nominee:ada','Ada Lovelace');
        INSERT INTO aliases VALUES ('ada','nominee:ada');
        INSERT INTO denials(nominee_id) VALUES ('nominee:ada');
        INSERT INTO cases(id,nominee_id,nominator,phase) VALUES (3,'nominee:ada','alex','denied');
        INSERT INTO votes VALUES (3,'alex','no');
        INSERT INTO exhibits(case_id,text) VALUES (3,'pitch');
        INSERT INTO processed VALUES ('already-done');
        INSERT INTO outbox VALUES ('announcement','@gentalent-bot says: announced','sent','old-pending','final-photo','reply','media/strip.jpg',3);
        INSERT INTO outbox VALUES ('other-reply','@gentalent-bot says: later','uncertain','later-pending',NULL,'shame','media/demo.gif',3);
        INSERT INTO outbox VALUES ('interrupted','@gentalent-bot says: unfinished','sending',NULL,NULL,'reply','',NULL);
        INSERT INTO outbox VALUES ('pending','@gentalent-bot says: waiting','pending','old-lookup',NULL,'reply','',NULL);
        INSERT INTO outbox VALUES ('failed','@gentalent-bot says: failed','failed','rejected-lookup','rejected','reply','',NULL);
        INSERT INTO outbox VALUES ('previewed','@gentalent-bot says: preview','preview',NULL,NULL,'reply','',NULL);
        INSERT INTO outbox VALUES ('queued','@gentalent-bot says: next','queued',NULL,NULL,'reply','',NULL);
        INSERT INTO shames(case_id,shamer_id,target_id,message_id,created_at) VALUES (3,'blair','alex','message','2026-01-02T03:04:05Z');
        INSERT INTO reaction_votes VALUES (3,'casey','no');
        INSERT INTO feedback(message_id,sender_id,kind,text,said_at) VALUES ('idea','alex','feedback','idea','2026-01-02T03:04:05Z');
    """)
`, source)
	python.Dir = "../../../.."
	if out, err := python.CombinedOutput(); err != nil {
		t.Fatalf("build Python database: %v\n%s", err, out)
	}
	before, err := os.ReadFile(source)
	must(t, err)
	s := openStore(t, source)
	ctx := t.Context()
	baseline, err := s.Baseline(ctx)
	must(t, err)
	if baseline.Cursor != "opaque/?" || !baseline.ArmedAt.Equal(time.Date(2026, 1, 2, 3, 4, 5, 67000000, time.UTC)) {
		t.Fatalf("baseline = %+v", baseline)
	}
	var other string
	must(t, s.db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='other'").Scan(&other))
	if other != "keep" {
		t.Fatalf("metadata did not survive: %s", other)
	}
	announcement, err := s.TaggedMessage(ctx, "announces/3")
	must(t, err)
	if announcement != "final-photo" {
		t.Fatalf("legacy announcement = %s", announcement)
	}
	replies, err := s.replies(ctx, "SELECT "+replyColumns+" FROM outbox ORDER BY rowid")
	must(t, err)
	if len(replies) != 7 || replies[0].Key != "announcement" || replies[0].PendingID != "old-pending" ||
		replies[1].Key != "other-reply" || replies[1].Tag != "" || replies[1].State != domain.Uncertain ||
		replies[1].PendingID != "later-pending" || replies[1].Kind != "shame" || replies[1].Attachment != "media/demo.gif" ||
		replies[6].Key != "queued" {
		t.Fatalf("replies lost, reordered, or mistagged: %+v", replies)
	}
	must(t, s.StartWorker(ctx, "binding"))
	interrupted, err := s.Reply(ctx, "interrupted")
	must(t, err)
	if interrupted.State != domain.Uncertain {
		t.Fatalf("interrupted reply = %+v", interrupted)
	}
	named, err := s.KnownAttachment(ctx, "demo.gif")
	must(t, err)
	if !named {
		t.Fatal("lost known attachment")
	}
	must(t, s.SavePage(ctx, "opaque/?", "advanced", []port.InboxItem{
		{Message: domain.Incoming{ID: "already-done", SentAt: baseline.ArmedAt}},
		{Message: domain.Incoming{ID: "new-message", SentAt: baseline.ArmedAt}},
	}))
	next, err := s.Next(ctx)
	must(t, err)
	if next == nil || next.ID != "new-message" {
		t.Fatalf("migrated message replayed: %+v", next)
	}
	must(t, s.Handled(ctx, next.ID))
	must(t, s.SavePage(ctx, "advanced", "again", []port.InboxItem{{Message: *next}}))
	next, err = s.Next(ctx)
	must(t, err)
	if next != nil {
		t.Fatalf("acknowledged message replayed: %+v", next)
	}
	after, err := os.ReadFile(source)
	must(t, err)
	if !bytes.Equal(before, after) {
		t.Fatal("source database changed")
	}
}
