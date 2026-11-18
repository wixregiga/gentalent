package command

import (
	"context"
	"errors"
	"slices"
	"testing"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type flushStore struct {
	rows []domain.Reply
}

func (s *flushStore) ActiveReplies(context.Context) ([]domain.Reply, error) {
	var active []domain.Reply
	for _, reply := range s.rows {
		if reply.State == domain.Queued || reply.State == domain.Pending {
			active = append(active, reply)
		}
	}
	return active, nil
}
func (s *flushStore) Transition(_ context.Context, key string, from, to domain.State, id string) (domain.State, error) {
	for i := range s.rows {
		reply := &s.rows[i]
		if reply.Key == key && reply.State == from && from.Allows(to) {
			reply.State = to
			switch to {
			case domain.Pending:
				reply.PendingID = id
			case domain.Sent, domain.Failed:
				reply.FinalID = id
			}
			return to, nil
		}
	}
	return "", nil
}

type flushPlatform struct {
	store                *flushStore
	posts, lookups       []string
	retrieval            map[string]domain.Message
	missing              map[string]bool
	sendError, readError error
}

func (p *flushPlatform) Send(_ context.Context, reply domain.Outgoing) (port.Receipt, error) {
	if !slices.ContainsFunc(p.store.rows, func(row domain.Reply) bool {
		return row.Key == reply.Key && row.State == domain.Sending
	}) {
		return port.Receipt{}, errors.New("reply not durably claimed before POST")
	}
	p.posts = append(p.posts, reply.Key)
	return port.Receipt{PendingID: "pending-" + reply.Key}, p.sendError
}

func (p *flushPlatform) Retrieve(_ context.Context, id string) (domain.Message, error) {
	p.lookups = append(p.lookups, id)
	if p.missing[id] {
		return domain.Message{}, port.ErrMessageMissing
	}
	return p.retrieval[id], p.readError
}

func TestFlushStopsAtAnAmbiguousWriteAndNeverRetriesIt(t *testing.T) {
	store := &flushStore{rows: []domain.Reply{
		{Outgoing: domain.Outgoing{Key: "a"}, State: domain.Queued},
		{Outgoing: domain.Outgoing{Key: "b"}, State: domain.Queued},
	}}
	platform := &flushPlatform{store: store, sendError: errors.New("timeout after POST")}
	flush := Flusher{Store: store, Sender: platform, Reader: platform}
	ctx := t.Context()
	if err := flush.Flush(ctx); err == nil || store.rows[0].State != domain.Uncertain || store.rows[1].State != domain.Queued ||
		!slices.Equal(platform.posts, []string{"a"}) {
		t.Fatalf("ambiguous flush = %v, %v, %v", store.rows, platform.posts, err)
	}
	platform.sendError = nil
	if err := flush.Flush(ctx); err != nil || store.rows[1].State != domain.Pending || len(platform.lookups) != 0 ||
		!slices.Equal(platform.posts, []string{"a", "b"}) {
		t.Fatalf("following flush = %v, sends %v, reads %v, %v", store.rows, platform.posts, platform.lookups, err)
	}
	platform.retrieval = map[string]domain.Message{"pending-b": {ID: "final-b", Status: domain.AwaitingConfirmation}}
	if err := flush.Flush(ctx); err != nil || store.rows[1].State != domain.Pending {
		t.Fatalf("final ID without delivery evidence = %v, %v", store.rows, err)
	}
	platform.retrieval["pending-b"] = domain.Message{ID: "final-b"}
	if err := flush.Flush(ctx); err != nil || store.rows[1].State != domain.Sent || store.rows[1].FinalID != "final-b" || len(platform.posts) != 2 {
		t.Fatalf("resolved flush = %v, %v, %v", store.rows, platform.posts, err)
	}
}

func TestExpiredPendingIDDoesNotBlockAnotherReply(t *testing.T) {
	store := &flushStore{rows: []domain.Reply{
		{Outgoing: domain.Outgoing{Key: "a"}, State: domain.Pending, PendingID: "pending-a"},
		{Outgoing: domain.Outgoing{Key: "b"}, State: domain.Queued},
	}}
	platform := &flushPlatform{store: store, readError: errors.New("temporary failure")}
	flush := Flusher{Store: store, Sender: platform, Reader: platform}
	ctx := t.Context()
	if err := flush.Flush(ctx); err == nil || store.rows[0].State != domain.Pending || len(platform.posts) != 0 {
		t.Fatalf("temporary lookup error = %v, %v, %v", store.rows, platform.posts, err)
	}
	platform.readError = nil
	platform.missing = map[string]bool{"pending-a": true}
	if err := flush.Flush(ctx); err != nil || store.rows[0].State != domain.Uncertain || store.rows[1].State != domain.Pending ||
		!slices.Equal(platform.posts, []string{"b"}) {
		t.Fatalf("expired pending lookup = %v, %v, %v", store.rows, platform.posts, err)
	}
}

type queuedReplies struct{ replies []domain.Outgoing }

func (q *queuedReplies) Enqueue(_ context.Context, replies []domain.Outgoing) error {
	q.replies = replies
	return nil
}

func TestEnqueuePrefixesTextAndLeavesPhotosTextless(t *testing.T) {
	store := &queuedReplies{}
	replies := []Outgoing{{Key: "text", Text: "literal \\*markdown\\*", Tag: "tag"}, {Key: "photo", Attachment: "photo.jpg"}}
	if err := (Enqueuer{Store: store}).Enqueue(t.Context(), replies); err != nil {
		t.Fatal(err)
	}
	if store.replies[0].Text != domain.ReplyPrefix+" "+replies[0].Text || store.replies[0].Tag != "tag" ||
		store.replies[1] != replies[1] {
		t.Fatalf("queued replies = %+v", store.replies)
	}
}
