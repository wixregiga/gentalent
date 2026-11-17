package command

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type pollPlatform struct {
	pages    map[string]port.Page
	messages map[string]domain.Message
}

func (p pollPlatform) VerifyChat(context.Context) error { return nil }
func (p pollPlatform) Messages(_ context.Context, cursor string) (port.Page, error) {
	page, ok := p.pages[cursor]
	if !ok {
		return page, errors.New("unknown cursor")
	}
	return page, nil
}
func (p pollPlatform) Retrieve(_ context.Context, id string) (domain.Message, error) {
	message, ok := p.messages[id]
	if !ok {
		return message, port.ErrMessageMissing
	}
	return message, nil
}

type pollStore struct {
	baseline port.Baseline
	items    []port.InboxItem
}

func (p *pollStore) Baseline(context.Context) (port.Baseline, error) { return p.baseline, nil }
func (p *pollStore) Arm(_ context.Context, baseline port.Baseline) (bool, error) {
	if !p.baseline.ArmedAt.IsZero() {
		return false, nil
	}
	p.baseline = baseline
	return true, nil
}
func (p *pollStore) SavePage(_ context.Context, before, after string, items []port.InboxItem) error {
	if before != p.baseline.Cursor {
		return errors.New("stale cursor")
	}
	p.items = append(p.items, items...)
	p.baseline.Cursor = after
	return nil
}
func (p *pollStore) KnownReplyID(_ context.Context, id string) (bool, error) {
	return id == "pending-id" || id == "final-id", nil
}
func (p *pollStore) KnownAttachment(_ context.Context, name string) (bool, error) {
	return name == "strip.jpg", nil
}

func TestPollFiltersAndLoadsLinkedMessagesAcrossPages(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	scope := domain.Group{ChatID: "group", AccountID: "account", Owner: "alex", Members: []string{"alex", "blair", "casey"}}
	base := domain.Message{ChatID: scope.ChatID, AccountID: scope.AccountID, Sender: "alex", SentAt: start, Text: "!gthelp", FromAccount: true}
	makeMessage := func(id string, change func(*domain.Message)) domain.Message {
		m := base
		m.ID = id
		if change != nil {
			change(&m)
		}
		return m
	}
	messages := []domain.Message{
		makeMessage("missing-link", func(m *domain.Message) { m.SortKey = "2"; m.LinkedID = "unknown" }),
		makeMessage("empty", func(m *domain.Message) { m.Text = "<p> &nbsp; </p>" }),
		makeMessage("early", func(m *domain.Message) { m.SentAt = start.Add(-time.Second) }),
		makeMessage("invalid-time", func(m *domain.Message) { m.SentAt = time.Time{} }),
		makeMessage("other-chat", func(m *domain.Message) { m.ChatID = "other" }),
		makeMessage("other-account", func(m *domain.Message) { m.AccountID = "other" }),
		makeMessage("other-sender", func(m *domain.Message) { m.Sender = "other" }),
		makeMessage("edited", func(m *domain.Message) { m.Edited = true }),
		makeMessage("hidden", func(m *domain.Message) { m.Hidden = true }),
		makeMessage("deleted", func(m *domain.Message) { m.Deleted = true }),
		makeMessage("pending-id", nil),
		makeMessage("final-id", nil),
		makeMessage("prefix", func(m *domain.Message) { m.Sender = "blair"; m.Text = "<p>@GenTalent-Bot Says: hi</p>" }),
		makeMessage("photo", func(m *domain.Message) { m.Attachments = []domain.Attachment{{Name: "strip.jpg"}} }),
		makeMessage("linked-bot", func(m *domain.Message) {
			m.SortKey = "1"
			m.Text = "<p>hi &amp; all</p>"
			m.LinkedID = "quoted"
			m.Mentions = []string{"alex"}
		}),
		makeMessage("", nil),
	}
	platform := pollPlatform{
		pages: map[string]port.Page{
			"baseline/+": {Messages: messages, NewestCursor: "next=?", HasMore: true},
			"next=?":     {Messages: []domain.Message{makeMessage("last", func(m *domain.Message) { m.SentAt = start.Add(time.Second) })}, NewestCursor: "done"},
		},
		messages: map[string]domain.Message{"quoted": makeMessage("quoted", func(m *domain.Message) { m.Text = "<em>@gentalent-bot says:</em> quoted" })},
	}
	s := &pollStore{baseline: port.Baseline{Cursor: "baseline/+", ArmedAt: start}}
	if err := (Poller{Platform: platform, Store: s, Group: scope}).Poll(t.Context()); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, item := range s.items {
		got = append(got, item.Message.ID)
	}
	if !reflect.DeepEqual(got, []string{"linked-bot", "missing-link", "last"}) || s.baseline.Cursor != "done" {
		t.Fatalf("inbox order = %v, cursor = %q", got, s.baseline.Cursor)
	}
	first, second := s.items[0].Message, s.items[1].Message
	if first.Text != " hi & all " || first.Linked == nil || !first.Linked.FromBot || !reflect.DeepEqual(first.Mentions, []string{"alex"}) ||
		second.Linked == nil || !second.Linked.Missing || second.Linked.ID != "unknown" {
		t.Fatalf("linked/normalized messages = %+v %+v", first, second)
	}
}

func TestArmAndCursorGuards(t *testing.T) {
	start := time.Now().UTC()
	store := &pollStore{}
	platform := pollPlatform{pages: map[string]port.Page{"": {NewestCursor: "latest"}}}
	poll := Poller{Platform: platform, Store: store}
	if err := poll.Poll(t.Context()); err == nil {
		t.Fatal("unarmed poll passed")
	}
	arm := Armer{Platform: platform, Store: store, Now: func() time.Time { return start }}
	if changed, err := arm.Arm(t.Context(), false); err != nil || !changed || store.baseline.Cursor != "latest" {
		t.Fatalf("arming = %v, %v, %+v", changed, err, store.baseline)
	}
	if changed, err := arm.Arm(t.Context(), true); err != nil || changed {
		t.Fatalf("if unarmed = %v, %v", changed, err)
	}
	if _, err := arm.Arm(t.Context(), false); err == nil {
		t.Fatal("rearmed")
	}
	for _, tc := range []struct {
		name, cursor, saved string
		pages               map[string]port.Page
	}{
		{"no baseline", "", "", map[string]port.Page{"": {HasMore: true, NewestCursor: "next"}}},
		{"non advancing", "latest", "latest", map[string]port.Page{"latest": {HasMore: true, NewestCursor: "latest"}}},
		{"cycle", "latest", "next", map[string]port.Page{"latest": {HasMore: true, NewestCursor: "next"}, "next": {HasMore: true, NewestCursor: "latest"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store.baseline.Cursor = tc.cursor
			poll.Platform = pollPlatform{pages: tc.pages}
			if err := poll.Poll(t.Context()); err == nil || store.baseline.Cursor != tc.saved {
				t.Fatalf("poll = %v, cursor = %s", err, store.baseline.Cursor)
			}
		})
	}
}

func TestPollCallerCanSelectWhichLinkedMessagesNeedLoading(t *testing.T) {
	at := time.Now().UTC()
	group := domain.Group{ChatID: "group", AccountID: "account", Members: []string{"alex"}}
	base := domain.Message{ID: "one", ChatID: "group", AccountID: "account", Sender: "alex", SentAt: at, LinkedID: "missing", Text: "ordinary reply"}
	needsLink := base
	needsLink.ID = "two"
	needsLink.Text = "<p>needs &amp; uses quote</p>"
	store := &pollStore{baseline: port.Baseline{Cursor: "start", ArmedAt: at}}
	p := Poller{
		Platform: pollPlatform{pages: map[string]port.Page{"start": {Messages: []domain.Message{base, needsLink}}}},
		Store:    store, Group: group, NeedsLinked: func(text string) bool { return text == " needs & uses quote " },
	}
	if err := p.Poll(t.Context()); err != nil || len(store.items) != 2 {
		t.Fatalf("poll = %+v, %v", store.items, err)
	}
	if store.items[0].Message.Linked != nil || store.items[1].Message.Linked == nil || !store.items[1].Message.Linked.Missing {
		t.Fatalf("linked messages = %+v, %+v", store.items[0].Message.Linked, store.items[1].Message.Linked)
	}
}
