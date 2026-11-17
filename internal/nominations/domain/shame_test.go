package domain_test

import (
	"testing"

	"gentalent/internal/nominations/domain"
)

func shame(by, target member) domain.Shame {
	return domain.Shame{
		By:        by.ID,
		Target:    target.ID,
		Reason:    "voted no",
		Shamed:    domain.ShamedMessage{ID: "message-shamed", Text: "nope", SentAt: monday},
		GIF:       "bell.gif",
		MessageID: "message-shame",
		At:        today,
	}
}

func TestOneShameGivenPerNominee(t *testing.T) {
	group := newGroup(t, severalAtOnce, finalized(1, ada, domain.Rejected), open(2, grace, alex))

	given, err := group.Shame(shame(blair, casey), domain.Ref{Number: 1})
	if err != nil {
		t.Fatalf("Shame: %v", err)
	}
	if given.ID != 1 || given.Nomination != 1 || given.TargetVote != domain.No {
		t.Fatalf("B106 B108 a shame records its Nomination and the target's vote, even after the verdict, got %+v", given)
	}
	before := snapshot(group)
	_, err = group.Shame(shame(blair, alex), domain.Ref{Person: ada.ID})
	refused := refusal(t, err, domain.OneShameGivenPerNominee)
	wantNumbers(t, refused.Numbers, 1)
	wantUnchanged(t, group, before)

	other, err := group.Shame(shame(blair, casey), domain.Ref{Number: 2})
	if err != nil || other.ID != 2 || other.TargetVote != domain.NotVoted {
		t.Fatalf("B108 the same Member may shame over another Nomination, before the target votes, got %+v, %v", other, err)
	}
}

func TestRenominationAllowsANewShame(t *testing.T) {
	allow := oneAtATime
	allow.AllowRenomination = true
	group := newGroup(t, allow, finalized(1, ada, domain.Rejected))
	if _, err := group.Shame(shame(blair, casey), domain.Ref{}); err != nil {
		t.Fatal(err)
	}
	if _, err := group.Renominate(alex.ID, candidates(ada), today); err != nil {
		t.Fatal(err)
	}
	if again, err := group.Shame(shame(blair, casey), domain.Ref{Person: ada.ID}); err != nil || again.Nomination != 2 {
		t.Fatalf("D16 the renominated Person's new Nomination allows a new shame, got %+v, %v", again, err)
	}
}

func TestShameRefusals(t *testing.T) {
	cases := []struct {
		name    string
		history []domain.Nomination
		shame   domain.Shame
		about   domain.Ref
		rule    domain.Rule
	}{
		{"B97 a target outside the Group", []domain.Nomination{open(1, ada, alex)}, shame(blair, member{ID: "stranger"}), domain.Ref{}, domain.ShameOnlyMembers},
		{"B98 shaming yourself", []domain.Nomination{open(1, ada, alex)}, shame(blair, blair), domain.Ref{}, domain.NoSelfShame},
		{"B101 nothing nominated yet", nil, shame(blair, casey), domain.Ref{}, domain.NothingToShame},
		{"Q15 several open and none named", []domain.Nomination{open(1, ada, alex), open(2, grace, blair)}, shame(blair, casey), domain.Ref{}, domain.ShameNamesItsNomination},
		{"Q17 an unknown number", []domain.Nomination{open(1, ada, alex)}, shame(blair, casey), domain.Ref{Number: 9}, domain.UnknownNomination},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			group := newGroup(t, severalAtOnce, c.history...)
			_, err := group.Shame(c.shame, c.about)
			refusal(t, err, c.rule)
			if len(group.Shames()) != 0 {
				t.Fatal("a refused shame must record nothing")
			}
		})
	}
}

func TestUnnamedShameIsAboutTheLatestNomination(t *testing.T) {
	group := newGroup(t, severalAtOnce, open(1, ada, alex), finalized(2, grace, domain.Certified))
	given, err := group.Shame(shame(blair, casey), domain.Ref{})
	if err != nil || given.Nomination != 2 {
		t.Fatalf("B100 with at most one open the shame is about the latest Nomination, got %+v, %v", given, err)
	}
	if uses := group.GifUses(); uses["bell.gif"] != 1 {
		t.Fatalf("B107 the Group counts each GIF it used, got %v", uses)
	}
}
