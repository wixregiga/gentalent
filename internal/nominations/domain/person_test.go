package domain_test

import (
	"encoding/binary"
	"testing"
	"uuid"

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
		id := nomination(t, group, outcome.Number).Person
		parsed, err := uuid.Parse(string(id))
		if err != nil || parsed[6]>>4 != 7 || parsed[8]>>6 != 0b10 || parsed.String() != string(id) {
			t.Fatalf("Person ID %q is not a canonical UUIDv7 of the RFC 9562 variant: %v", id, err)
		}
		if ms := binary.BigEndian.Uint64(append([]byte{0, 0}, parsed[:6]...)); int64(ms) != today.UnixMilli() {
			t.Fatalf("Person ID %q carries %d ms, want the nomination's %d", id, ms, today.UnixMilli())
		}
		if seen[id] {
			t.Fatalf("two new Persons share the ID %q", id)
		}
		seen[id] = true
	}
}
