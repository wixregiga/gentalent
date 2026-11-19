package domain_test

import (
	"testing"

	"github.com/google/uuid"

	"gentalent/internal/nominations/domain"
)

func TestTheGroupGivesEachNewPersonTheirOwnUUIDv7(t *testing.T) {
	group := newGroup(t, fixing(100))
	opened, err := group.Nominate(alex.ID, candidates(ada, grace), today)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[domain.PersonID]bool{}
	for _, outcome := range opened {
		id := nomination(t, group, outcome.Number).Person.ID
		parsed, err := uuid.Parse(string(id))
		if err != nil || parsed.Version() != 7 || parsed.Variant() != uuid.RFC4122 || parsed.String() != string(id) {
			t.Fatalf("Person ID %q is not a canonical UUIDv7: %v", id, err)
		}
		if seconds, nanos := parsed.Time().UnixTime(); seconds*1000+nanos/1e6 != today.UnixMilli() {
			t.Fatalf("Person ID %q carries %d ms, want the nomination's %d", id, seconds*1000+nanos/1e6, today.UnixMilli())
		}
		if seen[id] {
			t.Fatalf("two new Persons share the ID %q", id)
		}
		seen[id] = true
	}
}
