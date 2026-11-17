package domain_test

import (
	"testing"

	"gentalent/internal/nominations/domain"
)

func withPhotos(person domain.Person) []domain.Candidate {
	return []domain.Candidate{{Person: person.ID, Name: person.Name, Pitch: "A pitch for " + person.Name, WithPhotos: true}}
}

func TestPhotosSearchedAtMostOncePerNomination(t *testing.T) {
	group := newGroup(t, oneAtATime, open(1, ada, alex))

	outcome, err := group.Nominate(blair.ID, withPhotos(ada), today)
	if err != nil || len(outcome) != 1 || !outcome[0].PhotosOnly || outcome[0].Number != 1 {
		t.Fatalf("B40 !!gt on the open Nomination asks for its photos, got %+v, %v", outcome, err)
	}
	if got := nomination(t, group, 1); got.Photos != domain.PhotosRequested || got.Pitch != "A pitch for Ada Lovelace" {
		t.Fatalf("B40 photos are requested with the pitch, got %+v", got)
	}
	before := snapshot(group)
	_, err = group.Nominate(blair.ID, withPhotos(ada), today)
	refused := refusal(t, err, domain.PhotosSearchedAtMostOncePerNomination)
	wantNumbers(t, refused.Numbers, 1)
	wantUnchanged(t, group, before)

	claimed, err := group.ClaimPhotoSearches()
	if err != nil || len(claimed) != 1 {
		t.Fatalf("the requested search is claimed once, got %+v, %v", claimed, err)
	}
	if again, err := group.ClaimPhotoSearches(); err != nil || len(again) != 0 {
		t.Fatalf("a claimed search is never claimed again, got %+v, %v", again, err)
	}
	if _, err := group.FailPhotoSearch(1); err != nil {
		t.Fatal(err)
	}
	_, err = group.Nominate(blair.ID, withPhotos(ada), today)
	refusal(t, err, domain.PhotosSearchedAtMostOncePerNomination)
}

func TestPhotosForALockedNominationAreRefused(t *testing.T) {
	group := newGroup(t, oneAtATime, locked(1, ada, alex, domain.Yes, today))
	_, err := group.Nominate(blair.ID, withPhotos(ada), today)
	refused := refusal(t, err, domain.OneOpenNominationPerPerson)
	wantNumbers(t, refused.Numbers, 1)
}

func TestPhotoSearchLifecycle(t *testing.T) {
	group := newGroup(t, oneAtATime)
	if _, err := group.Nominate(alex.ID, withPhotos(ada), today); err != nil {
		t.Fatal(err)
	}
	if _, err := group.AttachPhotos(1, "/media/1/strip.jpg", "AI media"); err == nil {
		t.Fatal("photos attach only to a claimed search")
	}
	if _, err := group.ClaimPhotoSearches(); err != nil {
		t.Fatal(err)
	}
	found, err := group.AttachPhotos(1, "/media/1/strip.jpg", "AI media (codex)")
	if err != nil || found.Photos != domain.PhotosFound || found.Strip != "/media/1/strip.jpg" {
		t.Fatalf("B50 a found search records its strip, got %+v, %v", found, err)
	}
	if len(found.Exhibits) != 2 || found.Exhibits[1] != "AI media (codex)" {
		t.Fatalf("B47 B50 the pitch and the AI note are exhibits, got %v", found.Exhibits)
	}
}

func TestAbandonInterruptedSearches(t *testing.T) {
	group := newGroup(t, oneAtATime)
	if _, err := group.Nominate(alex.ID, withPhotos(ada), today); err != nil {
		t.Fatal(err)
	}
	if _, err := group.ClaimPhotoSearches(); err != nil {
		t.Fatal(err)
	}
	abandoned, err := group.AbandonInterruptedSearches()
	if err != nil || len(abandoned) != 1 || abandoned[0].Photos != domain.PhotosFailed {
		t.Fatalf("D7 a started search is recorded failed, got %+v, %v", abandoned, err)
	}
	if again, _ := group.ClaimPhotoSearches(); len(again) != 0 {
		t.Fatalf("D7 an interrupted search is never rerun, got %+v", again)
	}
}

func TestExhibitsKeepAtMost2000Characters(t *testing.T) {
	group := newGroup(t, oneAtATime)
	long := make([]rune, 2100)
	for i := range long {
		long[i] = 'é'
	}
	if _, err := group.Nominate(alex.ID, []domain.Candidate{{Person: ada.ID, Name: ada.Name, Pitch: string(long)}}, today); err != nil {
		t.Fatal(err)
	}
	if got := []rune(nomination(t, group, 1).Exhibits[0]); len(got) != 2000 {
		t.Fatalf("B47 a pitch is stored cut to 2000 characters, got %d", len(got))
	}
}
