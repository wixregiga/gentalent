package domain_test

import (
	"errors"
	"testing"
	"time"

	"gentalent/internal/nominations/domain"
)

func fixing(percentage int) domain.Settings {
	return domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 3, AllowRenomination: true,
		AllowNameFixes: true, NameFixAgreementPercentage: percentage, NameFixWindow: 24 * time.Hour}
}

var misspelled = domain.Person{ID: "adalovelase", Name: "Ada Lovelase"}

func proposal(number domain.NominationNumber, name string) domain.NameFixProposal {
	return domain.NameFixProposal{Nomination: number, Name: name, Spelling: spelling(name)}
}

func propose(t *testing.T, group *domain.Group, by member, p domain.NameFixProposal) domain.NameFix {
	t.Helper()
	fix, err := group.ProposeNameFix(by.ID, p, today)
	if err != nil {
		t.Fatalf("ProposeNameFix: %v", err)
	}
	return fix
}

func agree(t *testing.T, group *domain.Group, by member, number domain.NominationNumber) domain.NameFix {
	t.Helper()
	fix, err := group.AgreeToNameFix(by.ID, number, today)
	if err != nil {
		t.Fatalf("AgreeToNameFix: %v", err)
	}
	return fix
}

func personName(t *testing.T, group *domain.Group, number domain.NominationNumber) string {
	t.Helper()
	nomination, ok := group.Nomination(number)
	if !ok {
		t.Fatalf("no Nomination %d", number)
	}
	return named(t, group, nomination.Person)
}

func TestNameFixedOnlyWhenEnoughMembersAgree(t *testing.T) {
	group := newGroup(t, fixing(100), open(1, misspelled, alex))
	fix := propose(t, group, alex, proposal(1, "Ada Lovelace"))
	if !fix.Pending() || personName(t, group, 1) != "Ada Lovelase" {
		t.Fatalf("one of three agreeing fixed the name: %+v", fix)
	}
	if fix = agree(t, group, blair, fix.Nomination); !fix.Pending() || personName(t, group, 1) != "Ada Lovelase" {
		t.Fatalf("two of three agreeing fixed the name at 100%%: %+v", fix)
	}
	if fix = agree(t, group, casey, fix.Nomination); fix.Status != domain.NameFixed || personName(t, group, 1) != "Ada Lovelace" {
		t.Fatalf("every Member agreed and the name is %q: %+v", personName(t, group, 1), fix)
	}

	group = newGroup(t, fixing(50), open(1, misspelled, alex))
	fix = propose(t, group, blair, proposal(1, "Ada Lovelace"))
	if !fix.Pending() {
		t.Fatalf("one of three is not half: %+v", fix)
	}
	if fix = agree(t, group, casey, fix.Nomination); fix.Status != domain.NameFixed {
		t.Fatalf("two of three reach 50%%: %+v", fix)
	}

	group = newGroup(t, fixing(30), open(1, misspelled, alex))
	if fix = propose(t, group, casey, proposal(1, "Ada Lovelace")); fix.Status != domain.NameFixed {
		t.Fatalf("the proposer alone reaches 30%% of three: %+v", fix)
	}
}

func TestNameFixCarriesTheWholePersonAndTheirVotes(t *testing.T) {
	history := []domain.Nomination{finalized(1, misspelled, domain.Rejected), open(2, misspelled, blair)}
	group := newGroup(t, fixing(100), history...)
	fix := propose(t, group, casey, proposal(2, "Ada Lovelace"))
	agree(t, group, alex, fix.Nomination)
	agree(t, group, blair, fix.Nomination)
	for _, number := range []domain.NominationNumber{1, 2} {
		nomination, _ := group.Nomination(number)
		if nomination.Person != misspelled.ID || named(t, group, nomination.Person) != "Ada Lovelace" {
			t.Fatalf("Nomination %d names %+v", number, nomination.Person)
		}
	}
	if finalizedOne, _ := group.Nomination(1); finalizedOne.Status != domain.Rejected || len(finalizedOne.Votes) != 3 {
		t.Fatalf("the fix changed an outcome: %+v", finalizedOne)
	}
	if openOne, _ := group.Nomination(2); openOne.Votes[blair.ID] != domain.Yes || len(openOne.Votes) != 1 {
		t.Fatalf("the fix changed votes: %+v", openOne.Votes)
	}
}

func TestCertifiedPersonCanBeFixed(t *testing.T) {
	group := newGroup(t, fixing(100), finalized(1, misspelled, domain.Certified))
	fix := propose(t, group, alex, proposal(1, "Ada Lovelace"))
	agree(t, group, blair, fix.Nomination)
	agree(t, group, casey, fix.Nomination)
	if nomination, _ := group.Nomination(1); named(t, group, nomination.Person) != "Ada Lovelace" || nomination.Status != domain.Certified {
		t.Fatalf("certified Person after the fix: %+v", nomination)
	}
}

func TestOnePendingNameFixPerPerson(t *testing.T) {
	group := newGroup(t, fixing(100), open(1, misspelled, alex))
	first := propose(t, group, alex, proposal(1, "Ada Lovelace"))
	_, err := group.ProposeNameFix(blair.ID, proposal(1, "Ada King"), today)
	if refused := refusal(t, err, domain.OnePendingNameFixPerPerson); refused.Fix != first.Number {
		t.Fatalf("the refusal names fix %d, want the pending %d", refused.Fix, first.Number)
	}
	history := []domain.Nomination{open(1, misspelled, alex)}
	if _, err := domain.NewGroup("group-test", memberIDs, fixing(100), personsOf(history), history, nil, []domain.NameFix{
		{Number: 1, Nomination: 1, Person: misspelled.ID, Status: domain.NameFixPending},
		{Number: 2, Nomination: 1, Person: misspelled.ID, Status: domain.NameFixPending},
	}); !errors.As(err, new(domain.Refusal)) {
		t.Fatalf("a Group loaded with two pending fixes for one Person: %v", err)
	}
	if _, err := group.StopNameFix(alex.ID, first.Nomination, today); err != nil {
		t.Fatal(err)
	}
	propose(t, group, blair, proposal(1, "Ada King"))
}

func TestNameFixedOncePerPerson(t *testing.T) {
	group := newGroup(t, fixing(100), open(1, misspelled, alex))
	stopped := propose(t, group, alex, proposal(1, "Ada Lovelacey"))
	if _, err := group.StopNameFix(blair.ID, stopped.Nomination, today); err != nil {
		t.Fatal(err)
	}
	fix := propose(t, group, blair, proposal(1, "Ada Lovelace"))
	agree(t, group, alex, fix.Nomination)
	agree(t, group, casey, fix.Nomination)
	_, err := group.ProposeNameFix(casey.ID, proposal(1, "Ada King"), today)
	if refused := refusal(t, err, domain.NameFixedOncePerPerson); refused.Fix != fix.Number {
		t.Fatalf("the refusal names fix %d, want %d", refused.Fix, fix.Number)
	}
}

func TestNameFixNeverTakesAnotherPersonsSpelling(t *testing.T) {
	group := newGroup(t, fixing(100), open(1, misspelled, alex), finalized(2, grace, domain.Certified))
	_, err := group.ProposeNameFix(blair.ID, proposal(1, "Grace Hopper"), today)
	if refused := refusal(t, err, domain.NameFixNeverTakesAnotherPersonsSpelling); refused.Ref.Person != grace.ID {
		t.Fatalf("the refusal names %q, want %q", refused.Ref.Person, grace.ID)
	}
	if _, err := group.ProposeNameFix(blair.ID, proposal(1, "grace HOPPER!"), today); !errors.As(err, new(domain.Refusal)) {
		t.Fatalf("a spelling folding to another Person's was accepted: %v", err)
	}

	pending := propose(t, group, blair, proposal(1, "Ada Lovelace"))
	if _, err := group.Nominate(casey.ID, candidates(domain.Person{ID: "adalovelace", Name: "Ada Lovelace"}), today); err != nil {
		t.Fatal(err)
	}
	agree(t, group, alex, pending.Nomination)
	_, err = group.AgreeToNameFix(casey.ID, pending.Nomination, today)
	refusal(t, err, domain.NameFixNeverTakesAnotherPersonsSpelling)
	if personName(t, group, 1) != "Ada Lovelase" {
		t.Fatalf("a fix took a spelling another Person got while it waited")
	}

	group = newGroup(t, fixing(100), open(1, domain.Person{ID: "ada-in-lower-case", Name: "ada lovelace"}, alex))
	fix := propose(t, group, alex, proposal(1, "Ada Lovelace"))
	agree(t, group, blair, fix.Nomination)
	if fix = agree(t, group, casey, fix.Nomination); fix.Status != domain.NameFixed {
		t.Fatalf("fixing only the capitals of the Person's own spelling was refused: %+v", fix)
	}
}

func TestEarlierSpellingsStillNameThePerson(t *testing.T) {
	group := newGroup(t, fixing(100), finalized(1, misspelled, domain.Rejected))
	fix := propose(t, group, alex, proposal(1, "Ada Lovelace"))
	agree(t, group, blair, fix.Nomination)
	agree(t, group, casey, fix.Nomination)
	spellings := group.Spellings()
	for _, typed := range []string{"Ada Lovelase", "Ada Lovelace"} {
		if spellings[spelling(typed)] != misspelled.ID {
			t.Fatalf("%q names %q, want %q", typed, spellings[spelling(typed)], misspelled.ID)
		}
	}
	_, err := group.Nominate(blair.ID, []domain.Candidate{{Spelling: spelling("Ada Lovelase"), Name: "Ada Lovelase"}}, today)
	refusal(t, err, domain.RenominatedOnlyThroughRenomination)
	renominated, err := group.Renominate(blair.ID, []domain.Candidate{{Spelling: spelling("Ada Lovelase"), Name: "Ada Lovelase"}}, today)
	if err != nil || renominated[0].Person != misspelled.ID || named(t, group, renominated[0].Person) != "Ada Lovelace" {
		t.Fatalf("renominating by the old spelling: %+v, %v", renominated, err)
	}
}

func TestNameFixStoppedWhenAgreementIsOutOfReach(t *testing.T) {
	group := newGroup(t, fixing(100), open(1, misspelled, alex))
	fix := propose(t, group, alex, proposal(1, "Ada Lovelace"))
	stopped, err := group.StopNameFix(casey.ID, fix.Nomination, today)
	if err != nil || stopped.Status != domain.NameFixStopped {
		t.Fatalf("one no at 100%%: %+v, %v", stopped, err)
	}
	_, err = group.AgreeToNameFix(blair.ID, fix.Nomination, today)
	refusal(t, err, domain.NoPendingNameFix)

	group = newGroup(t, fixing(50), open(1, misspelled, alex))
	fix = propose(t, group, alex, proposal(1, "Ada Lovelace"))
	if stopped, _ = group.StopNameFix(blair.ID, fix.Nomination, today); !stopped.Pending() {
		t.Fatalf("one no of three still leaves two of three in reach at 50%%: %+v", stopped)
	}
	if stopped, _ = group.StopNameFix(casey.ID, fix.Nomination, today); stopped.Status != domain.NameFixStopped {
		t.Fatalf("two noes leave 50%% of three out of reach: %+v", stopped)
	}

	group = newGroup(t, fixing(50), open(1, misspelled, alex))
	fix = propose(t, group, alex, proposal(1, "Ada Lovelace"))
	if withdrawn, _ := group.StopNameFix(alex.ID, fix.Nomination, today); withdrawn.Status != domain.NameFixWithdrawn {
		t.Fatalf("the proposer's no withdraws the fix: %+v", withdrawn)
	}
	if personName(t, group, 1) != "Ada Lovelase" {
		t.Fatal("a stopped fix changed the name")
	}
}

func TestNameFixLapsesAfterTheNameFixWindow(t *testing.T) {
	group := newGroup(t, fixing(100), open(1, misspelled, alex))
	fix := propose(t, group, alex, proposal(1, "Ada Lovelace"))
	agree(t, group, blair, fix.Nomination)
	if lapsed, err := group.ExpireNameFixes(today.Add(24*time.Hour - time.Second)); err != nil || len(lapsed) != 0 {
		t.Fatalf("a fix lapsed inside its window: %+v, %v", lapsed, err)
	}
	lapsed, err := group.ExpireNameFixes(today.Add(24 * time.Hour))
	if err != nil || len(lapsed) != 1 || lapsed[0].Status != domain.NameFixLapsed {
		t.Fatalf("a fix outlived its window: %+v, %v", lapsed, err)
	}
	_, err = group.AgreeToNameFix(casey.ID, fix.Nomination, today.Add(25*time.Hour))
	refusal(t, err, domain.NoPendingNameFix)
	if personName(t, group, 1) != "Ada Lovelase" {
		t.Fatal("a lapsed fix changed the name")
	}
	propose(t, group, casey, proposal(1, "Ada Lovelace"))
}

func TestNameFixedOnlyWhenGroupAllows(t *testing.T) {
	settings := fixing(100)
	settings.AllowNameFixes = false
	group := newGroup(t, settings, open(1, misspelled, alex))
	_, err := group.ProposeNameFix(alex.ID, proposal(1, "Ada Lovelace"), today)
	refusal(t, err, domain.NameFixedOnlyWhenGroupAllows)
}

func TestNameFixAgreementPercentageWithinRange(t *testing.T) {
	for _, percentage := range []int{0, -5, 101} {
		var out domain.AgreementPercentageOutOfRange
		if _, err := loaded(fixing(percentage)); !errors.As(err, &out) || out.Value != percentage {
			t.Fatalf("percentage %d: %v", percentage, err)
		}
	}
	for _, percentage := range []int{1, 100} {
		if _, err := loaded(fixing(percentage)); err != nil {
			t.Fatalf("percentage %d: %v", percentage, err)
		}
	}
}

func TestNameFixWindowAtLeastOneHour(t *testing.T) {
	settings := fixing(100)
	settings.NameFixWindow = time.Hour - time.Second
	var below domain.NameFixWindowBelowAnHour
	if _, err := loaded(settings); !errors.As(err, &below) {
		t.Fatalf("a window under an hour: %v", err)
	}
	settings.NameFixWindow = time.Hour
	if _, err := loaded(settings); err != nil {
		t.Fatalf("a one-hour window: %v", err)
	}
}

func TestNameFixWithPhotosSearchesOnceUnderTheNewName(t *testing.T) {
	group := newGroup(t, fixing(100), open(1, misspelled, alex))
	withPhotos := proposal(1, "Ada Lovelace")
	withPhotos.WithPhotos = true
	fix := propose(t, group, alex, withPhotos)
	if nomination, _ := group.Nomination(1); nomination.Photos != domain.NoPhotos {
		t.Fatal("photos were requested before the fix took effect")
	}
	agree(t, group, blair, fix.Nomination)
	agree(t, group, casey, fix.Nomination)
	claimed, err := group.ClaimPhotoSearches()
	if err != nil || len(claimed) != 1 || named(t, group, claimed[0].Person) != "Ada Lovelace" {
		t.Fatalf("photo search after the fix: %+v, %v", claimed, err)
	}

	searched := open(1, misspelled, alex)
	searched.Photos = domain.PhotosFound
	group = newGroup(t, fixing(100), searched)
	_, err = group.ProposeNameFix(alex.ID, withPhotos, today)
	refusal(t, err, domain.PhotosSearchedAtMostOncePerNomination)
}

func TestNameFixRefusesWhatChangesNothing(t *testing.T) {
	group := newGroup(t, fixing(100), open(1, misspelled, alex))
	_, err := group.ProposeNameFix(alex.ID, proposal(1, "Ada Lovelase"), today)
	refusal(t, err, domain.NameUnchanged)
	_, err = group.ProposeNameFix(alex.ID, proposal(9, "Ada Lovelace"), today)
	refusal(t, err, domain.UnknownNomination)
	_, err = group.AgreeToNameFix(alex.ID, 9, today)
	refusal(t, err, domain.UnknownNomination)
	_, err = group.AgreeToNameFix(alex.ID, 1, today)
	refusal(t, err, domain.NoPendingNameFix)
}

func TestNameFixWithdrawnByItsProposer(t *testing.T) {
	group := newGroup(t, fixing(50), open(1, misspelled, alex))
	fix := propose(t, group, blair, proposal(1, "Ada Lovelace"))
	if withdrawn, err := group.StopNameFix(blair.ID, fix.Nomination, today); err != nil || withdrawn.Status != domain.NameFixWithdrawn {
		t.Fatalf("the proposer's no withdraws the fix even while agreement is in reach: %+v, %v", withdrawn, err)
	}
	_, err := group.AgreeToNameFix(casey.ID, fix.Nomination, today)
	refusal(t, err, domain.NoPendingNameFix)

	fix = propose(t, group, casey, proposal(1, "Ada Lovelace"))
	if stillPending, _ := group.StopNameFix(alex.ID, fix.Nomination, today); !stillPending.Pending() {
		t.Fatalf("another Member's no is a no, not a withdrawal: %+v", stillPending)
	}
}

func TestAFixIsAnsweredThroughAnyNominationOfItsPerson(t *testing.T) {
	group := newGroup(t, fixing(100), finalized(1, misspelled, domain.Rejected), open(2, misspelled, blair), open(3, grace, casey))
	fix := propose(t, group, alex, proposal(2, "Ada Lovelace"))
	agree(t, group, blair, 1)
	if fixed := agree(t, group, casey, 2); fixed.Number != fix.Number || fixed.Status != domain.NameFixed {
		t.Fatalf("answers through Nominations 1 and 2 reached fix %d as %+v", fix.Number, fixed)
	}
	_, err := group.StopNameFix(alex.ID, 3, today)
	refusal(t, err, domain.NoPendingNameFix)
}
