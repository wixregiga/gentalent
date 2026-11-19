package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/infra/sqlite"
	"gentalent/internal/nominations/port"
)

var (
	alex, blair, casey = domain.MemberID("member-alex"), domain.MemberID("member-blair"), domain.MemberID("member-casey")
	members            = []domain.MemberID{alex, blair, casey}
	settings           = domain.Settings{GroupOpenNominationLimit: 2, MemberOpenNominationLimit: 2, SingleMessageNominationLimit: 3, AllowRenomination: true, AllowNameFixes: true, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}
	today              = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
)

func openDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	dsn := (&url.URL{Scheme: "file", Opaque: path, RawQuery: "_pragma=foreign_keys(1)"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func migrated(t *testing.T) *sql.DB {
	t.Helper()
	db := openDatabase(t, filepath.Join(t.TempDir(), "nominations.sqlite3"))
	for i, migration := range sqlite.Migrations() {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migration(context.Background(), tx); err != nil {
			tx.Rollback()
			t.Fatalf("migration %d: %v", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func newStore(t *testing.T, db *sql.DB) *sqlite.Store {
	t.Helper()
	store, err := sqlite.NewStore(context.Background(), db, "group-test", members, settings)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func apply(t *testing.T, store *sqlite.Store, messageID string, change func(*domain.Group) error, replies ...port.Reply) {
	t.Helper()
	err := store.Apply(context.Background(), messageID, func(group *domain.Group) ([]port.Reply, error) {
		return replies, change(group)
	})
	if err != nil {
		t.Fatalf("Apply %s: %v", messageID, err)
	}
}

func loaded(t *testing.T, store *sqlite.Store) *domain.Group {
	t.Helper()
	var group *domain.Group
	apply(t, store, "", func(g *domain.Group) error { group = g; return nil })
	return group
}

func ada() []domain.Candidate {
	return []domain.Candidate{{Spelling: "adalovelace", NewPerson: "person-ada", Name: "Ada Lovelace", Pitch: "Engines", WithPhotos: true}}
}

func TestStoreKeepsEveryChangeOfTheGroup(t *testing.T) {
	store := newStore(t, migrated(t))
	apply(t, store, "message-1", func(g *domain.Group) error { _, err := g.Nominate(alex, ada(), today); return err })
	apply(t, store, "message-2", func(g *domain.Group) error {
		_, err := g.CastVote(blair, []domain.Ballot{{Vote: domain.Yes}}, today)
		return err
	})
	apply(t, store, "", func(g *domain.Group) error {
		_, err := g.CountReactions(1, map[domain.MemberID]domain.Vote{casey: domain.No}, today.Add(time.Second))
		return err
	})
	apply(t, store, "", func(g *domain.Group) error { _, err := g.ClaimPhotoSearches(); return err })
	apply(t, store, "", func(g *domain.Group) error {
		_, err := g.AttachPhotos(1, "/media/nominations/1-ada/strip.jpg", "AI media (codex)")
		return err
	})
	apply(t, store, "message-3", func(g *domain.Group) error { _, err := g.Finalize(alex, nil, today.Add(2*time.Second)); return err })
	apply(t, store, "message-4", func(g *domain.Group) error {
		_, err := g.Shame(domain.Shame{By: blair, Target: casey, Reason: "the no", GIF: "bell.gif", MessageID: "message-4", At: today,
			Shamed: domain.ShamedMessage{ID: "message-x", Text: "nope", SentAt: today}}, domain.Ref{})
		return err
	})

	got := loaded(t, store).Nominations()[0]
	want := domain.Nomination{
		Number: 1, Person: domain.Person{ID: "person-ada", Name: "Ada Lovelace"}, Nominator: alex, Status: domain.Rejected,
		Votes:       map[domain.MemberID]domain.Vote{alex: domain.Yes, blair: domain.Yes, casey: domain.No},
		Reactions:   map[domain.MemberID]domain.Vote{casey: domain.No},
		NominatedAt: today, LockedAt: today.Add(time.Second), FinalizedAt: today.Add(2 * time.Second),
		Pitch: "Engines", Exhibits: []string{"Engines", "AI media (codex)"}, Photos: domain.PhotosFound,
		Strip: "/media/nominations/1-ada/strip.jpg",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the reloaded Nomination differs:\ngot  %+v\nwant %+v", got, want)
	}
	shames := loaded(t, store).Shames()
	if len(shames) != 1 || shames[0].TargetVote != domain.No || shames[0].Shamed.Text != "nope" || !shames[0].At.Equal(today) {
		t.Fatalf("B106 the shame is kept as recorded, got %+v", shames)
	}
}

func TestAHandledMessageIsNeverAppliedTwice(t *testing.T) {
	db := migrated(t)
	store := newStore(t, db)
	nominate := func(g *domain.Group) error { _, err := g.Nominate(alex, ada(), today); return err }
	apply(t, store, "message-1", nominate, port.Reply{Key: "message-1/1", Text: "announced"})
	ran := false
	apply(t, store, "message-1", func(*domain.Group) error { ran = true; return nil }, port.Reply{Key: "message-1/1", Text: "again"})
	if ran {
		t.Fatal("B181 a handled message is skipped")
	}
	replies, err := sqlite.NewReader(db).Unrelayed(context.Background())
	if err != nil || len(replies) != 1 || replies[0].Text != "announced" {
		t.Fatalf("the replies commit with the change, once, got %+v, %v", replies, err)
	}
}

func TestAFailedChangeCommitsNothing(t *testing.T) {
	db := migrated(t)
	store := newStore(t, db)
	err := store.Apply(context.Background(), "message-1", func(g *domain.Group) ([]port.Reply, error) {
		if _, err := g.Nominate(alex, ada(), today); err != nil {
			return nil, err
		}
		return []port.Reply{{Key: "message-1/1"}}, errors.New("wording failed")
	})
	if err == nil {
		t.Fatal("the failure must be returned")
	}
	if len(loaded(t, store).Nominations()) != 0 {
		t.Fatal("D22 a failed message changes nothing")
	}
	ran := false
	apply(t, store, "message-1", func(*domain.Group) error { ran = true; return nil })
	if !ran {
		t.Fatal("D22 a failed message is not recorded as handled, so it is retried")
	}
}

func TestOutcomesAndShamesArePermanent(t *testing.T) {
	db := migrated(t)
	store := newStore(t, db)
	apply(t, store, "message-1", func(g *domain.Group) error { _, err := g.Nominate(alex, ada(), today); return err })
	apply(t, store, "message-2", func(g *domain.Group) error {
		_, err := g.CastVote(blair, []domain.Ballot{{Vote: domain.No}}, today)
		if err == nil {
			_, err = g.CastVote(casey, []domain.Ballot{{Vote: domain.No}}, today)
		}
		return err
	})
	apply(t, store, "message-3", func(g *domain.Group) error { _, err := g.Finalize(alex, nil, today); return err })
	apply(t, store, "message-4", func(g *domain.Group) error {
		_, err := g.Shame(domain.Shame{By: blair, Target: casey, MessageID: "message-4", At: today}, domain.Ref{Number: 1})
		return err
	})
	for _, statement := range []string{
		"UPDATE nominations SET status = 'certified' WHERE number = 1",
		"DELETE FROM nominations WHERE number = 1",
		"UPDATE shames SET reason = 'edited'",
		"DELETE FROM shames",
	} {
		_, err := db.Exec(statement)
		if err == nil || !strings.Contains(err.Error(), "permanent") {
			t.Fatalf("D24 B176 B183 %q must be refused as permanent, got %v", statement, err)
		}
	}
	if _, err := db.Exec("UPDATE nominations SET photos = 'failed' WHERE number = 1"); err != nil {
		t.Fatalf("photo details of a finished Nomination may still change: %v", err)
	}
}

func TestTheOutboxRelaysEachReplyOnce(t *testing.T) {
	db := migrated(t)
	store := newStore(t, db)
	reader := sqlite.NewReader(db)
	ctx := context.Background()
	if err := store.Answer(ctx, "message-1", []port.Reply{{Key: "message-1/1", Text: "first"}, {Key: "message-1/2", Attachment: "/a.jpg", Announces: 3}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Answer(ctx, "message-1", []port.Reply{{Key: "message-1/3", Text: "again"}}); err != nil {
		t.Fatal(err)
	}
	replies, err := reader.Unrelayed(ctx)
	want := []port.Reply{{Key: "message-1/1", Text: "first"}, {Key: "message-1/2", Attachment: "/a.jpg", Announces: 3}}
	if err != nil || !reflect.DeepEqual(replies, want) {
		t.Fatalf("unrelayed replies in order, once per message, got %+v, %v", replies, err)
	}
	if err := store.MarkRelayed(ctx, []string{"message-1/1", "message-1/2"}); err != nil {
		t.Fatal(err)
	}
	if replies, _ := reader.Unrelayed(ctx); len(replies) != 0 {
		t.Fatalf("relayed replies are not relayed again, got %+v", replies)
	}
}

func TestFeedbackIsLoggedOnce(t *testing.T) {
	db := migrated(t)
	store := newStore(t, db)
	ctx := context.Background()
	entry := port.Feedback{MessageID: "message-1", Sender: alex, Kind: "suggestion", Text: "show who voted last", SaidAt: today}
	for range 2 {
		if err := store.Log(ctx, entry, []port.Reply{{Key: "message-1/1", Text: "thanks"}}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := sqlite.NewReader(db).Feedback(ctx)
	if err != nil || len(entries) != 1 || !reflect.DeepEqual(entries[0], entry) {
		t.Fatalf("B113 one entry per message, got %+v, %v", entries, err)
	}
}

func TestTheDatabaseBelongsToOneGroup(t *testing.T) {
	db := migrated(t)
	newStore(t, db)
	if _, err := sqlite.NewStore(context.Background(), db, "another-group", members, settings); err == nil {
		t.Fatal("B173 a database of another Group is refused")
	}
}

func TestALimitBelowOneRefusesToOpenTheStore(t *testing.T) {
	low := settings
	low.MemberOpenNominationLimit = 0
	_, err := sqlite.NewStore(context.Background(), migrated(t), "group-test", members, low)
	var below domain.LimitBelowOne
	if !errors.As(err, &below) || below.Limit != domain.MemberOpenNominationLimit {
		t.Fatalf("R9 the store names the limit below one, got %v", err)
	}
}

func TestReadStanding(t *testing.T) {
	db := migrated(t)
	store := newStore(t, db)
	grace := []domain.Candidate{{Spelling: "gracehopper", NewPerson: "person-grace", Name: "Grace Hopper"}}
	reject := func(g *domain.Group, number domain.NominationNumber) error {
		if _, err := g.CastVote(blair, []domain.Ballot{{Nomination: domain.Ref{Number: number}, Vote: domain.No}}, today); err != nil {
			return err
		}
		if _, err := g.CastVote(casey, []domain.Ballot{{Nomination: domain.Ref{Number: number}, Vote: domain.No}}, today); err != nil {
			return err
		}
		_, err := g.Finalize(alex, []domain.Ref{{Number: number}}, today)
		return err
	}
	apply(t, store, "m1", func(g *domain.Group) error { _, err := g.Nominate(alex, ada(), today); return err })
	apply(t, store, "m2", func(g *domain.Group) error { return reject(g, 1) })
	apply(t, store, "m3", func(g *domain.Group) error { _, err := g.Nominate(alex, grace, today); return err })
	apply(t, store, "m4", func(g *domain.Group) error {
		for _, m := range []domain.MemberID{blair, casey} {
			if _, err := g.CastVote(m, []domain.Ballot{{Vote: domain.Yes}}, today); err != nil {
				return err
			}
		}
		_, err := g.Finalize(alex, nil, today)
		return err
	})
	apply(t, store, "m5", func(g *domain.Group) error { _, err := g.Renominate(blair, ada(), today); return err })
	apply(t, store, "", func(g *domain.Group) error { _, err := g.ClaimPhotoSearches(); return err })

	standing, err := sqlite.NewReader(db).Standing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(standing.Certified, []string{"Grace Hopper"}) || !reflect.DeepEqual(standing.Rejected, []string{"Ada Lovelace"}) {
		t.Fatalf("D17 D24 a renominated Person keeps the status of their latest finalized Nomination while open, got %+v", standing)
	}
	if len(standing.Open) != 1 || standing.Open[0].Number != 3 || standing.Open[0].Votes[blair] != domain.Yes || standing.Latest.Number != 3 {
		t.Fatalf("Q24 the open Nomination with its votes, got %+v", standing)
	}
	if !reflect.DeepEqual(standing.Searching, []domain.NominationNumber{1, 3}) {
		t.Fatalf("D27 running photo searches are reported, got %v", standing.Searching)
	}
	every, err := sqlite.NewReader(db).Nominations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	listed := make([]string, len(every))
	for i, view := range every {
		listed[i] = fmt.Sprintf("%d %s %s %d", view.Number, view.Person, view.Status, len(view.Votes))
	}
	if want := []string{"1 Ada Lovelace rejected 3", "2 Grace Hopper certified 3", "3 Ada Lovelace nominated 1"}; !reflect.DeepEqual(listed, want) {
		t.Fatalf("B158 every Nomination with its status and vote count, got %q", listed)
	}
	names, err := sqlite.NewReader(db).Names(context.Background())
	if err != nil || len(names) != 2 || names[0].Person != "person-ada" || names[1].Display != "Grace Hopper" || !names[0].Open || names[1].Open {
		t.Fatalf("names come latest Nomination first and say whose Nomination is open, got %+v, %v", names, err)
	}
}
