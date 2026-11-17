package domain

import "time"

type Incoming struct {
	ID       string
	Sender   string
	Text     string
	SentAt   time.Time
	Linked   *Linked
	Mentions []string
}

type Linked struct {
	ID      string
	Sender  string
	Text    string
	SentAt  time.Time
	FromBot bool
	Missing bool
}

type Message struct {
	ID, ChatID, AccountID, Sender, Text, SortKey, LinkedID string
	SentAt                                                 time.Time
	Edited, Deleted, Hidden, FromAccount                   bool
	Status                                                 SendStatus
	Mentions                                               []string
	Attachments                                            []Attachment
	Reactions                                              []Reaction
}

type Attachment struct {
	Name      string
	Bytes     int64
	SizeKnown bool
}

type Reaction struct {
	Sender, Key string
}

type Group struct {
	ChatID, AccountID, Owner string
	Members                  []string
}
