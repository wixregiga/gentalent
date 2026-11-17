package domain

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPhotoPathsNameThePersonAndStayInTheFolder(t *testing.T) {
	cases := map[string]string{
		"Aster Example":           "01_aster_example.jpg",
		"  --Mx. D'Arcy, Jr.!  ":  "01_mx_d_arcy_jr.jpg",
		"":                        "01_nominee.jpg",
		"日本語":                     "01_nominee.jpg",
		strings.Repeat("ab ", 40): "01_" + strings.Repeat("ab_", 21) + "a.jpg",
	}
	for name, want := range cases {
		dir := t.TempDir()
		got := Subject{Name: name, Dir: dir}.PhotoPath(1)
		if got != filepath.Join(dir, want) {
			t.Errorf("name %q: got %s, want %s", name, got, filepath.Join(dir, want))
		}
	}
}

func TestPhotoPathsAreNumberedPerPhoto(t *testing.T) {
	s := Subject{Name: "Aster", Dir: "/x"}
	if got := s.PhotoPath(3); got != "/x/03_aster.jpg" {
		t.Fatalf("got %s", got)
	}
}

func TestProviderLabelOmitsAMissingModel(t *testing.T) {
	if got := (Provider{CLI: "claude", Model: "sonnet"}).Label(); got != "claude:sonnet" {
		t.Fatalf("got %s", got)
	}
	if got := (Provider{CLI: "agy"}).Label(); got != "agy" {
		t.Fatalf("got %s", got)
	}
}
