package chat_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gentalent/internal/nominations/delivery/chat"
	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

var (
	alex   = chat.Member{ID: "member-alex", Name: "Alex"}
	blair  = chat.Member{ID: "member-blair", Name: "Blair"}
	casey  = chat.Member{ID: "member-casey", Name: "Casey"}
	roster = []chat.Member{alex, blair, casey}
	start  = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
)

var oneAtATime = domain.Settings{GroupOpenNominationLimit: 1, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3}

type harness struct {
	t       *testing.T
	ports   *fakePorts
	bot     *chat.Bot
	now     time.Time
	sent    int
	relayed int
}

func newHarness(t *testing.T, settings domain.Settings, change ...func(*chat.Config)) *harness {
	t.Helper()
	h := &harness{t: t, now: start}
	ids := make([]domain.MemberID, len(roster))
	for i, member := range roster {
		ids[i] = member.ID
	}
	h.ports = newFakePorts(t, ids, settings)
	config := chat.Config{Members: roster, Settings: settings, PhotoLayout: chat.Individual, MediaDir: "/media", Now: func() time.Time { return h.now }}
	for _, apply := range change {
		apply(&config)
	}
	p := h.ports
	h.bot = chat.NewBot(config, chat.Ports{Groups: p, Outbox: p, Feedback: p, Gifs: p, Standing: p, Names: p, Unrelayed: p, Entries: p})
	return h
}

func (h *harness) handle(msg chat.Message) []port.Reply {
	h.t.Helper()
	if msg.ID == "" {
		h.sent++
		msg.ID = fmt.Sprintf("message-%d", h.sent)
	}
	if msg.SentAt.IsZero() {
		msg.SentAt = h.now
	}
	if err := h.bot.Handle(context.Background(), msg); err != nil {
		h.t.Fatalf("Handle %q: %v", msg.Text, err)
	}
	return h.fresh()
}

func (h *harness) send(from chat.Member, text string) []port.Reply {
	h.t.Helper()
	return h.handle(chat.Message{Sender: string(from.ID), Text: text})
}

func (h *harness) say(from chat.Member, text string) string {
	h.t.Helper()
	replies := h.send(from, text)
	if len(replies) != 1 {
		h.t.Fatalf("%s %q: want one reply, got %+v", from.Name, text, replies)
	}
	return replies[0].Text
}

func (h *harness) silent(from chat.Member, text string) {
	h.t.Helper()
	if replies := h.send(from, text); len(replies) != 0 {
		h.t.Fatalf("%s %q must get no reply, got %+v", from.Name, text, replies)
	}
}

func (h *harness) fresh() []port.Reply {
	replies := h.ports.outbox[h.relayed:]
	h.relayed = len(h.ports.outbox)
	return replies
}

func (h *harness) nomination(number domain.NominationNumber) domain.Nomination {
	h.t.Helper()
	nomination, ok := h.ports.group().Nomination(number)
	if !ok {
		h.t.Fatalf("Nomination %d does not exist", number)
	}
	return nomination
}

func (h *harness) open() []domain.Nomination {
	return h.ports.group().Open()
}

func contains(t *testing.T, text string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(text, part) {
			t.Fatalf("want %q in reply:\n%s", part, text)
		}
	}
}
