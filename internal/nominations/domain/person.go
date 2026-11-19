package domain

import (
	"crypto/rand"
	"encoding/binary"
	"slices"
	"time"
	"uuid"
)

type PersonID string

type Spelling string

type Person struct {
	ID        PersonID
	Name      string
	Spellings []Spelling
}

func (p Person) SpelledAs(spelling Spelling) bool {
	return slices.Contains(p.Spellings, spelling)
}

func (p Person) clone() Person {
	p.Spellings = slices.Clone(p.Spellings)
	return p
}

func newPersonID(at time.Time) PersonID {
	var id uuid.UUID
	var stamp [8]byte
	binary.BigEndian.PutUint64(stamp[:], uint64(at.UnixMilli()))
	copy(id[:6], stamp[2:])
	rand.Read(id[6:])
	id[6] = 0x70 | id[6]&0x0f
	id[8] = 0x80 | id[8]&0x3f
	return PersonID(id.String())
}
