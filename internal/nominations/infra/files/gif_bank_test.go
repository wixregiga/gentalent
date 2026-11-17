package files_test

import (
	"os"
	"path/filepath"
	"testing"

	"gentalent/internal/nominations/infra/files"
)

func bank(t *testing.T, names ...string) (files.GifBank, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "shame-gifs")
	if err := os.MkdirAll(filepath.Join(dir, "nested.gif"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("GIF89a"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return files.NewGifBank(dir), dir
}

func TestTheLeastUsedGifIsPicked(t *testing.T) {
	gifs, dir := bank(t, "bell.gif", "gong.gif", "notes.txt")
	got, err := gifs.Pick(map[string]int{"bell.gif": 2, "gong.gif": 1})
	if err != nil || got != filepath.Join(dir, "gong.gif") {
		t.Fatalf("B107 the least used GIF, got %q, %v", got, err)
	}
}

func TestTiesArePickedAtRandom(t *testing.T) {
	gifs, dir := bank(t, "bell.gif", "gong.gif", "horn.gif")
	seen := map[string]bool{}
	for range 200 {
		got, err := gifs.Pick(map[string]int{"horn.gif": 1})
		if err != nil {
			t.Fatal(err)
		}
		seen[got] = true
	}
	if len(seen) != 2 || !seen[filepath.Join(dir, "bell.gif")] || !seen[filepath.Join(dir, "gong.gif")] {
		t.Fatalf("B107 every least used GIF is picked sometimes and no other, got %v", seen)
	}
}

func TestAnEmptyOrMissingBankPicksNothing(t *testing.T) {
	empty, _ := bank(t, "notes.txt")
	missing := files.NewGifBank(filepath.Join(t.TempDir(), "absent"))
	for _, gifs := range []files.GifBank{empty, missing} {
		if got, err := gifs.Pick(nil); err != nil || got != "" {
			t.Fatalf("B107 no GIF, got %q, %v", got, err)
		}
	}
}
