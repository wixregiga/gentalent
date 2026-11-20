package domain_test

import (
	"testing"
	"time"

	"gentalent/internal/nominations/domain"
)

func whoLimit(limit int) domain.Settings {
	settings := oneAtATime
	settings.WhoSearchDailyLimit = limit
	return settings
}

func searchWho(t *testing.T, group *domain.Group, by member, at time.Time) domain.WhoSearch {
	t.Helper()
	search, err := group.SearchWho(by.ID, "the woman who wrote the first computer program", at)
	if err != nil {
		t.Fatalf("%s's who search at %s: %v", by.Name, at, err)
	}
	if search.By != by.ID || search.Status != domain.WhoRequested || !search.StartedAt.Equal(at) {
		t.Fatalf("a started who search is requested, by its Member, at its time, got %+v", search)
	}
	return search
}

func TestWhoSearchesWithinDailyLimit(t *testing.T) {
	group := newGroup(t, whoLimit(2))
	first := searchWho(t, group, alex, today)
	second := searchWho(t, group, alex, today.Add(time.Hour))
	if first.Number == second.Number {
		t.Fatalf("each who search has its own number, got %d twice", first.Number)
	}

	before := snapshot(group)
	_, err := group.SearchWho(alex.ID, "the man who sailed around the world alone", today.Add(2*time.Hour))
	refused := refusal(t, err, domain.WhoSearchesWithinDailyLimit)
	if refused.Limit != 2 || !refused.Next.Equal(today.Add(24*time.Hour)) {
		t.Fatalf("the refusal names the limit and when the oldest counted search leaves the day, got limit %d, next %s", refused.Limit, refused.Next)
	}
	wantUnchanged(t, group, before)

	searchWho(t, group, blair, today.Add(2*time.Hour))

	if _, err := group.ClaimWhoSearches(); err != nil {
		t.Fatal(err)
	}
	if _, err := group.FailWhoSearch(first.Number); err != nil {
		t.Fatal(err)
	}
	_, err = group.SearchWho(alex.ID, "the man who sailed around the world alone", today.Add(3*time.Hour))
	refusal(t, err, domain.WhoSearchesWithinDailyLimit)

	searchWho(t, group, alex, today.Add(24*time.Hour))

	off := newGroup(t, whoLimit(0))
	_, err = off.SearchWho(alex.ID, "the woman who wrote the first computer program", today)
	if refused := refusal(t, err, domain.WhoSearchesWithinDailyLimit); refused.Limit != 0 {
		t.Fatalf("a limit of 0 allows no who searches, got limit %d", refused.Limit)
	}
}

func TestPublicFigureSpelledAsAPersonIsThatPerson(t *testing.T) {
	fixed := domain.Person{ID: ada.ID, Name: ada.Name, Spellings: []domain.Spelling{spelling("Ada Lovelase"), spelling(ada.Name)}}
	cast[ada.ID] = ada
	group, err := domain.NewGroup("group-test", memberIDs, whoLimit(5), []domain.Person{fixed}, []domain.Nomination{finalized(4, ada, domain.Certified)}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	figure := func(name string) domain.PublicFigure {
		return domain.PublicFigure{Name: name, Spelling: spelling(name), Blurb: name + " did remarkable work.", Source: "https://example.org/" + string(spelling(name)), Photo: "/media/who/1/photo.jpg"}
	}
	receive := func(name string, forged domain.PersonID) domain.WhoSearch {
		t.Helper()
		search := searchWho(t, group, alex, today)
		if _, err := group.ReceivePublicFigure(search.Number, figure(name)); err == nil {
			t.Fatal("a public figure is received only by a claimed who search")
		}
		if _, err := group.ClaimWhoSearches(); err != nil {
			t.Fatal(err)
		}
		answer := figure(name)
		answer.Person = forged
		found, err := group.ReceivePublicFigure(search.Number, answer)
		if err != nil {
			t.Fatal(err)
		}
		if found.Status != domain.WhoFound || found.Figure.Name != name || found.Figure.Blurb == "" || found.Figure.Photo == "" {
			t.Fatalf("a received public figure is found with its blurb and photo, got %+v", found)
		}
		if _, err := group.ReceivePublicFigure(search.Number, answer); err == nil {
			t.Fatal("a who search receives its public figure once")
		}
		return found
	}

	if got := receive(ada.Name, grace.ID); got.Figure.Person != ada.ID {
		t.Fatalf("a public figure spelled as a Person is that Person, whatever the caller says, got %q", got.Figure.Person)
	}
	if got := receive("Ada Lovelase", ""); got.Figure.Person != ada.ID {
		t.Fatalf("an earlier spelling still names the Person, got %q", got.Figure.Person)
	}
	if got := receive(grace.Name, ada.ID); got.Figure.Person != "" {
		t.Fatalf("a public figure no spelling names is not a Person in the Group, got %q", got.Figure.Person)
	}
}
