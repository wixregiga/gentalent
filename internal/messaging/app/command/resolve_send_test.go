package command

import (
	"context"
	"errors"
	"testing"

	"gentalent/internal/messaging/domain"
)

type resolutionStore struct {
	reply      domain.Reply
	dup, stale bool
}

func (r *resolutionStore) Reply(context.Context, string) (domain.Reply, error) { return r.reply, nil }
func (r *resolutionStore) FinalIDUsed(context.Context, string, string) (bool, error) {
	return r.dup, nil
}
func (r *resolutionStore) Transition(_ context.Context, key string, from, to domain.State, id string) (domain.State, error) {
	if r.stale || r.reply.Key != key || r.reply.State != from {
		return "", nil
	}
	r.reply.State = to
	if id != "" {
		r.reply.FinalID = id
	}
	return to, nil
}

type resolutionFile struct {
	file domain.Attachment
	err  error
}

func (f resolutionFile) Attachment(context.Context, string) (domain.Attachment, error) {
	return f.file, f.err
}

func TestResolveSendNeedsEvidenceAndCannotResend(t *testing.T) {
	const photo = "media/strip.jpg"
	attached := domain.Attachment{Name: "strip.jpg", Bytes: 12, SizeKnown: true}
	for _, tc := range []struct {
		name             string
		state            domain.State
		mid, path        string
		message          domain.Message
		local            resolutionFile
		duplicate, stale bool
		want             domain.State
	}{
		{name: "record not delivered", state: domain.Uncertain, want: domain.Failed},
		{name: "queued not resolvable", state: domain.Queued},
		{name: "no row"},
		{name: "matching delivered text", state: domain.Uncertain, mid: "final", message: domain.Message{ID: "final", FromAccount: true, Text: "<p>They &amp; us<br><em>yes</em></p>"}, want: domain.Sent},
		{name: "account must own message", state: domain.Uncertain, mid: "final", message: domain.Message{ID: "final"}},
		{name: "text must match", state: domain.Uncertain, mid: "final", message: domain.Message{ID: "final", FromAccount: true, Text: "wrong"}},
		{name: "duplicate ID", state: domain.Uncertain, mid: "final", duplicate: true},
		{name: "racing update", state: domain.Uncertain, mid: "final", stale: true, message: domain.Message{ID: "final", FromAccount: true, Text: "They & us yes"}},
		{name: "matching photo without caption", state: domain.Failed, mid: "final", path: photo, message: domain.Message{ID: "final", FromAccount: true, Attachments: []domain.Attachment{attached}}, local: resolutionFile{file: attached}, want: domain.Sent},
		{name: "missing photo", state: domain.Failed, mid: "final", path: photo, message: domain.Message{ID: "final", FromAccount: true, Attachments: []domain.Attachment{attached}}, local: resolutionFile{err: errors.New("missing")}},
		{name: "wrong photo size", state: domain.Failed, mid: "final", path: photo, message: domain.Message{ID: "final", FromAccount: true, Attachments: []domain.Attachment{attached}}, local: resolutionFile{file: domain.Attachment{Name: "strip.jpg", Bytes: 13, SizeKnown: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &resolutionStore{reply: domain.Reply{Outgoing: domain.Outgoing{Key: "reply", Text: "They & us  \n*yes*", Attachment: tc.path}, State: tc.state}, dup: tc.duplicate, stale: tc.stale}
			if tc.state == domain.Failed {
				s.reply.FinalID = tc.mid
			}
			r := Resolver{Store: s, Platform: pollPlatform{messages: map[string]domain.Message{tc.mid: tc.message}}, Files: tc.local}
			err := r.ResolveSend(t.Context(), "reply", tc.mid)
			if tc.want == "" {
				if err == nil || s.reply.State != tc.state {
					t.Fatalf("invalid resolution = %+v, %v", s.reply, err)
				}
				return
			}
			if err != nil || s.reply.State != tc.want || s.reply.FinalID != tc.mid || s.reply.State.Allows(domain.Queued) {
				t.Fatalf("resolution = %+v, %v", s.reply, err)
			}
		})
	}
}
