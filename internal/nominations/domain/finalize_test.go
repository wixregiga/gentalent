package domain_test

import (
	"testing"
	"time"

	"gentalent/internal/nominations/domain"
)

func locked(number domain.NominationNumber, person domain.Person, nominator member, last domain.Vote, at time.Time) domain.Nomination {
	nomination := open(number, person, nominator)
	for _, member := range members {
		nomination.Votes[member.ID] = domain.Yes
	}
	nomination.Votes[casey.ID] = last
	nomination.LockedAt = at
	return nomination
}

var severalAtOnce = domain.Settings{GroupOpenNominationLimit: 3, MemberOpenNominationLimit: 3, SingleMessageNominationLimit: 3}

func TestCertifiedByUnanimousYes(t *testing.T) {
	group := newGroup(t, severalAtOnce, locked(1, ada, alex, domain.Yes, today), locked(2, grace, alex, domain.No, today))
	finalized, err := group.Finalize(alex.ID, nil, today)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if len(finalized) != 2 || finalized[0].Status != domain.Certified || finalized[1].Status != domain.Rejected {
		t.Fatalf("B84 unanimous yes certifies and any no rejects, in number order, got %+v", finalized)
	}
	if finalized[0].FinalizedAt != today || len(finalized[1].Votes) != 3 {
		t.Fatalf("B88 the outcome is dated and the votes stay, got %+v", finalized[1])
	}

	forged := finalized[1]
	forged.Status = domain.Certified
	_, err = domain.NewGroup("group-test", memberIDs, severalAtOnce, []domain.Nomination{forged}, nil)
	refused := refusal(t, err, domain.CertifiedByUnanimousYes)
	wantNumbers(t, refused.Numbers, 2)
}

func TestFinalizedOnlyWhenAllVotesAreIn(t *testing.T) {
	group := newGroup(t, severalAtOnce, open(1, ada, alex), open(2, grace, alex))
	before := snapshot(group)

	_, err := group.Finalize(alex.ID, nil, today)
	refused := refusal(t, err, domain.FinalizedOnlyWhenAllVotesAreIn)
	wantNumbers(t, refused.Numbers, 1, 2)

	_, err = group.Finalize(blair.ID, nil, today)
	refused = refusal(t, err, domain.FinalizedOnlyWhenAllVotesAreIn)
	wantNumbers(t, refused.Numbers, 1, 2)
	wantUnchanged(t, group, before)

	nothing := newGroup(t, oneAtATime, finalized(1, ada, domain.Certified))
	if got, err := nothing.Finalize(alex.ID, nil, today); err != nil || len(got) != 0 {
		t.Fatalf("B80 a bare !final with nothing open is silent, got %+v, %v", got, err)
	}
}

func TestFinalizedEarlyOnlyByNominator(t *testing.T) {
	group := newGroup(t, severalAtOnce, locked(1, ada, alex, domain.Yes, today), open(2, grace, blair))
	before := snapshot(group)

	_, err := group.Finalize(casey.ID, nil, today)
	refused := refusal(t, err, domain.FinalizedEarlyOnlyByNominator)
	wantNumbers(t, refused.Numbers, 1)

	_, err = group.Finalize(blair.ID, []domain.Ref{{Number: 1}}, today)
	refusal(t, err, domain.FinalizedEarlyOnlyByNominator)

	_, err = group.Finalize(blair.ID, nil, today)
	refused = refusal(t, err, domain.FinalizedOnlyWhenAllVotesAreIn)
	wantNumbers(t, refused.Numbers, 2)
	wantUnchanged(t, group, before)

	finalized, err := group.Finalize(alex.ID, []domain.Ref{{Person: ada.ID}}, today)
	if err != nil || len(finalized) != 1 || finalized[0].Number != 1 {
		t.Fatalf("B83 the nominator finalizes their locked Nomination by name, got %+v, %v", finalized, err)
	}
}

func TestFinalizeNamingSeveralFinalizesAllOrNone(t *testing.T) {
	group := newGroup(t, severalAtOnce, locked(1, ada, alex, domain.Yes, today), open(2, grace, alex), locked(3, katherine, alex, domain.No, today))
	before := snapshot(group)

	_, err := group.Finalize(alex.ID, []domain.Ref{{Number: 1}, {Number: 2}}, today)
	refusal(t, err, domain.FinalizedOnlyWhenAllVotesAreIn)
	wantUnchanged(t, group, before)

	finalized, err := group.Finalize(alex.ID, nil, today)
	if err != nil || len(finalized) != 2 || finalized[0].Number != 1 || finalized[1].Number != 3 {
		t.Fatalf("Q20 a bare !final finalizes every locked Nomination of the nominator, got %+v, %v", finalized, err)
	}
}

func TestFinalizedAMinuteAfterTheLastVote(t *testing.T) {
	group := newGroup(t, severalAtOnce, locked(1, ada, alex, domain.Yes, today), locked(2, grace, blair, domain.No, today.Add(30*time.Second)), open(3, katherine, casey))

	if got, err := group.FinalizeDue(today.Add(59 * time.Second)); err != nil || len(got) != 0 {
		t.Fatalf("B77 nothing is due at 59 seconds, got %+v, %v", got, err)
	}
	due, err := group.FinalizeDue(today.Add(time.Minute))
	if err != nil || len(due) != 1 || due[0].Number != 1 || due[0].Status != domain.Certified {
		t.Fatalf("B77 Q14 each Nomination keeps its own timer, got %+v, %v", due, err)
	}
	due, err = group.FinalizeDue(today.Add(90 * time.Second))
	if err != nil || len(due) != 1 || due[0].Number != 2 || due[0].Status != domain.Rejected {
		t.Fatalf("Q14 the second timer runs out on its own, got %+v, %v", due, err)
	}
	if got := nomination(t, group, 3); !got.Open() {
		t.Fatalf("an unlocked Nomination is never finalized by the timer, got %+v", got)
	}
}
