package chat_test

import (
	"time"

	"testing"

	"gentalent/internal/nominations/domain"
)

func severalOpen(group, member int) domain.Settings {
	return domain.Settings{GroupOpenNominationLimit: group, MemberOpenNominationLimit: member, SingleMessageNominationLimit: 3, NameFixAgreementPercentage: 100, NameFixWindow: 24 * time.Hour}
}

func TestScenarioANominationBeyondTheGroupLimitIsRefused(t *testing.T) {
	h := newHarness(t, severalOpen(2, 1))
	h.send(blair, "!gt Ada Lovelace. Engines")
	h.send(casey, "!gt Grace Hopper. Compilers")

	reply := h.say(alex, "!gt Katherine Johnson. Orbital math")
	if len(h.open()) != 2 {
		t.Fatalf("no Nomination is opened for Katherine Johnson, open: %+v", h.open())
	}
	contains(t, reply, "Nomination 1: Ada Lovelace", "Nomination 2: Grace Hopper", "!!Y 1")
}

func TestScenarioLoweringTheLimitRefusesOnlyNewNominations(t *testing.T) {
	h := newHarness(t, severalOpen(2, 1))
	h.send(blair, "!gt Ada Lovelace. Engines")
	h.send(casey, "!gt Grace Hopper. Compilers")
	lowered := newHarness(t, severalOpen(1, 1))
	lowered.ports.persons, lowered.ports.nominations = h.ports.persons, h.ports.nominations

	contains(t, lowered.say(alex, "!gt Katherine Johnson. Orbital math"), "Finish voting first")
	lowered.say(alex, "!!Y 1")
	if got := lowered.nomination(1).Votes[alex.ID]; got != domain.Yes {
		t.Fatalf("!!Y 1 still counts as Alex's vote on Nomination 1, got %q", got)
	}
}

func TestScenarioAMemberAtTheMemberLimitCannotNominateAgain(t *testing.T) {
	h := newHarness(t, severalOpen(3, 1))
	h.send(alex, "!gt Ada Lovelace. Engines")

	contains(t, h.say(alex, "!gt Grace Hopper. Compilers"), "You already have 1 open Nominations", "Nomination 1: Ada Lovelace")
	h.send(blair, "!gt Grace Hopper. Compilers")
	if got := h.nomination(2); h.nominee(2) != "Grace Hopper" || got.Nominator != blair.ID {
		t.Fatalf("Blair can still nominate Grace Hopper, got %+v", got)
	}
}

func TestScenarioAMessageOverTheSingleMessageLimitNominatesNobody(t *testing.T) {
	settings := severalOpen(3, 3)
	settings.SingleMessageNominationLimit = 2
	h := newHarness(t, settings)

	reply := h.say(alex, "!gt Ada Lovelace. Engines !gt Grace Hopper. Compilers !gt Katherine Johnson. Orbits")
	if len(h.open()) != 0 {
		t.Fatalf("no Nomination is opened, got %+v", h.open())
	}
	contains(t, reply, "At most 2 people can be nominated in one message.")
}

func TestScenarioAnUnnamedVoteWithSeveralOpenCountsNothing(t *testing.T) {
	h := newHarness(t, severalOpen(2, 1))
	h.send(alex, "!gt Ada Lovelace. Engines")
	h.send(casey, "!gt Grace Hopper. Compilers")

	reply := h.say(blair, "!!Y")
	if len(h.nomination(1).Votes) != 1 || len(h.nomination(2).Votes) != 1 {
		t.Fatal("no vote is recorded")
	}
	contains(t, reply, "Nomination 1: Ada Lovelace", "Nomination 2: Grace Hopper")
}

func TestScenarioWithOneOpenNominationAVoteNeedsNoName(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Ada Lovelace. Engines")
	h.say(blair, "!!Y")
	if got := h.nomination(1).Votes[blair.ID]; got != domain.Yes {
		t.Fatalf("Blair's yes is recorded on Nomination 1, got %q", got)
	}
}

func TestScenarioAPersonWithAnOpenNominationCannotBeNominatedAgain(t *testing.T) {
	h := newHarness(t, severalOpen(3, 3))
	h.send(alex, "!gt Ada Lovelace. Engines")
	reply := h.say(blair, "!gt Ada Lovelace. Engines")
	if len(h.open()) != 1 {
		t.Fatalf("no Nomination is opened, got %+v", h.open())
	}
	contains(t, reply, "Already nominated: Ada Lovelace (#1)")
}

func reject(t *testing.T, h *harness, name string) {
	t.Helper()
	h.send(alex, "!gt "+name+". A pitch")
	h.send(blair, "!!N")
	h.send(casey, "!!N")
	h.send(alex, "!final")
}

func certify(t *testing.T, h *harness, name string) {
	t.Helper()
	h.send(alex, "!gt "+name+". A pitch")
	h.send(blair, "!!Y")
	h.send(casey, "!!Y")
	h.send(alex, "!final")
}

func TestScenarioNominatingAPreviouslyRejectedPersonIsRefused(t *testing.T) {
	h := newHarness(t, oneAtATime)
	reject(t, h, "Ada Lovelace")
	contains(t, h.say(blair, "!gt Ada Lovelace. Engines"), "Rejected before: Ada Lovelace (#1)")
	if len(h.open()) != 0 {
		t.Fatalf("no Nomination is opened, got %+v", h.open())
	}
}

func TestScenarioACertifiedPersonCannotBeRenominated(t *testing.T) {
	settings := oneAtATime
	settings.AllowRenomination = true
	h := newHarness(t, settings)
	reject(t, h, "Ada Lovelace")
	certify(t, h, "Grace Hopper")
	contains(t, h.say(blair, "!gtrenominate Grace Hopper. Compilers"), "Already a Generational Talent: Grace Hopper (#2)")
	if len(h.open()) != 0 {
		t.Fatalf("no Renomination is opened, got %+v", h.open())
	}
}

func TestScenarioRenominationIsRefusedWhileTheGroupDoesNotAllowIt(t *testing.T) {
	h := newHarness(t, oneAtATime)
	reject(t, h, "Ada Lovelace")
	contains(t, h.say(blair, "!gtrenominate Ada Lovelace. Engines"), "Renomination is turned off")
	if len(h.open()) != 0 {
		t.Fatalf("no Renomination is opened, got %+v", h.open())
	}
}
