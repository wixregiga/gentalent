package domain

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

type PersonID string

type Spelling string

type Person struct {
	ID   PersonID
	Name string
}

func newPersonID(at time.Time) PersonID {
	var id [16]byte
	var stamp [8]byte
	binary.BigEndian.PutUint64(stamp[:], uint64(at.UnixMilli()))
	copy(id[:6], stamp[2:])
	rand.Read(id[6:])
	id[6] = 0x70 | id[6]&0x0f
	id[8] = 0x80 | id[8]&0x3f
	return PersonID(fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16]))
}
