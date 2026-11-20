package chat

import "time"

type Message struct {
	ID       string
	Sender   string
	Text     string
	SentAt   time.Time
	ReplyTo  *Quoted
	Mentions []string
}

type Quoted struct {
	ID            string
	Author        string
	Text          string
	SentAt        time.Time
	FromBot       bool
	Missing       bool
	HasAttachment bool
	Announces     int
}

type Reply struct {
	Key        string
	Text       string
	Attachment string
	Announces  int
}

type PhotoSearch struct {
	Nomination int
	Person     string
	Pitch      string
	Dir        string
}

type WhoSearch struct {
	Number      int
	Description string
	Dir         string
}

type WhoResult struct {
	Number   int
	Failure  string
	NoMatch  bool
	Name     string
	Blurb    string
	Source   string
	Photo    string
	Provider string
}

type PhotoResult struct {
	Nomination int
	Photos     []string
	Strip      string
	Failure    string
	Provider   string
	Summary    string
	Sources    string
}
