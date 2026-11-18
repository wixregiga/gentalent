package main

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	mdomain "gentalent/internal/messaging/domain"
	"gentalent/internal/shared/sqlitedb"
)

type databaseShape struct {
	version int
	objects []string
}

func shapeOf(t *testing.T, ctx context.Context, path string) databaseShape {
	t.Helper()
	db, err := sqlitedb.Open(ctx, path, sqlitedb.ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	version, err := sqlitedb.Version(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(ctx, "SELECT type || ' ' || name || ' ' || COALESCE(sql, '') FROM sqlite_master ORDER BY type, name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	shape := databaseShape{version: version}
	for rows.Next() {
		var object string
		if err := rows.Scan(&object); err != nil {
			t.Fatal(err)
		}
		shape.objects = append(shape.objects, object)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return shape
}

func hasTables(shape databaseShape, names ...string) bool {
	for _, name := range names {
		if !slices.ContainsFunc(shape.objects, func(object string) bool { return strings.HasPrefix(object, "table "+name+" ") }) {
			return false
		}
	}
	return true
}

func TestAMigratedDatabaseReopensUnchanged(t *testing.T) {
	ctx := t.Context()
	cfg := testConfig(t)
	first, err := openWorker(ctx, cfg, true, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.inbox.Enqueue(ctx, []mdomain.Outgoing{{Key: "reply-1", Text: "kept"}}); err != nil {
		t.Fatal(err)
	}
	if err := first.close(); err != nil {
		t.Fatal(err)
	}
	nominations, messaging := shapeOf(t, ctx, cfg.NominationsDatabase), shapeOf(t, ctx, cfg.MessagingDatabase)
	if nominations.version != 1 || !hasTables(nominations, "meta", "persons", "aliases", "nominations", "votes", "reactions", "exhibits", "shames", "feedback", "handled", "outbox") {
		t.Fatalf("a fresh nominations database gets the whole schema at version 1: %+v", nominations)
	}
	if messaging.version != 1 || !hasTables(messaging, "meta", "processed", "inbox", "outbox") {
		t.Fatalf("a fresh messaging database gets the whole schema at version 1: %+v", messaging)
	}

	second, err := openWorker(ctx, cfg, true, time.Now)
	if err != nil {
		t.Fatalf("a migrated database reopens: %v", err)
	}
	defer second.close()
	reply, err := second.inbox.Reply(ctx, "reply-1")
	if err != nil || reply.Text != "kept" {
		t.Fatalf("a reopened database keeps its rows: %+v %v", reply, err)
	}
	if again := shapeOf(t, ctx, cfg.NominationsDatabase); !slices.Equal(again.objects, nominations.objects) || again.version != nominations.version {
		t.Fatalf("reopening changed the nominations schema: %+v", again)
	}
	if again := shapeOf(t, ctx, cfg.MessagingDatabase); !slices.Equal(again.objects, messaging.objects) || again.version != messaging.version {
		t.Fatalf("reopening changed the messaging schema: %+v", again)
	}
}
