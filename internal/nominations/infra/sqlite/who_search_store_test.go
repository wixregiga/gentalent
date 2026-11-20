package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/infra/sqlite"
	"gentalent/internal/nominations/port"
)

func whoStore(t *testing.T, db *sql.DB, limit int) *sqlite.Store {
	t.Helper()
	limited := settings
	limited.WhoSearchDailyLimit = limit
	store, err := sqlite.NewStore(context.Background(), db, "group-test", members, limited)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func applyWho(t *testing.T, store *sqlite.Store, change func(*domain.Group) (domain.WhoSearch, error)) domain.WhoSearch {
	t.Helper()
	var search domain.WhoSearch
	apply(t, store, "", func(g *domain.Group) error {
		var err error
		search, err = change(g)
		return err
	})
	return search
}

func reloadedWho(t *testing.T, store *sqlite.Store, number domain.WhoSearchNumber) domain.WhoSearch {
	t.Helper()
	search, ok := loaded(t, store).WhoSearch(number)
	if !ok {
		t.Fatalf("who search %d is not kept", number)
	}
	return search
}

func TestAWhoSearchSurvivesAReloadWithItsFigure(t *testing.T) {
	db := migrated(t)
	store := whoStore(t, db, 5)
	apply(t, store, "m1", func(g *domain.Group) error { _, err := g.Nominate(alex, ada(), today); return err })
	ada := loaded(t, store).Spellings()["adalovelace"]

	started := applyWho(t, store, func(g *domain.Group) (domain.WhoSearch, error) {
		return g.SearchWho(blair, "the woman who wrote the first computer program", today.Add(time.Minute))
	})
	if got := reloadedWho(t, store, started.Number); !reflect.DeepEqual(got, started) {
		t.Fatalf("a requested who search reloads as requested:\ngot  %+v\nwant %+v", got, started)
	}
	apply(t, store, "", func(g *domain.Group) error { _, err := g.ClaimWhoSearches(); return err })
	standing, err := sqlite.NewReader(db).Standing(context.Background())
	if err != nil || !reflect.DeepEqual(standing.WhoSearching, []domain.WhoSearchNumber{started.Number}) {
		t.Fatalf("busy sees the running who search, got %v, %v", standing.WhoSearching, err)
	}

	figure := domain.PublicFigure{Name: "Ada Lovelace", Spelling: "adalovelace", Blurb: "A mathematician who wrote the first program.",
		Source: "https://example.org/ada", Photo: "/media/who/1/photo.jpg", Provider: "claude"}
	found := applyWho(t, store, func(g *domain.Group) (domain.WhoSearch, error) { return g.ReceivePublicFigure(started.Number, figure) })
	if found.Figure.Person != ada {
		t.Fatalf("the found figure is the Person the Group already holds, got %+v", found.Figure)
	}
	if got := reloadedWho(t, store, started.Number); !reflect.DeepEqual(got, found) {
		t.Fatalf("a found who search reloads with its figure:\ngot  %+v\nwant %+v", got, found)
	}
	standing, err = sqlite.NewReader(db).Standing(context.Background())
	if err != nil || len(standing.WhoSearching) != 0 {
		t.Fatalf("a found who search is no longer running, got %v, %v", standing.WhoSearching, err)
	}

	if _, err := db.Exec("UPDATE who_searches SET blurb = 'changed' WHERE number = ?", started.Number); err == nil || !strings.Contains(err.Error(), "permanent") {
		t.Fatalf("an ended who search is permanent, got %v", err)
	}
	if _, err := db.Exec("DELETE FROM who_searches WHERE number = ?", started.Number); err == nil || !strings.Contains(err.Error(), "permanent") {
		t.Fatalf("a who search is never deleted, got %v", err)
	}
}

func TestEveryStartedWhoSearchCountsAcrossAReload(t *testing.T) {
	db := migrated(t)
	store := whoStore(t, db, 2)
	first := applyWho(t, store, func(g *domain.Group) (domain.WhoSearch, error) {
		return g.SearchWho(alex, "the rear admiral who popularized the word bug", today)
	})
	applyWho(t, store, func(g *domain.Group) (domain.WhoSearch, error) {
		return g.SearchWho(alex, "the man who sailed around the world alone", today.Add(time.Hour))
	})
	apply(t, store, "", func(g *domain.Group) error { _, err := g.ClaimWhoSearches(); return err })
	apply(t, store, "", func(g *domain.Group) error { _, err := g.AbandonInterruptedWhoSearches(); return err })
	if got := reloadedWho(t, store, first.Number); got.Status != domain.WhoFailed {
		t.Fatalf("an abandoned who search is kept failed, got %+v", got)
	}

	reopened := whoStore(t, db, 2)
	err := reopened.Apply(context.Background(), "", func(g *domain.Group) ([]port.Reply, error) {
		_, err := g.SearchWho(alex, "the woman who wrote the first computer program", today.Add(2*time.Hour))
		return nil, err
	})
	var refused domain.Refusal
	if !errors.As(err, &refused) || refused.Rule != domain.WhoSearchesWithinDailyLimit || !refused.Next.Equal(today.Add(24*time.Hour)) {
		t.Fatalf("failed searches still count after a reload, got %v", err)
	}
	next := applyWho(t, reopened, func(g *domain.Group) (domain.WhoSearch, error) {
		return g.SearchWho(alex, "the woman who wrote the first computer program", today.Add(24*time.Hour))
	})
	if next.Number != 3 {
		t.Fatalf("who search numbers keep counting, got %d", next.Number)
	}
}
