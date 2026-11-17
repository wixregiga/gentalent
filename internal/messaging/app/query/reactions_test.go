package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gentalent/internal/messaging/domain"
)

type taggedReply struct{ id string }

func (r taggedReply) TaggedMessage(context.Context, string) (string, error) { return r.id, nil }

type reactionMessage struct {
	message domain.Message
	err     error
}

func (m reactionMessage) Retrieve(context.Context, string) (domain.Message, error) {
	return m.message, m.err
}

func TestReactionsKeepEveryMemberKeyAndDistinguishUnavailable(t *testing.T) {
	reader := ReactionReader{
		Replies: taggedReply{"delivered"},
		Platform: reactionMessage{message: domain.Message{ID: "delivered", Reactions: []domain.Reaction{
			{Sender: "alex", Key: "👍"},
			{Sender: "alex", Key: "👎"},
			{Sender: "blair", Key: "😂"},
			{Sender: "stranger", Key: "👍"},
		}}},
		Group: domain.Group{Members: []string{"alex", "blair", "casey"}},
	}
	got, err := reader.Reactions(t.Context(), "tag")
	if err != nil || !reflect.DeepEqual(got, map[string][]string{"alex": {"👍", "👎"}, "blair": {"😂"}}) {
		t.Fatalf("reactions = %v, %v", got, err)
	}
	reader.Platform = reactionMessage{message: domain.Message{ID: "delivered"}}
	if got, err = reader.Reactions(t.Context(), "tag"); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("delivered reply with removed reactions = %v, %v", got, err)
	}
	reader.Platform = reactionMessage{err: errors.New("unreachable")}
	if got, err = reader.Reactions(t.Context(), "tag"); err == nil || got != nil {
		t.Fatalf("unavailable platform looked like removals: %v, %v", got, err)
	}
	reader.Replies = taggedReply{}
	if got, err = reader.Reactions(t.Context(), "tag"); err != nil || got != nil {
		t.Fatalf("undelivered reply looked like removals: %v, %v", got, err)
	}
}
