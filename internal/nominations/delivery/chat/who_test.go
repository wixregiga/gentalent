package chat_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"gentalent/internal/nominations/delivery/chat"
	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

func whoAllowed(limit int) domain.Settings {
	settings := fixesAllowed(100)
	settings.WhoSearchDailyLimit = limit
	return settings
}

func (h *harness) startWho(from chat.Member, description string) chat.WhoSearch {
	h.t.Helper()
	contains(h.t, h.say(from, "!!gtwho "+description), "Looking up who that is.")
	claimed, err := h.bot.ClaimWhoSearches(context.Background())
	if err != nil || len(claimed) != 1 {
		h.t.Fatalf("the started who search is claimed, got %+v, %v", claimed, err)
	}
	return claimed[0]
}

func (h *harness) answerWho(result chat.WhoResult) port.Reply {
	h.t.Helper()
	if err := h.bot.ReceiveWho(context.Background(), result); err != nil {
		h.t.Fatalf("ReceiveWho %+v: %v", result, err)
	}
	replies := h.fresh()
	if len(replies) != 1 {
		h.t.Fatalf("one reply answers a who search, got %+v", replies)
	}
	return replies[0]
}

func (h *harness) whoSearch(number int) domain.WhoSearch {
	h.t.Helper()
	search, ok := h.ports.group().WhoSearch(domain.WhoSearchNumber(number))
	if !ok {
		h.t.Fatalf("who search %d does not exist", number)
	}
	return search
}

func TestAWhoSearchIsClaimedOnceIntoItsOwnFolder(t *testing.T) {
	h := newHarness(t, whoAllowed(5))
	search := h.startWho(alex, "the   rear admiral who popularized the word  bug")
	want := chat.WhoSearch{Number: 1, Description: "the rear admiral who popularized the word bug", Dir: "/media/who/1"}
	if search != want {
		t.Fatalf("the claimed search carries its folded description and its folder, got %+v", search)
	}
	if again, _ := h.bot.ClaimWhoSearches(context.Background()); len(again) != 0 {
		t.Fatalf("a claimed who search is never claimed again, got %+v", again)
	}
	if running, _ := h.bot.WhoSearchesRunning(context.Background()); !reflect.DeepEqual(running, []int{1}) {
		t.Fatalf("busy sees the running who search, got %v", running)
	}
}

func TestAPublicFigureNotInTheGroupComesWithTheirPhoto(t *testing.T) {
	h := newHarness(t, whoAllowed(5))
	search := h.startWho(alex, "the rear admiral who popularized the word bug")
	photo := search.Dir + "/photo.jpg"
	reply := h.answerWho(chat.WhoResult{Number: search.Number, Name: "grace hopper", Blurb: "A computer scientist who built\n the first compiler.",
		Source: "https://example.org/grace-hopper", Photo: photo, Provider: "codex"})

	contains(t, reply.Text, `Who is "the rear admiral who popularized the word bug"?`,
		"Best match: Grace Hopper. A computer scientist who built the first compiler.", "Not in GenTalent yet.")
	if reply.Attachment != photo || strings.Contains(reply.Text, "example.org") || strings.Contains(reply.Text, "No photo") {
		t.Fatalf("the reply carries the one photo and never the source link, got %+v", reply)
	}
	found := h.whoSearch(search.Number)
	if found.Status != domain.WhoFound || found.Figure.Source != "https://example.org/grace-hopper" || found.Figure.Provider != "codex" || found.Figure.Person != "" {
		t.Fatalf("the source link and provider are kept as metadata, got %+v", found)
	}
	if len(h.open()) != 0 {
		t.Fatalf("a who search nominates no one, got %+v", h.open())
	}
	if running, _ := h.bot.WhoSearchesRunning(context.Background()); len(running) != 0 {
		t.Fatalf("an answered who search is no longer running, got %v", running)
	}
}

func TestAPublicFigureAlreadyInTheGroupSaysWhereTheyStand(t *testing.T) {
	h := newHarness(t, whoAllowed(5))
	h.send(alex, "!gt Ada Lovelace. Engines")
	h.send(blair, "!!Y")
	h.send(casey, "!!Y")
	h.send(alex, "!final")
	h.send(blair, "!gt Katherine Johnsen. Orbits")
	h.send(casey, "!gtfix 2 Katherine Johnson")
	h.send(alex, "!gtfixyes 2")
	h.send(blair, "!gtfixyes 2")
	if h.nominee(2) != "Katherine Johnson" {
		t.Fatalf("the setup fixes the name, got %q", h.nominee(2))
	}

	certified := h.startWho(blair, "the woman who wrote the first computer program")
	contains(t, h.answerWho(chat.WhoResult{Number: certified.Number, Name: "Ada Lovelace", Blurb: "A mathematician.", Photo: certified.Dir + "/ada.jpg"}).Text,
		"Already in: certified, #1")

	earlier := h.startWho(casey, "the mathematician who checked the orbit by hand")
	reply := h.answerWho(chat.WhoResult{Number: earlier.Number, Name: "Katherine Johnsen", Blurb: "A mathematician.", Photo: earlier.Dir + "/kj.jpg"})
	contains(t, reply.Text, "Already in as Katherine Johnson: nominated, #2")
	if person := h.whoSearch(earlier.Number).Figure.Person; person != h.nomination(2).Person {
		t.Fatalf("an earlier spelling is that Person, got %q", person)
	}
}

func TestAPublicFigureWithoutAUsablePhoto(t *testing.T) {
	h := newHarness(t, whoAllowed(5))
	for _, photo := range []string{"", "/etc/passwd", "/media/who/2/photo.jpg", "/media/who/1/../2/photo.jpg"} {
		search := h.startWho(alex, "the man who sailed around the world alone")
		reply := h.answerWho(chat.WhoResult{Number: search.Number, Name: "Joshua Slocum", Blurb: "A sailor.", Photo: photo})
		contains(t, reply.Text, "Best match: Joshua Slocum. A sailor.", "Not in GenTalent yet.", "No photo found.")
		if reply.Attachment != "" || h.whoSearch(search.Number).Figure.Photo != "" {
			t.Fatalf("a photo outside the search's own folder is never sent, %q gave %+v", photo, reply)
		}
	}
}

func TestNoMatchAndFailureAreSaidPlainly(t *testing.T) {
	h := newHarness(t, whoAllowed(5))
	cases := []struct {
		result chat.WhoResult
		want   string
		status domain.WhoSearchStatus
	}{
		{chat.WhoResult{NoMatch: true}, "No reliable public match for that description.", domain.WhoNoMatch},
		{chat.WhoResult{Failure: "codex: exit 1 in /home/someone"}, "I couldn't work out who that is. Try again with more detail.", domain.WhoFailed},
		{chat.WhoResult{Name: " ... ", Blurb: "Nobody."}, "I couldn't work out who that is. Try again with more detail.", domain.WhoFailed},
	}
	for _, c := range cases {
		search := h.startWho(alex, "someone from somewhere")
		c.result.Number = search.Number
		reply := h.answerWho(c.result)
		contains(t, reply.Text, `Who is "someone from somewhere"?`, c.want)
		if strings.Contains(reply.Text, "codex") || reply.Attachment != "" {
			t.Fatalf("never the provider's error, got %+v", reply)
		}
		if got := h.whoSearch(search.Number); got.Status != c.status {
			t.Fatalf("want %s, got %+v", c.status, got)
		}
		if err := h.bot.ReceiveWho(context.Background(), chat.WhoResult{Number: search.Number, Name: "Ada Lovelace"}); err == nil {
			t.Fatal("an ended who search is never answered again")
		}
	}
}

func TestAnInterruptedWhoSearchIsAbandoned(t *testing.T) {
	h := newHarness(t, whoAllowed(5))
	search := h.startWho(alex, "the woman who wrote the first computer program")
	if err := h.bot.AbandonInterruptedSearches(context.Background()); err != nil {
		t.Fatal(err)
	}
	replies := h.fresh()
	if len(replies) != 1 || !strings.Contains(replies[0].Text, `Who is "the woman who wrote the first computer program"?`) ||
		!strings.Contains(replies[0].Text, "The search was interrupted, so it has no answer.") {
		t.Fatalf("the Group is told the search was interrupted, got %+v", replies)
	}
	if again, _ := h.bot.ClaimWhoSearches(context.Background()); len(again) != 0 {
		t.Fatal("an interrupted who search never runs again")
	}
	if running, _ := h.bot.WhoSearchesRunning(context.Background()); len(running) != 0 {
		t.Fatalf("nothing is running, got %v", running)
	}
	if err := h.bot.ReceiveWho(context.Background(), chat.WhoResult{Number: search.Number, Name: "Ada Lovelace"}); err == nil {
		t.Fatal("a late answer to an abandoned search is refused")
	}
	if got := h.whoSearch(search.Number); got.Status != domain.WhoFailed {
		t.Fatalf("an interrupted search is recorded failed, got %+v", got)
	}
}

func TestTheDailyLimitRefusalSaysWhenTheNextSearchOpens(t *testing.T) {
	h := newHarness(t, whoAllowed(1))
	h.startWho(alex, "the woman who wrote the first computer program")
	h.now = start.Add(time.Hour)
	contains(t, h.say(alex, "!!gtwho the man who sailed around the world alone"),
		"You've used your 1 who search for the last 24 hours. Your next one opens in 23 hours.")
	h.now = start.Add(90 * time.Minute)
	contains(t, h.say(alex, "!!gtwho the man who sailed around the world alone"), "Your next one opens in 22 hours 30 minutes.")
	if claimed, _ := h.bot.ClaimWhoSearches(context.Background()); len(claimed) != 0 {
		t.Fatalf("a refused who search starts nothing, got %+v", claimed)
	}
	h.startWho(blair, "the man who sailed around the world alone")
	h.now = start.Add(24 * time.Hour)
	h.startWho(alex, "the man who sailed around the world alone")

	off := newHarness(t, whoAllowed(0))
	if text := off.say(alex, "!!gtwho the woman who wrote the first computer program"); text != "Who searches are turned off for this Group." {
		t.Fatalf("a limit of 0 turns who searches off, got %q", text)
	}
	if strings.Contains(off.say(alex, "!gthelp"), "!!gtwho") {
		t.Fatal("help leaves out a turned off command")
	}
}

func TestOnlyADescribedDoubleBangStartsAWhoSearch(t *testing.T) {
	h := newHarness(t, whoAllowed(5))
	contains(t, h.say(alex, "!!gtwho"), "Find out who someone is: !!gtwho")
	contains(t, h.say(alex, "!!gtwho   "), "Find out who someone is: !!gtwho")
	contains(t, h.say(alex, "!!gtwho "+strings.Repeat("a", 301)), "A description can be at most 300 characters.")
	h.silent(alex, "!gtwho the woman who wrote the first computer program")
	if claimed, _ := h.bot.ClaimWhoSearches(context.Background()); len(claimed) != 0 {
		t.Fatalf("nothing starts without a two-bang description, got %+v", claimed)
	}
	contains(t, h.say(alex, "!gthelp"), "!!gtwho")
}
