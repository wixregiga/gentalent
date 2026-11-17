package chat_test

import (
	"strings"
	"testing"

	"gentalent/internal/nominations/delivery/chat"
	"gentalent/internal/nominations/domain"
)

func shameReplies(h *harness, from chat.Member, text string, to *chat.Quoted, mentions ...string) string {
	h.t.Helper()
	replies := h.handle(chat.Message{Sender: string(from.ID), Text: text, ReplyTo: to, Mentions: mentions})
	if len(replies) != 1 {
		h.t.Fatalf("%q: want one reply, got %+v", text, replies)
	}
	return replies[0].Text
}

func caseysMessage() *chat.Quoted {
	return &chat.Quoted{ID: "message-casey", Author: string(casey.ID), Text: "nope  *never*\never", SentAt: start}
}

func TestAReplyShameIsAPermanentSnapshot(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.ports.gifs = []string{"bell.gif"}
	h.send(alex, "!gt Juno Vale. Pitch")
	h.send(casey, "!!N")

	replies := h.handle(chat.Message{Sender: string(blair.ID), Text: "!gtshame juno vale. voted_no", ReplyTo: caseysMessage()})
	want := "🔔 SHAME 🔔  \n@Blair shames @Casey over Juno Vale (#1).  \nReason: voted\\_no  \nThey said: \"nope \\*never\\* ever\"  \nTheir vote right now: NO"
	if len(replies) != 1 || replies[0].Text != want || replies[0].Attachment != "/bank/bell.gif" {
		t.Fatalf("B103 B104 B105 B128 the shame reply, got %+v", replies)
	}
	shame := h.ports.shames[0]
	if shame.Shamed.ID != "message-casey" || shame.Shamed.Text != "nope *never* ever" || shame.Reason != "voted_no" || shame.GIF != "bell.gif" || shame.TargetVote != domain.No {
		t.Fatalf("B106 the record keeps that moment unescaped, got %+v", shame)
	}
	contains(t, shameReplies(h, blair, "!gtshame again", caseysMessage()), "You already used your shame for Juno Vale (#1).")
}

func TestShameRefusals(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		to       *chat.Quoted
		mentions []string
		want     string
	}{
		{"B90 the replied-to message could not be loaded", "!gtshame lol", &chat.Quoted{Missing: true}, nil,
			"I couldn't load the message you replied to. Reply to their message with !gtshame reason, or tag them: !gtshame @Name reason"},
		{"B92 a reply that tags someone else", "!gtshame @Alex lol", caseysMessage(), nil,
			"You replied to one person but tagged another. Reply without a tag, or tag without replying."},
		{"B96 no reply and no tag", "!gtshame lol", nil, nil, "Reply to their message with !gtshame reason, or tag them: !gtshame @Name reason"},
		{"B96 a tag naming nobody", "!gtshame @Dana lol", nil, nil, "Reply to their message with !gtshame reason"},
		{"B97 a reply to someone outside the Group", "!gtshame lol", &chat.Quoted{ID: "x", Author: "outsider"}, nil, "You can only shame someone in this group."},
		{"B98 yourself", "!gtshame @Blair lol", nil, nil, "You can't shame yourself."},
		{"B99 a near miss", "!gtshame @Casey juno vael lol", nil, nil, "Did you mean Juno Vale? Name them exactly as the bot shows them."},
		{"B110 a first name alone", "!gtshame @Casey Juno's fault", nil, nil, "Did you mean Juno Vale?"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, oneAtATime)
			h.send(alex, "!gt Juno Vale. Pitch")
			if got := shameReplies(h, blair, c.text, c.to, c.mentions...); !strings.HasPrefix(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
			if len(h.ports.shames) != 0 {
				t.Fatal("a refused shame records nothing")
			}
		})
	}
}

func TestNothingToShameOver(t *testing.T) {
	h := newHarness(t, oneAtATime)
	if got := shameReplies(h, blair, "!gtshame @Casey lol", nil); got != "There's no nominee to shame anyone over yet." {
		t.Fatalf("B101, got %q", got)
	}
}

func TestAReplyToTheBotIsSilent(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	if replies := h.handle(chat.Message{Sender: string(blair.ID), Text: "!gtshame lol", ReplyTo: &chat.Quoted{ID: "bot", FromBot: true}}); len(replies) != 0 {
		t.Fatalf("B91 nothing is said, got %+v", replies)
	}
	if len(h.ports.shames) != 0 {
		t.Fatal("B91 nothing is recorded")
	}
}

func TestTaggedShames(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		mentions []string
		target   domain.MemberID
		reason   string
	}{
		{"B94 a first name, the rest kept as typed", "!gtshame @casey: lol", nil, casey.ID, ": lol"},
		{"B94 a full name, any case", "!gtshame @ALEX lol", nil, alex.ID, "lol"},
		{"B94 a name must end at a non-alphanumeric", "!gtshame @Caseyx lol", []string{string(casey.ID)}, casey.ID, "lol"},
		{"B95 one mention when the name matches nobody", "!gtshame @Cee lol", []string{string(casey.ID)}, casey.ID, "lol"},
		{"B109 a nominee named first", "!gtshame @Casey jUnO vAlE 👀 \u2014 voted no", nil, casey.ID, "voted no"},
		{"Q17 a nominee named by number", "!gtshame @Casey 1 voted no", nil, casey.ID, "voted no"},
		{"B112 no match is all reason", "!gtshame @Casey totally unrelated", nil, casey.ID, "totally unrelated"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, oneAtATime)
			h.send(alex, "!gt Juno Vale. Pitch")
			by := blair
			if c.target == blair.ID || c.target == alex.ID {
				by = casey
			}
			h.handle(chat.Message{Sender: string(by.ID), Text: c.text, Mentions: c.mentions})
			if len(h.ports.shames) != 1 {
				t.Fatalf("want one shame, got %+v", h.ports.shames)
			}
			if got := h.ports.shames[0]; got.Target != c.target || got.Reason != c.reason || got.Nomination != 1 {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestShameReasonsAreCut(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	h.handle(chat.Message{Sender: string(blair.ID), Text: "!gtshame @Casey " + strings.Repeat("ab ", 200)})
	reason := []rune(h.ports.shames[0].Reason)
	if len(reason) != 300 || reason[299] != '…' {
		t.Fatalf("B103 a reason is cut to 300 characters with an ellipsis, got %d", len(reason))
	}
}

func TestAPhotoWithoutTextIsQuotedAsPhoto(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	got := shameReplies(h, blair, "!gtshame", &chat.Quoted{ID: "photo", Author: string(casey.ID), HasAttachment: true})
	contains(t, got, `They said: "\[photo\]"`, "Their vote right now: not cast", "(No shame GIFs in the bank yet.)")
}

func TestShamesWithSeveralOpenNameTheirNomination(t *testing.T) {
	h := newHarness(t, severalOpen(2, 2))
	h.send(alex, "!gt Juno Vale. Pitch !gt Remy Quill. Pitch")
	contains(t, shameReplies(h, blair, "!gtshame @Casey lol", nil), "Several Nominations are open", "Nomination 2: Remy Quill")
	shameReplies(h, blair, "!gtshame @Casey remy quill lol", nil)
	if len(h.ports.shames) != 1 || h.ports.shames[0].Nomination != 2 {
		t.Fatalf("Q15 the named Nomination, got %+v", h.ports.shames)
	}
}
