package domain

import (
	"strings"
	"testing"
)

func photo(file string) PhotoSource {
	return PhotoSource{File: file, PageURL: "https://example.org/page", ImageURL: "https://example.org/img.jpg", Caption: "A caption"}
}

func threePhotos() []PhotoSource {
	return []PhotoSource{photo("01.jpg"), photo("02.jpg"), photo("03.jpg")}
}

func TestNewManifestRequiresExactlyThreePhotos(t *testing.T) {
	for _, count := range []int{0, 2, 4} {
		photos := make([]PhotoSource, count)
		for i := range photos {
			photos[i] = photo("0.jpg")
		}
		if _, err := NewManifest("Name", "Summary", photos); err == nil {
			t.Errorf("%d photos accepted", count)
		}
	}
	if _, err := NewManifest("Name", "Summary", threePhotos()); err != nil {
		t.Fatalf("three photos refused: %v", err)
	}
}

func TestNewManifestRefusesFilenamesThatLeaveTheRawFolder(t *testing.T) {
	for _, file := range []string{"../x.jpg", "a/b.jpg", `a\b.jpg`, "..", ".", "/etc/passwd"} {
		photos := threePhotos()
		photos[1].File = file
		if _, err := NewManifest("Name", "Summary", photos); err == nil {
			t.Errorf("filename %q accepted", file)
		}
	}
}

func TestNewManifestEnforcesFieldLimitsAfterTrimming(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	cases := map[string]func(*[]PhotoSource, *string, *string){
		"blank filename": func(p *[]PhotoSource, _, _ *string) { (*p)[0].File = "   " },
		"long filename":  func(p *[]PhotoSource, _, _ *string) { (*p)[0].File = long(121) },
		"blank page url": func(p *[]PhotoSource, _, _ *string) { (*p)[1].PageURL = "" },
		"long page url":  func(p *[]PhotoSource, _, _ *string) { (*p)[1].PageURL = long(2001) },
		"long image url": func(p *[]PhotoSource, _, _ *string) { (*p)[2].ImageURL = long(2001) },
		"long caption":   func(p *[]PhotoSource, _, _ *string) { (*p)[2].Caption = long(301) },
		"blank caption":  func(p *[]PhotoSource, _, _ *string) { (*p)[2].Caption = " \n" },
		"long name":      func(_ *[]PhotoSource, name, _ *string) { *name = long(121) },
		"blank name":     func(_ *[]PhotoSource, name, _ *string) { *name = "" },
		"long summary":   func(_ *[]PhotoSource, _, summary *string) { *summary = long(1001) },
		"blank summary":  func(_ *[]PhotoSource, _, summary *string) { *summary = "  " },
	}
	for label, mutate := range cases {
		photos, name, summary := threePhotos(), "Name", "Summary"
		mutate(&photos, &name, &summary)
		if _, err := NewManifest(name, summary, photos); err == nil {
			t.Errorf("%s accepted", label)
		}
	}
	photos := threePhotos()
	photos[0].Caption = long(300)
	manifest, err := NewManifest(long(120), long(1000), photos)
	if err != nil {
		t.Fatalf("values at the limit refused: %v", err)
	}
	if len(manifest.Photos[0].Caption) != 300 {
		t.Fatal("caption at the limit was changed")
	}
}

func TestNewManifestCountsCharactersNotBytes(t *testing.T) {
	name := strings.Repeat("é", 120)
	if _, err := NewManifest(name, "Summary", threePhotos()); err != nil {
		t.Fatalf("120 characters refused: %v", err)
	}
}

func TestNewManifestTrimsValues(t *testing.T) {
	photos := threePhotos()
	photos[0].File = "  01.jpg \n"
	manifest, err := NewManifest("  Name ", " Summary\t", photos)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.CanonicalName != "Name" || manifest.Summary != "Summary" || manifest.Photos[0].File != "01.jpg" {
		t.Fatalf("values not trimmed: %+v", manifest)
	}
}

func TestSourcesListsEachPhotoWithItsProvenance(t *testing.T) {
	manifest, err := NewManifest("Aster Example", "Representative photos.", threePhotos())
	if err != nil {
		t.Fatal(err)
	}
	got := manifest.Sources("claude:sonnet", []string{"01_a.jpg", "02_a.jpg", "03_a.jpg"})
	for _, want := range []string{
		"Aster Example: photos\nProvider: claude:sonnet\nSummary: Representative photos.\n",
		"02_a.jpg\nCaption: A caption\nSource page: https://example.org/page\nImage URL: https://example.org/img.jpg\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("sources lack %q:\n%s", want, got)
		}
	}
}
