package chat_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gentalent/internal/nominations/delivery/chat"
	"gentalent/internal/nominations/domain"
)

func TestNominationRefusalsBeforeTheGroupIsAsked(t *testing.T) {
	long := strings.Repeat("a", 81)
	cases := []struct{ name, text, reply string }{
		{"B33 no name", "!gt", "Include a nominee name after !gt, e.g. !gt Ada Lovelace."},
		{"B33 no name with photos", "!!gt  ", "Include a nominee name after !!gt"},
		{"B33 a name of emoji only", "!gt 👀👀. Pitch", "Include a nominee name after !gt"},
		{"B35 a name over 80 characters", "!gt " + long, "Put a period after the nominee's name"},
		{"B35 one bad part refuses the whole message", "!gt Juno Vale. Pitch !gt " + long, "Put a period after the nominee's name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, severalOpen(3, 3))
			contains(t, h.say(alex, c.text), c.reply)
			if len(h.open()) != 0 {
				t.Fatal("nothing is nominated")
			}
		})
	}
}

func TestANomineeIsMatchedByTheirKey(t *testing.T) {
	h := newHarness(t, severalOpen(3, 3))
	h.send(alex, "!gt José Vale. Pitch")
	contains(t, h.say(blair, "!gt JOSÉ  vale 👀. Again"), "Already nominated: José Vale (#1)")
	h.send(blair, "!gt Jose Vale. Pitch")
	if got := h.nominee(2); got != "Jose Vale" {
		t.Fatalf("B34 accents are part of the key, so Jose Vale is another Person, got %q", got)
	}
}

func TestTheAnnouncement(t *testing.T) {
	h := newHarness(t, oneAtATime)
	replies := h.send(alex, "!gt Juno Vale. Pitch")
	want := "@Alex would like to nominate Juno Vale (#1) as a 2026 Generational Talent Nominee!  \n1/3 votes are in.\n\nVote: !!Y yes · !!N no · !!C clear  \nRules: !gthelp"
	if len(replies) != 1 || replies[0].Text != want || replies[0].Announces != 1 || replies[0].Key != "message-1/1" {
		t.Fatalf("B45 B46 the announcement opens at 1/3 and is the reaction target, got %+v", replies)
	}
	if got := h.nomination(1).Votes; len(got) != 1 || got[alex.ID] != domain.Yes {
		t.Fatalf("B45 the nominator's yes counts, got %+v", got)
	}
}

func TestTheGroupLimitOfOneNamesTheOpenNomination(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	got := h.say(blair, "!gt Remy Quill. Pitch")
	if got != "Finish voting on Juno Vale (#1) first (1/3 votes in). Respond with !!Y or !!N." {
		t.Fatalf("B37 R1, got %q", got)
	}
}

func TestPhotosForTheOpenNomination(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	got := h.say(blair, "!!gt juno vale. New pitch")
	if got != "Finding photos for Juno Vale (#1).  \n1/3 votes in." {
		t.Fatalf("B40 B49 !!gt on the open Nomination asks for its photos, got %q", got)
	}
	contains(t, h.say(casey, "!!gt Juno Vale"), "Photos were already requested for Juno Vale (#1)")
	searches, err := h.bot.ClaimPhotoSearches(context.Background())
	if err != nil || len(searches) != 1 {
		t.Fatalf("B48 one search is claimed, got %+v, %v", searches, err)
	}
	want := chat.PhotoSearch{Nomination: 1, Person: "Juno Vale", Pitch: "New pitch", Dir: filepath.Join("/media", "nominations", "1-juno-vale")}
	if searches[0] != want {
		t.Fatalf("B48 D26 the search names its Person, pitch and its own folder, got %+v", searches[0])
	}
}

func TestPhotosForALockedNominationAreRefused(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	h.send(blair, "!!Y")
	h.send(casey, "!!Y")
	contains(t, h.say(blair, "!!gt Juno Vale"), "Already nominated: Juno Vale (#1)")
}

func TestPreviouslyDecidedNominees(t *testing.T) {
	h := newHarness(t, oneAtATime)
	reject(t, h, "Juno Vale")
	if got := h.say(blair, "!gt juno vale. Again"); got != "Rejected before: Juno Vale (#1). They can't be nominated again." {
		t.Fatalf("B43 R6 a rejected Person, got %q", got)
	}
	certify(t, h, "Remy Quill")
	if got := h.say(blair, "!!gt Remy Quill"); got != "Already a Generational Talent: Remy Quill (#2)." {
		t.Fatalf("B44 D12 a certified Person, got %q", got)
	}
	if searches, _ := h.bot.ClaimPhotoSearches(context.Background()); len(searches) != 0 {
		t.Fatal("B43 B44 a refused !!gt asks for no photos")
	}
}

func TestRenomination(t *testing.T) {
	settings := oneAtATime
	settings.AllowRenomination = true
	h := newHarness(t, settings)
	reject(t, h, "Juno Vale")
	got := h.say(blair, "!gt Juno Vale. Again")
	if got != "Rejected before: Juno Vale (#1).  \nRenominate: !gtrenominate Juno Vale. Pitch" {
		t.Fatalf("R6 the refusal says how to renominate, got %q", got)
	}
	replies := h.send(blair, "!!gtrenominate juno vale. Better pitch")
	if len(replies) != 1 || replies[0].Announces != 2 {
		t.Fatalf("Q36 the renomination is announced as Nomination 2, got %+v", replies)
	}
	contains(t, replies[0].Text, "@Blair would like to renominate Juno Vale (#2)", "Photos are on the way.")
	contains(t, h.say(casey, "!gtrenominate Remy Quill. Pitch"), "Never nominated: Remy Quill. Nominate: !gt Remy Quill. Pitch")
}

func TestHelpStatusAndListAskForNoPhotos(t *testing.T) {
	h := newHarness(t, oneAtATime)
	for _, text := range []string{"!gthelp", "!gtstatus", "!gtlist", "!gt Juno Vale. Pitch"} {
		h.send(alex, text)
	}
	if searches, _ := h.bot.ClaimPhotoSearches(context.Background()); len(searches) != 0 {
		t.Fatalf("B54 B125 only !!gt asks for photos, got %+v", searches)
	}
}

func TestFoundPhotos(t *testing.T) {
	for _, layout := range []chat.PhotoLayout{chat.Individual, chat.Stitched} {
		t.Run(string(layout), func(t *testing.T) {
			h := newHarness(t, oneAtATime, func(c *chat.Config) { c.PhotoLayout = layout })
			h.send(alex, "!!gt Juno Vale. Pitch")
			if _, err := h.bot.ClaimPhotoSearches(context.Background()); err != nil {
				t.Fatal(err)
			}
			result := chat.PhotoResult{Nomination: 1, Photos: []string{"/p/01.jpg", "/p/02.jpg", "/p/03.jpg"}, Strip: "/p/strip.jpg", Provider: "codex", Summary: "Pioneer of orbital math.", Sources: "/p/SOURCES.txt"}
			if err := h.bot.AttachPhotos(context.Background(), result); err != nil {
				t.Fatal(err)
			}
			replies := h.fresh()
			if layout == chat.Stitched {
				if len(replies) != 1 || replies[0].Attachment != "/p/strip.jpg" || replies[0].Announces != 0 {
					t.Fatalf("B51 stitched: one message with the strip, got %+v", replies)
				}
			} else if len(replies) != 4 || replies[1].Text != "" || replies[1].Attachment != "/p/01.jpg" || replies[3].Key != "photos/1/4" {
				t.Fatalf("B51 individual: each photo alone with no text, in order, got %+v", replies)
			}
			got := h.nomination(1)
			if got.Strip != "/p/strip.jpg" || got.Exhibits[len(got.Exhibits)-1] != "AI media (codex): Pioneer of orbital math. [sources: /p/SOURCES.txt]" {
				t.Fatalf("B50 the strip and an AI media exhibit are kept, got %+v", got)
			}
		})
	}
}

func TestAFailedPhotoSearch(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!!gt Juno Vale. Pitch")
	h.bot.ClaimPhotoSearches(context.Background())
	if err := h.bot.AttachPhotos(context.Background(), chat.PhotoResult{Nomination: 1, Failure: "codex: exit 1 in /home/someone"}); err != nil {
		t.Fatal(err)
	}
	replies := h.fresh()
	if len(replies) != 1 || replies[0].Text != "No photos found for Juno Vale (#1)." {
		t.Fatalf("B52 D13 one short reply and never the provider's error, got %+v", replies)
	}
	if got := h.nomination(1); !got.Open() || got.Photos != domain.PhotosFailed {
		t.Fatalf("D13 the Nomination stays open without photos, got %+v", got)
	}
}

func TestAnInterruptedSearchIsAbandoned(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!!gt Juno Vale. Pitch")
	h.bot.ClaimPhotoSearches(context.Background())
	if err := h.bot.AbandonInterruptedSearches(context.Background()); err != nil {
		t.Fatal(err)
	}
	replies := h.fresh()
	if len(replies) != 1 || !strings.Contains(replies[0].Text, "photo search for Juno Vale (#1) was interrupted") {
		t.Fatalf("D7 the Group is told, got %+v", replies)
	}
	if again, _ := h.bot.ClaimPhotoSearches(context.Background()); len(again) != 0 {
		t.Fatal("D7 never rerun")
	}
	if running, _ := h.bot.SearchesRunning(context.Background()); len(running) != 0 {
		t.Fatalf("D27 nothing is running, got %v", running)
	}
}

func TestTheVerdictCarriesTheStripInTheIndividualLayout(t *testing.T) {
	strip := filepath.Join(t.TempDir(), "strip.jpg")
	if err := os.WriteFile(strip, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		layout chat.PhotoLayout
		strip  string
		want   int
	}{
		{"B87 individual with the strip", chat.Individual, strip, 2},
		{"B87 individual with the strip gone", chat.Individual, strip + ".gone", 1},
		{"B87 stitched never on the verdict", chat.Stitched, strip, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, oneAtATime, func(config *chat.Config) { config.PhotoLayout = c.layout })
			h.send(alex, "!!gt Juno Vale. Pitch")
			h.bot.ClaimPhotoSearches(context.Background())
			h.bot.AttachPhotos(context.Background(), chat.PhotoResult{Nomination: 1, Strip: c.strip, Photos: []string{"/p/1.jpg"}})
			h.fresh()
			h.send(blair, "!!N")
			h.send(casey, "!!Y")
			replies := h.send(alex, "!final")
			if len(replies) != c.want || (c.want == 2 && replies[1].Attachment != strip) {
				t.Fatalf("got %+v", replies)
			}
		})
	}
}

func TestANewPersonNamedTwiceInOneMessageIsNamedAsTyped(t *testing.T) {
	h := newHarness(t, oneAtATime)
	contains(t, h.say(alex, "!gt Remy Quill. One !gt remy quill. Two"), "Named twice in this message: Remy Quill.")
	if len(h.open()) != 0 {
		t.Fatalf("a refused message opened %+v", h.open())
	}
}
