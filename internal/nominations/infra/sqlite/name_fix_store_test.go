package sqlite_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/infra/sqlite"
)

func misspelledAda() []domain.Candidate {
	return []domain.Candidate{{Spelling: "adalovelase", NewPerson: "person-ada", Name: "Ada Lovelase"}}
}

func toLovelace(withPhotos bool) domain.NameFixProposal {
	return domain.NameFixProposal{Nomination: 1, Name: "Ada Lovelace", Spelling: "adalovelace", WithPhotos: withPhotos}
}

func applyFix(t *testing.T, store *sqlite.Store, messageID string, change func(*domain.Group) (domain.NameFix, error)) domain.NameFix {
	t.Helper()
	var fix domain.NameFix
	apply(t, store, messageID, func(g *domain.Group) error {
		var err error
		fix, err = change(g)
		return err
	})
	return fix
}

func reloadedFix(t *testing.T, store *sqlite.Store, number domain.NameFixNumber) domain.NameFix {
	t.Helper()
	fix, ok := loaded(t, store).NameFix(number)
	if !ok {
		t.Fatalf("name fix %d is not kept", number)
	}
	return fix
}

func TestANameFixIsKeptFromProposalToFixedName(t *testing.T) {
	db := migrated(t)
	store := newStore(t, db)
	apply(t, store, "m1", func(g *domain.Group) error { _, err := g.Nominate(alex, misspelledAda(), today); return err })

	proposed := applyFix(t, store, "m2", func(g *domain.Group) (domain.NameFix, error) {
		return g.ProposeNameFix(alex, toLovelace(true), today.Add(time.Minute))
	})
	if got := reloadedFix(t, store, proposed.Number); !reflect.DeepEqual(got, proposed) {
		t.Fatalf("a proposed name fix reloads as proposed:\ngot  %+v\nwant %+v", got, proposed)
	}
	agreed := applyFix(t, store, "m3", func(g *domain.Group) (domain.NameFix, error) {
		return g.AgreeToNameFix(blair, proposed.Number, today.Add(2*time.Minute))
	})
	if got := reloadedFix(t, store, proposed.Number); !reflect.DeepEqual(got, agreed) || got.Count(domain.Yes) != 2 || !got.Pending() {
		t.Fatalf("an answer to a pending name fix is kept:\ngot  %+v\nwant %+v", got, agreed)
	}
	fixed := applyFix(t, store, "m4", func(g *domain.Group) (domain.NameFix, error) {
		return g.AgreeToNameFix(casey, proposed.Number, today.Add(3*time.Minute))
	})
	if fixed.Status != domain.NameFixed {
		t.Fatalf("the third yes fixes the name, got %+v", fixed)
	}

	group := loaded(t, store)
	if got, _ := group.NameFix(proposed.Number); !reflect.DeepEqual(got, fixed) {
		t.Fatalf("a fixed name fix reloads as fixed:\ngot  %+v\nwant %+v", got, fixed)
	}
	nomination, _ := group.Nomination(1)
	if nomination.Person != (domain.Person{ID: "person-ada", Name: "Ada Lovelace"}) || nomination.Photos != domain.PhotosRequested {
		t.Fatalf("the fixed name and the photo search it asked for are kept, got %+v", nomination)
	}
	want := map[domain.Spelling]domain.PersonID{"adalovelase": "person-ada", "adalovelace": "person-ada"}
	if got := group.Spellings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("the new spelling and the earlier spelling both name the Person, got %v", got)
	}
	names, err := sqlite.NewReader(db).Names(context.Background())
	if err != nil || len(names) != 2 {
		t.Fatalf("both spellings are readable names, got %+v, %v", names, err)
	}
	for _, name := range names {
		if name.Person != "person-ada" || name.Display != "Ada Lovelace" || !name.Open {
			t.Fatalf("every spelling shows the fixed name, got %+v", names)
		}
	}
	standing, err := sqlite.NewReader(db).Standing(context.Background())
	if err != nil || len(standing.Open) != 1 || standing.Open[0].Person != "Ada Lovelace" {
		t.Fatalf("status shows the fixed name, got %+v, %v", standing, err)
	}
}

func TestEndedNameFixesAreKeptWithoutChangingTheName(t *testing.T) {
	store := newStore(t, migrated(t))
	apply(t, store, "m1", func(g *domain.Group) error { _, err := g.Nominate(alex, misspelledAda(), today); return err })
	propose := func(messageID string, by domain.MemberID, at time.Time) domain.NameFix {
		return applyFix(t, store, messageID, func(g *domain.Group) (domain.NameFix, error) {
			return g.ProposeNameFix(by, toLovelace(false), at)
		})
	}

	first := propose("m2", alex, today)
	stopped := applyFix(t, store, "m3", func(g *domain.Group) (domain.NameFix, error) {
		return g.StopNameFix(blair, first.Number, today.Add(time.Minute))
	})
	second := propose("m4", blair, today.Add(2*time.Minute))
	withdrawn := applyFix(t, store, "m5", func(g *domain.Group) (domain.NameFix, error) {
		return g.StopNameFix(blair, second.Number, today.Add(3*time.Minute))
	})
	third := propose("m6", casey, today.Add(4*time.Minute))
	var lapsed []domain.NameFix
	apply(t, store, "", func(g *domain.Group) error {
		var err error
		lapsed, err = g.ExpireNameFixes(today.Add(4*time.Minute + 24*time.Hour))
		return err
	})

	for _, want := range []struct {
		fix    domain.NameFix
		status domain.NameFixStatus
	}{{stopped, domain.NameFixStopped}, {withdrawn, domain.NameFixWithdrawn}} {
		got := reloadedFix(t, store, want.fix.Number)
		if got.Status != want.status || !reflect.DeepEqual(got, want.fix) {
			t.Fatalf("a %s name fix reloads as it ended:\ngot  %+v\nwant %+v", want.status, got, want.fix)
		}
	}
	if len(lapsed) != 1 || lapsed[0].Number != third.Number {
		t.Fatalf("the third name fix lapses, got %+v", lapsed)
	}
	if got := reloadedFix(t, store, third.Number); got.Status != domain.NameFixLapsed || !reflect.DeepEqual(got, lapsed[0]) {
		t.Fatalf("a lapsed name fix reloads as lapsed:\ngot  %+v\nwant %+v", got, lapsed[0])
	}
	group := loaded(t, store)
	if nomination, _ := group.Nomination(1); nomination.Person.Name != "Ada Lovelase" {
		t.Fatalf("an ended name fix leaves the name, got %+v", nomination.Person)
	}
	if spellings := group.Spellings(); len(spellings) != 1 || spellings["adalovelase"] != "person-ada" {
		t.Fatalf("an ended name fix adds no spelling, got %v", spellings)
	}
}

func TestEndedNameFixesArePermanent(t *testing.T) {
	db := migrated(t)
	store := newStore(t, db)
	apply(t, store, "m1", func(g *domain.Group) error { _, err := g.Nominate(alex, misspelledAda(), today); return err })
	applyFix(t, store, "m2", func(g *domain.Group) (domain.NameFix, error) { return g.ProposeNameFix(alex, toLovelace(false), today) })
	applyFix(t, store, "m3", func(g *domain.Group) (domain.NameFix, error) { return g.StopNameFix(blair, 1, today) })
	applyFix(t, store, "m4", func(g *domain.Group) (domain.NameFix, error) { return g.ProposeNameFix(alex, toLovelace(false), today) })

	if _, err := db.Exec("INSERT INTO name_fix_answers(number, member, answer) VALUES(2, 'member-casey', 'no')"); err != nil {
		t.Fatalf("a pending name fix still takes answers: %v", err)
	}
	for _, statement := range []string{
		"UPDATE name_fixes SET status = 'pending', ended_at = '' WHERE number = 1",
		"UPDATE name_fixes SET to_name = 'Ada King' WHERE number = 1",
		"DELETE FROM name_fixes WHERE number = 1",
		"UPDATE name_fix_answers SET answer = 'yes' WHERE number = 1",
		"UPDATE name_fix_answers SET number = 1 WHERE number = 2 AND member = 'member-casey'",
		"DELETE FROM name_fix_answers WHERE number = 1",
		"INSERT INTO name_fix_answers(number, member, answer) VALUES(1, 'member-casey', 'yes')",
	} {
		_, err := db.Exec(statement)
		if err == nil || !strings.Contains(err.Error(), "permanent") {
			t.Fatalf("%q must be refused as permanent, got %v", statement, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO name_fixes(number, nomination, person_id, from_name, to_name, spelling, proposer, proposed_at, status)
		VALUES(3, 1, 'person-ada', 'Ada Lovelase', 'Ada Lovelace', 'adalovelace', 'member-casey', '', 'pending')`); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("the database refuses a second pending name fix for one Person, got %v", err)
	}
	if _, err := db.Exec("UPDATE name_fixes SET status = 'fixed' WHERE number = 2"); err != nil {
		t.Fatalf("a pending name fix may end: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO name_fixes(number, nomination, person_id, from_name, to_name, spelling, proposer, proposed_at, status)
		VALUES(3, 1, 'person-ada', 'Ada Lovelace', 'Ada King', 'adaking', 'member-casey', '', 'fixed')`); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("the database refuses a second fixed name fix for one Person, got %v", err)
	}
}
