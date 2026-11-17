package chat_test

import (
	"strings"
	"testing"
	"time"

	"gentalent/internal/nominations/delivery/chat"
	"gentalent/internal/nominations/domain"
)

func TestKeywords(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		reply string
	}{
		{"B8 help", "!gthelp", "How GenTalent works"},
		{"B8 help with two marks in capitals", "!!GTHELP", "How GenTalent works"},
		{"B9 status", "!!gtstatus", "No vote right now"},
		{"B10 list", "!!gtlist", "Winners (0)"},
		{"B18 help after the nominate keyword", "!gt help", "How GenTalent works"},
		{"B18 status after the nominate keyword", "!!gt STATUS", "No vote right now"},
		{"B19 list with a mode after the nominate keyword", "!gt list losers", "Losers (0)"},
		{"B12 an upper case suggestion", "!GTSUGGEST more GIFs", "your suggestion is logged"},
		{"B11 shame", "!gtshame", "Reply to their message with !gtshame reason"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			contains(t, newHarness(t, oneAtATime).say(alex, c.text), c.reply)
		})
	}
}

func TestTextThatIsNoCommandIsIgnored(t *testing.T) {
	for _, text := range []string{
		"", "   ", "!gthelp now", "!gtstatus please", "!gtlistx", "!!gtshame @Blair", "!gtx Ada", "!!gtx Ada",
		"!ghelp", "!gstatus", "!glist losers", "!!ghelp", "!!yesterday", "!!cat", "!!normal", "!final now", "!!final",
		"!gtrenominatex Ada", "hello !gt Ada",
	} {
		t.Run(text, func(t *testing.T) {
			h := newHarness(t, oneAtATime)
			h.silent(alex, text)
			if h.ports.handled["message-1"] {
				t.Fatal("B6 B8 B13 B14 B16 B17 text that is no command is not recorded")
			}
		})
	}
}

func TestOnlyMembersAreHeard(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.handle(chat.Message{Sender: "outsider", Text: "!gthelp"})
	if len(h.ports.outbox) != 0 {
		t.Fatal("B2 a sender outside the roster is ignored")
	}
}

func TestNomineeNames(t *testing.T) {
	cases := []struct {
		name, body, nominee, pitch string
	}{
		{"B23 a leading nominate word is dropped", "nominate juno vale. Pitch", "Juno Vale", "Pitch"},
		{"B24 a quoted name", `"remy  quill" — the pitch`, "Remy Quill", "the pitch"},
		{"B24 curly quotes", "“Juno Vale”: pitch", "Juno Vale", "pitch"},
		{"B25 a multi-line body", "Juno Vale. Inline\nsecond line\nthird", "Juno Vale", "Inline second line\nthird"},
		{"B26 a spaced hyphen", "juno vale - she rules", "Juno Vale", "she rules"},
		{"B26 a colon", "juno vale: she rules", "Juno Vale", "she rules"},
		{"B26 an exclamation", "juno vale!! she rules", "Juno Vale", "she rules"},
		{"B27 initials keep their dots", "j.k. vale. Pitch", "J.K. Vale", "Pitch"},
		{"B27 an honorific keeps its dot", "dr. juno vale. Pitch", "Dr. Juno Vale", "Pitch"},
		{"B28 a suffix keeps its dot", "remy quill jr. Pitch", "Remy Quill Jr.", "Pitch"},
		{"B29 no split point", "juno vale", "Juno Vale", ""},
		{"B30 shouting is calmed", "JUNO VALE. Pitch", "Juno Vale", "Pitch"},
		{"B30 mixed case pieces are kept", "juno-vale DeVale McQuill. Pitch", "Juno-Vale DeVale McQuill", "Pitch"},
		{"B30 one letter with a dot", "juno q. vale. Pitch", "Juno Q. Vale", "Pitch"},
		{"B31 emoji stay in the name", "juno vale 👀. Pitch", "Juno Vale 👀", "Pitch"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, oneAtATime)
			reply := h.say(alex, "!gt "+c.body)
			got := h.nomination(1)
			if got.Person.Name != c.nominee || got.Pitch != c.pitch {
				t.Fatalf("want %q with pitch %q, got %q with pitch %q", c.nominee, c.pitch, got.Person.Name, got.Pitch)
			}
			if c.pitch != "" && strings.Contains(reply, c.pitch) {
				t.Fatalf("B32 the pitch is never shown, got %q", reply)
			}
		})
	}
}

func TestTheAnnouncementYearIsTheMessageYear(t *testing.T) {
	h := newHarness(t, oneAtATime)
	sent := time.Date(2025, 12, 31, 23, 30, 0, 0, time.FixedZone("west", -5*3600))
	replies := h.handle(chat.Message{Sender: string(alex.ID), Text: "!gt Juno Vale. Pitch", SentAt: sent})
	contains(t, replies[0].Text, "as a 2025 Generational Talent Nominee")
}

func TestSeveralNominationsInOneMessage(t *testing.T) {
	h := newHarness(t, severalOpen(3, 3))
	replies := h.send(alex, "!gt Ada Lovelace. Engines !!gt Grace Hopper. Compilers")
	if len(replies) != 2 || replies[0].Announces != 1 || replies[1].Announces != 2 {
		t.Fatalf("Q8 each Nomination gets its own announcement, got %+v", replies)
	}
	contains(t, replies[1].Text, "Vote: !!Y 2 yes", "Photos are on the way.")
	if h.nomination(1).Photos != "" || h.nomination(2).Photos == "" {
		t.Fatal("Q8 each !gt or !!gt decides photos for its own nominee")
	}
}

func TestSeveralVotesInOneMessage(t *testing.T) {
	h := newHarness(t, severalOpen(3, 3))
	h.send(alex, "!gt Ada Lovelace. Engines !gt Grace Hopper. Compilers !gt Katherine Johnson. Orbits")

	replies := h.send(blair, "!!Y ada lovelace !!N gracehopper 👀 !!y 3")
	if len(replies) != 1 {
		t.Fatalf("one reply per message, got %+v", replies)
	}
	if h.nomination(1).Votes[blair.ID] != "yes" || h.nomination(2).Votes[blair.ID] != "no" || h.nomination(3).Votes[blair.ID] != "yes" {
		t.Fatal("Q11 Q18 each vote names its Nomination by name, squashed name or number")
	}
	h.send(casey, "!!N 1-3")
	for number := 1; number <= 3; number++ {
		if h.nomination(domainNumber(number)).Votes[casey.ID] != "no" {
			t.Fatalf("Q17 a range votes on every Nomination in it, missing %d", number)
		}
	}
	contains(t, h.say(alex, "!!C 2 !!Y gracie hoper"), "Did you mean Grace Hopper?")
	if h.nomination(2).Votes[alex.ID] != "yes" {
		t.Fatal("Q11 Q18 a near miss anywhere in the message counts nothing")
	}
	contains(t, h.say(alex, "!!N 3-1"), "Name Nominations by number")
}

func TestVoteWords(t *testing.T) {
	cases := map[string]domain.Vote{
		"!!y lol": domain.Yes, "!!yes!": domain.Yes, "!!n, sorry": domain.No, "!!NO": domain.No, "!!c": domain.NotVoted, "!!clear.": domain.NotVoted,
	}
	for text, want := range cases {
		t.Run(text, func(t *testing.T) {
			h := newHarness(t, oneAtATime)
			h.send(alex, "!gt Juno Vale. Pitch")
			h.send(blair, "!!n")
			h.send(blair, text)
			if got := h.nomination(1).Votes[blair.ID]; got != want {
				t.Fatalf("B15 %q votes %q with its trailing text ignored, got %q", text, want, got)
			}
		})
	}
}

func domainNumber(number int) domain.NominationNumber {
	return domain.NominationNumber(number)
}
