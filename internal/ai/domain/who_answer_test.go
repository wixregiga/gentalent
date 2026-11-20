package domain

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAnAnswerThatFindsNoOneCarriesNothingElse(t *testing.T) {
	answer, err := NewWhoAnswer(false, "Ada Lovelace", "A guess.", "https://example.org/ada", "../photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if answer != (WhoAnswer{}) {
		t.Fatalf("a not-found answer kept the agent's guess: %+v", answer)
	}
}

func TestAFoundAnswerNeedsANameAndABlurbWithinTheirLimits(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	cases := map[string][2]string{
		"blank name":          {" \n", "A blurb."},
		"long name":           {long(121), "A blurb."},
		"blank blurb":         {"Ada Lovelace", "\t"},
		"long blurb":          {"Ada Lovelace", long(601)},
		"name with a control": {"Ada\x00Lovelace", "A blurb."},
		"blurb with escape":   {"Ada Lovelace", "A \x1b[31mred\x1b[0m blurb."},
	}
	for label, fields := range cases {
		if _, err := NewWhoAnswer(true, fields[0], fields[1], "", ""); err == nil {
			t.Errorf("%s accepted", label)
		}
	}
	answer, err := NewWhoAnswer(true, strings.Repeat("é", 120), long(600), "", "")
	if err != nil {
		t.Fatalf("values at the limit refused: %v", err)
	}
	if !answer.Found || answer.PhotoFile != "" || answer.Source != "" {
		t.Fatalf("answer %+v", answer)
	}
}

func TestAFoundAnswerKeepsItsNameAndBlurbOnOneLine(t *testing.T) {
	answer, err := NewWhoAnswer(true, "  Ada\n  Lovelace ", "English mathematician.\n\nWrote the first\tprogram.", "", " photo.jpg ")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Name != "Ada Lovelace" || answer.Blurb != "English mathematician. Wrote the first program." || answer.PhotoFile != "photo.jpg" {
		t.Fatalf("answer %+v", answer)
	}
}

func TestAFoundAnswerRefusesAPhotoOutsideTheRawFolder(t *testing.T) {
	for _, file := range []string{"../photo.jpg", "a/photo.jpg", `a\photo.jpg`, "..", ".", "/etc/passwd", strings.Repeat("p", 121)} {
		if _, err := NewWhoAnswer(true, "Ada Lovelace", "A blurb.", "", file); err == nil {
			t.Errorf("photo file %q accepted", file)
		}
	}
}

func TestAFoundAnswerKeepsOnlyAWebSourceLink(t *testing.T) {
	kept := []string{"https://en.wikipedia.org/wiki/Ada_Lovelace", "http://example.org/a?b=c#d"}
	for _, link := range kept {
		answer, err := NewWhoAnswer(true, "Ada Lovelace", "A blurb.", " "+link+"\n", "")
		if err != nil || answer.Source != link {
			t.Errorf("source %q: %+v, %v", link, answer, err)
		}
	}
	dropped := []string{"", "Wikipedia", "javascript:alert(1)", "file:///etc/passwd", "/wiki/Ada", "https://", "https://exa mple.org",
		"https://example.org/" + strings.Repeat("a", 2000)}
	for _, link := range dropped {
		answer, err := NewWhoAnswer(true, "Ada Lovelace", "A blurb.", link, "")
		if err != nil {
			t.Errorf("source %q failed the answer: %v", link, err)
		}
		if answer.Source != "" {
			t.Errorf("source %q kept", link)
		}
	}
}

func TestADescriptionsPhotoIsNamedAfterTheFigureInsideItsFolder(t *testing.T) {
	dir := t.TempDir()
	description := Description{Text: "the first programmer", Dir: dir}
	if got := description.PhotoPath("Ada Lovelace"); got != filepath.Join(dir, "ada_lovelace.jpg") {
		t.Fatalf("got %s", got)
	}
	if got := description.PhotoPath("../.."); got != filepath.Join(dir, "public_figure.jpg") {
		t.Fatalf("got %s", got)
	}
	if got := description.Raw(); got != filepath.Join(dir, "raw") {
		t.Fatalf("got %s", got)
	}
}
