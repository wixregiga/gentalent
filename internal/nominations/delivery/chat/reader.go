package chat

import (
	"context"

	"gentalent/internal/nominations/app/query"
	"gentalent/internal/nominations/port"
)

type ReadPorts struct {
	Standing    port.StandingReader
	Names       port.NameReader
	Unrelayed   port.OutboxReader
	Entries     port.FeedbackReader
	Nominations port.NominationReader
}

type Reader struct {
	config      Config
	standing    query.StandingHandler
	names       query.NamesHandler
	unrelayed   query.UnrelayedHandler
	entries     query.FeedbackHandler
	nominations query.NominationsHandler
}

type NominationRecord struct {
	Number int
	Person string
	Status string
	Votes  int
}

func NewReader(config Config, ports ReadPorts) *Reader {
	return &Reader{
		config:      config,
		standing:    query.StandingHandler{Reader: ports.Standing},
		names:       query.NamesHandler{Reader: ports.Names},
		unrelayed:   query.UnrelayedHandler{Reader: ports.Unrelayed},
		entries:     query.FeedbackHandler{Reader: ports.Entries},
		nominations: query.NominationsHandler{Reader: ports.Nominations},
	}
}

func (r *Reader) Unrelayed(ctx context.Context) ([]Reply, error) {
	stored, err := r.unrelayed.Handle(ctx)
	if err != nil {
		return nil, err
	}
	replies := make([]Reply, len(stored))
	for i, reply := range stored {
		replies[i] = Reply{Key: reply.Key, Text: reply.Text, Attachment: reply.Attachment, Announces: int(reply.Announces)}
	}
	return replies, nil
}

func (r *Reader) Feedback(ctx context.Context) ([]port.Feedback, error) {
	return r.entries.Handle(ctx)
}

func (r *Reader) StatusReport(ctx context.Context) (string, error) {
	standing, err := r.standing.Handle(ctx)
	if err != nil {
		return "", err
	}
	return r.statusText(standing), nil
}

func (r *Reader) SearchesRunning(ctx context.Context) ([]int, error) {
	standing, err := r.standing.Handle(ctx)
	if err != nil {
		return nil, err
	}
	running := make([]int, len(standing.Searching))
	for i, number := range standing.Searching {
		running[i] = int(number)
	}
	return running, nil
}

func (r *Reader) member(id string) (Member, bool) {
	for _, member := range r.config.Members {
		if string(member.ID) == id {
			return member, true
		}
	}
	return Member{}, false
}

func (r *Reader) Nominations(ctx context.Context) ([]NominationRecord, error) {
	views, err := r.nominations.Handle(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]NominationRecord, len(views))
	for i, view := range views {
		records[i] = NominationRecord{Number: int(view.Number), Person: view.Person, Status: string(view.Status), Votes: len(view.Votes)}
	}
	return records, nil
}
