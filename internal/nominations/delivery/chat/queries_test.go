package chat_test

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"gentalent/internal/nominations/delivery/chat"
	"gentalent/internal/nominations/domain"
)

func TestFeedback(t *testing.T) {
	h := newHarness(t, oneAtATime)
	cases := []struct{ text, want string }{
		{"!gtfeedback", "Add your feedback after the command, e.g. !gtfeedback show who voted last."},
		{"!gtsuggest   ", "Add your suggestion after the command, e.g. !gtsuggest show who voted last."},
		{"!gtfeedback " + strings.Repeat("x", 2001), "Keep it under 2000 characters."},
		{"!gtsuggest show\nwho voted last", "Thanks @Alex, your suggestion is logged and will be considered for a future release."},
	}
	for _, c := range cases {
		if got := h.say(alex, c.text); got != c.want {
			t.Fatalf("B114 B115 B116 %q: got %q", c.text, got)
		}
	}
	entries, err := h.bot.Feedback(context.Background())
	if err != nil || len(entries) != 1 || entries[0].Kind != "suggestion" || entries[0].Text != "show\nwho voted last" || entries[0].SaidAt != start {
		t.Fatalf("B113 B117 only the valid entry is kept and never echoed, got %+v, %v", entries, err)
	}
}

func TestHelp(t *testing.T) {
	h := newHarness(t, oneAtATime)
	contains(t, h.say(alex, "!gthelp"), "3/3 YES certifies. Any NO denies forever.", "Name a Nomination by number or name: !!Y 12",
		"Nominate: !gt Name. Pitch", "Vote: !!Y yes · !!N no · !!C clear", "Ideas for the bot: !gtfeedback your idea")
	renominating := newHarness(t, domainSettingsWithRenomination())
	contains(t, renominating.say(alex, "!gthelp"), "Any NO rejects.", "Renominate the rejected: !gtrenominate Name. Pitch")
}

func TestStatus(t *testing.T) {
	h := newHarness(t, severalOpen(2, 2))
	if got := h.say(alex, "!gtstatus"); got != "No vote right now  \nRecord: 0 certified, 0 rejected\n\nNominate: !gt Name. Pitch  \nWith photos: !!gt Name. Pitch  \nMore: !gtlist · !gthelp" {
		t.Fatalf("B119 with nothing at all, got %q", got)
	}
	h.send(alex, "!gt Juno Vale. Pitch")
	h.send(blair, "!!N")
	got := h.say(alex, "!gtstatus")
	contains(t, got, "Nomination 1: Juno Vale, 2/3 votes in: ✅ Alex · ❌ Blair · ⏳ Casey", "Add photos: !!gt Juno Vale", "Vote: !!Y yes")
	h.send(casey, "!!Y")
	h.now = start.Add(20 * time.Second)
	got = h.say(alex, "!gtstatus")
	contains(t, got, "Nomination 1: Juno Vale, 3/3 votes in, locked. Result in about 40s.", "Finish now: !final 1 (@Alex)")
	if strings.Contains(got, "Add photos") {
		t.Fatal("B122 no add-photos once locked")
	}
	h.now = start.Add(2 * time.Minute)
	replies := h.send(alex, "!gtstatus")
	if len(replies) != 2 || !strings.HasPrefix(replies[0].Text, "Rejected: Juno Vale (#1)") {
		t.Fatalf("B78 the due verdict comes before the status, got %+v", replies)
	}
	contains(t, replies[1].Text, "No vote right now", "Last: Juno Vale (#1), rejected", "Record: 0 certified, 1 rejected")
	report, err := h.bot.StatusReport(context.Background())
	if err != nil || !strings.HasPrefix(report, "No vote right now") {
		t.Fatalf("the CLI reads the same report, got %q, %v", report, err)
	}
}

func TestStatusWithSeveralOpen(t *testing.T) {
	h := newHarness(t, severalOpen(2, 2))
	h.send(alex, "!gt Juno Vale. Pitch !gt Remy Quill. Pitch")
	got := h.say(blair, "!gtstatus")
	contains(t, got, "Nomination 1: Juno Vale", "Nomination 2: Remy Quill", "Vote: !!Y 1 yes · !!N 1 no · !!C 1 clear")
	if strings.Contains(got, "Add photos") {
		t.Fatal("Q24 add-photos only with one open Nomination")
	}
}

func TestLists(t *testing.T) {
	h := newHarness(t, domainSettingsWithRenomination())
	reject(t, h, "Juno Vale")
	certify(t, h, "Remy Quill")
	if got := h.say(alex, "!gtlist"); got != "Winners (1)  \n🏆 Remy Quill\n\nNow voting: none  \nMore: !gtlist losers · !gtstatus" {
		t.Fatalf("B124 D17 winners, got %q", got)
	}
	if got := h.say(alex, "!gtlist LOSERS"); got != "Losers (1)  \n🚫 Juno Vale\n\nNow voting: none  \nMore: !gtlist winners · !gtstatus" {
		t.Fatalf("B123 B124 losers, got %q", got)
	}
	h.send(blair, "!gtrenominate Juno Vale. Again")
	contains(t, h.say(alex, "!gtlist losers"), "Losers (0)", "None yet", "Now voting: Juno Vale (#3, 1/3 votes in)")
	if got := h.say(alex, "!gtlist everyone"); got != "Use !gtlist, !gtlist winners, or !gtlist losers." {
		t.Fatalf("B123, got %q", got)
	}
}

var markdownStart = regexp.MustCompile(`^\s*([-*+#>]|\d+[.)])\s`)

func TestNoReplyLineStartsWithMarkdown(t *testing.T) {
	h := newHarness(t, severalOpen(3, 3), func(c *chat.Config) { c.Members = []chat.Member{alex, blair, {ID: casey.ID, Name: "- Casey"}} })
	h.ports.gifs = []string{"bell.gif"}
	for _, step := range []struct {
		from chat.Member
		text string
	}{
		{alex, `!gt "- 1. Juno" pitch !gt "# Remy" pitch`}, {blair, "!!Y 1"}, {casey, "!!N"}, {blair, "!gtstatus"}, {alex, "!gthelp"},
		{blair, "!gtlist"}, {alex, "!gt > Quote. pitch"}, {blair, "!gtshame @Alex 1 - * reason"}, {alex, "!!Y 1-2"}, {blair, "!!N 2"},
		{casey, "!!Y 1 !!Y 2"}, {alex, "!final"}, {alex, "!gtlist losers"}, {blair, "!gtfeedback - idea"},
	} {
		h.send(step.from, step.text)
	}
	for _, reply := range h.ports.outbox {
		for _, line := range strings.Split(reply.Text, "\n") {
			if markdownStart.MatchString(line) {
				t.Fatalf("B127 a line starts with Markdown syntax: %q in\n%s", line, reply.Text)
			}
		}
	}
}

func TestRepliesEscapeTypedNames(t *testing.T) {
	h := newHarness(t, oneAtATime)
	reply := h.say(alex, "!gt juno_vale *bold*. Pitch")
	contains(t, reply, `nominate Juno\_vale \*bold\* (#1)`)
}

func TestLayout(t *testing.T) {
	h := newHarness(t, oneAtATime)
	reply := h.say(alex, "!gthelp")
	sections := strings.Split(reply, "\n\n")
	if len(sections) != 2 || strings.Contains(reply, "\n\n\n") || !strings.Contains(sections[0], "  \n") {
		t.Fatalf("B126 lines joined by hard breaks, sections by one blank line, got %q", reply)
	}
}

func domainSettingsWithRenomination() domain.Settings {
	settings := oneAtATime
	settings.AllowRenomination = true
	return settings
}
