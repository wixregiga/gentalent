package domain

import (
	"slices"
	"time"
)

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

func (g Group) Accepts(message Message, armedAt time.Time) bool {
	return message.ID != "" && message.ChatID == g.ChatID && message.AccountID == g.AccountID &&
		g.HasMember(message.Sender) && !message.Edited && !message.Hidden && !message.Deleted &&
		!message.SentAt.IsZero() && !message.SentAt.Before(armedAt)
}

func (g Group) HasMember(sender string) bool {
	return slices.Contains(g.Members, sender)
}
