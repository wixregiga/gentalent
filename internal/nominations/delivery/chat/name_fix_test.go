package chat_test

import (
	"strings"
	"testing"
	"time"

	"gentalent/internal/nominations/domain"
)

func fixesAllowed(percentage int) domain.Settings {
	return domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 3, AllowRenomination: true,
		AllowNameFixes: true, NameFixAgreementPercentage: percentage, NameFixWindow: 24 * time.Hour}
}

func TestAMisspelledNameIsFixedWhenEveryMemberAgrees(t *testing.T) {
	h := newHarness(t, fixesAllowed(100))
	h.send(blair, "!gt Grace Hopper. Compilers")
	h.send(alex, "!gt Ada Lovelase. Engines")

	contains(t, h.say(casey, "!gtfix 2 Ada Lovelace"),
		"Fix for #2: Ada Lovelase → Ada Lovelace? Votes carry over.", "@Casey proposed it. It needs 3 of 3 Members.",
		"Agree: !gtfixyes 2 · Stop: !gtfixno 2", "It lapses in 24 hours")
	contains(t, h.say(alex, "!gtfixyes 2"), "Fix for #2 (Ada Lovelase → Ada Lovelace): @Alex agrees. 2 of 3 agree; it needs 3.")
	if got := h.nominee(2); got != "Ada Lovelase" {
		t.Fatalf("two of three agreeing changed the name to %q", got)
	}
	contains(t, h.say(blair, "!gtfixyes 2"), "Name fixed for #2: Ada Lovelase → Ada Lovelace. Votes and history carried over.")
	if got := h.nomination(2); h.nominee(2) != "Ada Lovelace" || got.Votes[alex.ID] != domain.Yes || len(got.Votes) != 1 {
		t.Fatalf("after the fix: %+v", got)
	}
	if h.nominee(1) != "Grace Hopper" {
		t.Fatalf("the fix renamed the other Nomination: %q", h.nominee(1))
	}

	contains(t, h.say(casey, "!!Y ada lovelase"), "Ada Lovelace (#2)")
	contains(t, h.say(blair, "!!Y Ada Lovelace"), "Ada Lovelace (#2)")
	if votes := h.nomination(2).Votes; votes[casey.ID] != domain.Yes || votes[blair.ID] != domain.Yes {
		t.Fatalf("votes by the old and new spellings: %+v", votes)
	}
}

func TestBothSpellingsKeepNamingTheFixedPerson(t *testing.T) {
	h := newHarness(t, fixesAllowed(100))
	h.send(alex, "!gt Ada Lovelase. Engines")
	h.send(alex, "!gtfix 1 Ada Lovelace")
	h.send(blair, "!gtfixyes 1")
	h.send(casey, "!gtfixyes 1")

	for _, typed := range []string{"!gt Ada Lovelase. Again", "!gt ada lovelace. Again"} {
		contains(t, h.say(blair, typed), "Already nominated: Ada Lovelace (#1)")
	}
	if len(h.open()) != 1 {
		t.Fatalf("a spelling of the fixed Person opened another Nomination: %+v", h.open())
	}
}

func TestOneNoStopsAFixThatNeedsEveryMember(t *testing.T) {
	h := newHarness(t, fixesAllowed(100))
	h.send(alex, "!gt Ada Lovelase. Engines")
	h.send(alex, "!gtfix 1 Ada Lovelace")
	contains(t, h.say(blair, "!gtfixno 1"), "Fix for #1 stopped. Ada Lovelase (#1) keeps that name.")
	contains(t, h.say(casey, "!gtfixyes 1"), "No fix is waiting for Ada Lovelase (#1). Propose one: !gtfix 1 Correct Name")
	if got := h.nominee(1); got != "Ada Lovelase" {
		t.Fatalf("a stopped fix changed the name to %q", got)
	}
	contains(t, h.say(blair, "!gtfix 1 Ada Lovelace"), "Fix for #1: Ada Lovelase → Ada Lovelace?")
	contains(t, h.say(blair, "!gtfixno 1"), "Fix for #1 withdrawn.")
}

func TestAFixNeedsOnlyTheConfiguredShareOfMembers(t *testing.T) {
	h := newHarness(t, fixesAllowed(50))
	h.send(alex, "!gt Ada Lovelase. Engines")
	contains(t, h.say(alex, "!gtfix 1 Ada Lovelace"), "It needs 2 of 3 Members.")
	contains(t, h.say(blair, "!gtfixno 1"), "Fix for #1 (Ada Lovelase → Ada Lovelace): @Blair says no. 1 of 3 agree; it needs 2.")
	contains(t, h.say(casey, "!gtfixyes 1"), "Name fixed for #1")
}

func TestAnUnansweredFixLapsesAfterItsWindow(t *testing.T) {
	h := newHarness(t, fixesAllowed(100))
	h.send(alex, "!gt Ada Lovelase. Engines")
	h.send(alex, "!gtfix 1 Ada Lovelace")
	h.now = h.now.Add(24 * time.Hour)
	var lapsed bool
	for _, reply := range h.send(blair, "!gtstatus") {
		lapsed = lapsed || strings.Contains(reply.Text, "The fix for #1 lapsed without enough agreement. Ada Lovelase (#1) keeps that name.")
	}
	if !lapsed {
		t.Fatal("no lapse reply after the window ended")
	}
	contains(t, h.say(casey, "!gtfixyes 1"), "No fix is waiting for Ada Lovelase (#1).")
}

func TestSeveralFixesInOneMessageAreAnsweredOneByOne(t *testing.T) {
	h := newHarness(t, fixesAllowed(100))
	h.send(alex, "!gt Ada Lovelase. Engines")
	h.send(blair, "!gt Grace Hoper. Compilers")
	replies := h.send(casey, "!gtfix 1 Ada Lovelace\n!gtfix 2 Grace Hopper\n!gtfix 9 Nobody")
	if len(replies) != 3 {
		t.Fatalf("want a reply per fix, got %+v", replies)
	}
	contains(t, replies[0].Text, "Fix for #1", "!gtfixyes 1")
	contains(t, replies[1].Text, "Fix for #2", "!gtfixyes 2")
	contains(t, replies[2].Text, "There is no Nomination 9.")
}

func TestAFixWithPhotosSearchesUnderTheNewName(t *testing.T) {
	h := newHarness(t, fixesAllowed(100))
	h.send(alex, "!gt Ada Lovelase. Engines")
	contains(t, h.say(alex, "!!gtfix 1 Ada Lovelace"), "Photos are searched under the new name once it's fixed.")
	h.send(blair, "!gtfixyes 1")
	contains(t, h.say(casey, "!gtfixyes 1"), "Name fixed for #1", "Photos are on the way.")
	if got := h.nomination(1); got.Photos != domain.PhotosRequested || h.nominee(1) != "Ada Lovelace" {
		t.Fatalf("after a fix with photos: %+v", got)
	}

	h = newHarness(t, fixesAllowed(100))
	h.send(alex, "!!gt Ada Lovelase. Engines")
	contains(t, h.say(alex, "!!gtfix 1 Ada Lovelace"), "Photos were already searched for Ada Lovelase (#1). Fix the name alone with !gtfix 1.")
}

func TestNameFixRefusals(t *testing.T) {
	off := fixesAllowed(100)
	off.AllowNameFixes = false
	h := newHarness(t, off)
	h.send(alex, "!gt Ada Lovelase. Engines")
	contains(t, h.say(alex, "!gtfix 1 Ada Lovelace"), "Name fixes are turned off for this Group.")

	h = newHarness(t, fixesAllowed(100))
	h.send(alex, "!gt Ada Lovelase. Engines")
	h.send(blair, "!gt Grace Hopper. Compilers")
	contains(t, h.say(casey, "!gtfix 1 Grace Hopper"), "That spelling already names Grace Hopper (#2). Pick a different spelling.")
	contains(t, h.say(casey, "!gtfix 1 Ada Lovelase"), "Ada Lovelase (#1) is already spelled that way.")
	contains(t, h.say(casey, "!gtfix Ada Lovelace"), "Fix a name: !gtfix 12 Ada Lovelace")
	contains(t, h.say(casey, "!gtfixyes"), "Answer a fix by its Nomination's number: !gtfixyes 12 or !gtfixno 12")
	contains(t, h.say(casey, "!gtfixyes 7"), "There is no Nomination 7.")
	contains(t, h.say(casey, "!gtfixyes 2"), "No fix is waiting for Grace Hopper (#2).")

	h.send(casey, "!gtfix 1 Ada Lovelace")
	contains(t, h.say(blair, "!gtfix 1 Ada King"), "A fix is already waiting for #1: Ada Lovelase → Ada Lovelace.", "Agree: !gtfixyes 1")
	h.send(alex, "!gtfixyes 1")
	h.send(blair, "!gtfixyes 1")
	contains(t, h.say(blair, "!gtfix 1 Ada King"), "Ada Lovelace (#1) already had their one name fix.")
}

func TestHelpShowsNameFixesOnlyWhenAllowed(t *testing.T) {
	contains(t, newHarness(t, fixesAllowed(100)).say(alex, "!gthelp"), "Fix a misspelled name: !gtfix 12 Ada Lovelace")
	off := fixesAllowed(100)
	off.AllowNameFixes = false
	if help := newHarness(t, off).say(alex, "!gthelp"); strings.Contains(help, "!gtfix") {
		t.Fatalf("help offers name fixes that are turned off:\n%s", help)
	}
}

func TestAFixIsAnsweredByAnyNominationOfItsPerson(t *testing.T) {
	h := newHarness(t, fixesAllowed(100))
	h.send(alex, "!gt Ada Lovelase. Engines")
	h.send(blair, "!!N")
	h.send(casey, "!!N")
	h.now = h.now.Add(2 * time.Minute)
	h.send(alex, "!gtstatus")
	if h.nomination(1).Status != domain.Rejected {
		t.Fatalf("Nomination 1 is %s", h.nomination(1).Status)
	}
	h.send(blair, "!gtrenominate Ada Lovelase. Again")
	contains(t, h.say(casey, "!gtfix 1 Ada Lovelace"), "Fix for #1: Ada Lovelase → Ada Lovelace?")
	contains(t, h.say(alex, "!gtfixyes 2"), "Fix for #1 (Ada Lovelase → Ada Lovelace): @Alex agrees.")
	contains(t, h.say(blair, "!gtfixyes 2"), "Name fixed for #1")
	if h.nominee(1) != "Ada Lovelace" || h.nominee(2) != "Ada Lovelace" {
		t.Fatalf("both Nominations of the Person show %q and %q", h.nominee(1), h.nominee(2))
	}
}
