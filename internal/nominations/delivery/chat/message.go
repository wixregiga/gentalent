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

type PhotoResult struct {
	Nomination int
	Photos     []string
	Strip      string
	Failure    string
	Provider   string
	Sources    string
}
