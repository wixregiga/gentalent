package programs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gentalent/internal/ai/app/command"
	"gentalent/internal/ai/domain"
	"gentalent/internal/ai/port"
)

func paintRawPhotos(t *testing.T) string {
	t.Helper()
	convert := newTestMagick(t).convert
	return `for n in 1 2 3; do ` + convert + ` -size 1200x900 xc:navy "0$n.jpg"; done`
}

func TestFindFallsBackFromAFailingCodexToClaudeWithRealImages(t *testing.T) {
	setSecretKeys(t)
	installFake(t, "codex", `echo "codex rate limit" >&2; exit 1`)
	installFake(t, "claude", paintRawPhotos(t)+"\n"+printManifest(`{"structured_output":%s}`))
	agy := installFake(t, "agy", `exit 9`)
	finder, err := command.NewFinder(
		[]domain.Provider{{CLI: "codex", Model: "m"}, {CLI: "claude", Model: "sonnet"}, {CLI: "agy", Model: "g"}},
		Programs(), newTestMagick(t), SearchFiles{})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "7-aster_example")

	result, err := finder.Find(context.Background(), domain.Subject{Name: "Aster Example", Pitch: "aka Ace", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "claude:sonnet" || len(result.Photos) != 3 {
		t.Fatalf("result %+v", result)
	}
	if _, err := os.Stat(filepath.Join(agy.dir, "args")); err == nil {
		t.Error("agy ran although claude succeeded")
	}
	if want := filepath.Join(dir, "01_aster_example.jpg"); result.Photos[0] != want {
		t.Errorf("first photo %s, want %s", result.Photos[0], want)
	}
	for _, photo := range append(result.Photos, result.Strip, result.Sources, filepath.Join(dir, "manifest.json")) {
		if info, err := os.Stat(photo); err != nil || info.Size() == 0 {
			t.Errorf("%s missing or empty: %v", photo, err)
		}
	}
	if _, height := imageSize(t, result.Strip); height != 492 {
		t.Errorf("strip height %d", height)
	}
	sources, _ := os.ReadFile(result.Sources)
	if !strings.Contains(string(sources), "Provider: claude:sonnet\n") || !strings.Contains(string(sources), "01_aster_example.jpg\nCaption: Shot 1") {
		t.Errorf("sources:\n%s", sources)
	}
}

func TestFindWritesOnlyInsideTheSubjectFolder(t *testing.T) {
	installFake(t, "agy", paintRawPhotos(t)+"\n"+printManifest("%s"))
	parent := t.TempDir()
	dir := filepath.Join(parent, "1-aster")
	finder, err := command.NewFinder([]domain.Provider{{CLI: "agy", Model: "g"}}, map[string]port.Agent{"agy": Agy{}}, newTestMagick(t), SearchFiles{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finder.Find(context.Background(), domain.Subject{Name: "Aster", Dir: dir}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 || entries[0].Name() != "1-aster" {
		t.Fatalf("parent folder holds %v, %v", entries, err)
	}
}

func TestFindGivesUpWithEveryReasonWhenNoProviderDelivers(t *testing.T) {
	installFake(t, "claude", `echo "claude is down" >&2; exit 1`)
	installFake(t, "agy", `echo "not json"`)
	finder, err := command.NewFinder(
		[]domain.Provider{{CLI: "claude", Model: "sonnet"}, {CLI: "agy", Model: "g"}}, Programs(), newTestMagick(t), SearchFiles{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = finder.Find(context.Background(), domain.Subject{Name: "Aster", Dir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "claude (sonnet): claude exited 1: claude is down | agy (g): ") {
		t.Fatalf("got %v", err)
	}
}
