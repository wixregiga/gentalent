package main

import (
	"context"
	"strings"
	"testing"

	"gentalent/internal/shared/sqlitedb"
)

func TestARequestedSearchStrandedByACrashRunsAtTheNextStart(t *testing.T) {
	b := newTestBot(t)
	ctx := t.Context()
	b.chat.say(blairID, "!!gt Grace Hopper. Wrote the first compiler")
	if err := b.engine.poller.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	incoming, err := b.engine.inbox.Next(ctx)
	if err != nil || incoming == nil {
		t.Fatalf("inbox: %v %v", incoming, err)
	}
	if err := b.engine.apply(ctx, *incoming); err != nil {
		t.Fatal(err)
	}
	if err := b.databases.close(); err != nil {
		t.Fatal(err)
	}

	close(b.agent.release)
	restarted := startBot(t, b.cfg, b.chat, b.agent)
	if err := restarted.engine.run(ctx, true); err != nil {
		t.Fatal(err)
	}
	if runs, _ := b.agent.counts(); runs != 1 {
		t.Fatalf("photos-searched-at-most-once-per-nomination: a requested search left by a crash runs once at start, ran %d", runs)
	}
	if photos := restarted.photoReplies(); photos != 3 {
		t.Fatalf("the search's photos are sent, got %d", photos)
	}
}

func TestAFinishedSearchWhoseCommitFailedIsAttachedOnALaterCycle(t *testing.T) {
	b := newTestBot(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer b.engine.photos.stop(cancel)
	b.chat.say(blairID, "!!gt Grace Hopper. Wrote the first compiler")
	if err := b.engine.cycle(ctx); err != nil {
		t.Fatal(err)
	}
	b.waitStarted(1)
	close(b.agent.release)
	result := <-b.engine.photos.results

	db, err := sqlitedb.Open(ctx, b.cfg.NominationsDatabase, sqlitedb.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "CREATE TRIGGER failing_commit BEFORE UPDATE OF photos ON nominations BEGIN SELECT RAISE(ABORT, 'disk busy'); END"); err != nil {
		t.Fatal(err)
	}
	if err := b.engine.attach(ctx, result); err == nil || !strings.Contains(err.Error(), "disk busy") {
		t.Fatalf("the failed commit is reported: %v", err)
	}
	if err := b.engine.cycle(ctx); err == nil {
		t.Fatal("the completion is retried on the next cycle while the commit still fails")
	}
	if _, err := db.ExecContext(ctx, "DROP TRIGGER failing_commit"); err != nil {
		t.Fatal(err)
	}
	b.cycle()
	if photos := b.photoReplies(); photos != 3 || b.engine.photos.running != 0 {
		t.Fatalf("the kept result commits once the database recovers: %d photos, %d running", photos, b.engine.photos.running)
	}
	b.cycle()
	if photos := b.photoReplies(); photos != 3 {
		t.Fatalf("an attached result is never attached twice: %d photos", photos)
	}
}

func (b *testBot) photoReplies() int {
	photos := 0
	for _, reply := range b.chat.delivered() {
		if reply.Text == "" && reply.Attachment != "" {
			photos++
		}
	}
	return photos
}
