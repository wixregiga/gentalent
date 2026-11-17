package command

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"gentalent/internal/ai/domain"
	"gentalent/internal/ai/port"
)

type fakeAgent struct {
	manifest domain.Manifest
	err      error
	tasks    []port.Task
	onRun    func()
}

func (a *fakeAgent) Run(_ context.Context, task port.Task) (domain.Manifest, error) {
	a.tasks = append(a.tasks, task)
	if a.onRun != nil {
		a.onRun()
	}
	return a.manifest, a.err
}

type fakeImages struct {
	checked    []string
	optimized  [][2]string
	stitched   []string
	stripPaths []string
	failCheck  string
}

func (i *fakeImages) Check(_ context.Context, path string) error {
	i.checked = append(i.checked, path)
	if filepath.Base(path) == i.failCheck {
		return errors.New("not a valid image")
	}
	return nil
}

func (i *fakeImages) Optimize(_ context.Context, source, destination string) error {
	i.optimized = append(i.optimized, [2]string{source, destination})
	return nil
}

func (i *fakeImages) Stitch(_ context.Context, photos []string, destination string) error {
	i.stitched = append(i.stitched, photos...)
	i.stripPaths = append(i.stripPaths, destination)
	return nil
}

type fakeSources struct {
	path string
	text string
}

func (s *fakeSources) WriteSources(path, text string) error {
	s.path, s.text = path, text
	return nil
}

func manifestOf(files ...string) domain.Manifest {
	photos := make([]domain.PhotoSource, len(files))
	for i, file := range files {
		photos[i] = domain.PhotoSource{File: file, PageURL: "https://example.org/p", ImageURL: "https://example.org/i", Caption: "c"}
	}
	return domain.Manifest{CanonicalName: "Aster Example", Summary: "s", Photos: photos}
}

var threeFiles = manifestOf("01.jpg", "02.jpg", "03.jpg")

var codex, claude, agy = domain.Provider{CLI: "codex", Model: "m1"}, domain.Provider{CLI: "claude", Model: "sonnet"}, domain.Provider{CLI: "agy"}

func newFinder(t *testing.T, providers []domain.Provider, agents map[string]port.Agent, images *fakeImages, sources *fakeSources) *Finder {
	t.Helper()
	finder, err := NewFinder(providers, agents, images, sources)
	if err != nil {
		t.Fatal(err)
	}
	return finder
}

func TestFindFallsBackThroughProvidersInOrderAndNamesTheWinner(t *testing.T) {
	failing := &fakeAgent{err: errors.New("codex rate limit")}
	working := &fakeAgent{manifest: threeFiles}
	never := &fakeAgent{manifest: threeFiles}
	images, sources := &fakeImages{}, &fakeSources{}
	finder := newFinder(t, []domain.Provider{codex, claude, agy},
		map[string]port.Agent{"codex": failing, "claude": working, "agy": never}, images, sources)
	dir := t.TempDir()

	result, err := finder.Find(context.Background(), domain.Subject{Name: "Aster Example", Pitch: "pitch", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "claude:sonnet" || len(never.tasks) != 0 {
		t.Fatalf("provider %q, later provider ran %d times", result.Provider, len(never.tasks))
	}
	wantPhotos := []string{filepath.Join(dir, "01_aster_example.jpg"), filepath.Join(dir, "02_aster_example.jpg"), filepath.Join(dir, "03_aster_example.jpg")}
	if strings.Join(result.Photos, ",") != strings.Join(wantPhotos, ",") || result.Strip != filepath.Join(dir, "strip.jpg") {
		t.Fatalf("result %+v", result)
	}
	if strings.Join(images.stitched, ",") != strings.Join(wantPhotos, ",") || images.stripPaths[0] != result.Strip {
		t.Fatalf("strip built from %v into %v", images.stitched, images.stripPaths)
	}
	if sources.path != filepath.Join(dir, "SOURCES.txt") || !strings.Contains(sources.text, "Provider: claude:sonnet") {
		t.Fatalf("sources %q: %s", sources.path, sources.text)
	}
}

func TestFindGivesEachProviderTheSubjectFolderAndAPromptNamingIt(t *testing.T) {
	agent := &fakeAgent{manifest: threeFiles}
	finder := newFinder(t, []domain.Provider{claude}, map[string]port.Agent{"claude": agent}, &fakeImages{}, &fakeSources{})
	dir := t.TempDir()
	if _, err := finder.Find(context.Background(), domain.Subject{Name: "Aster Example", Pitch: "aka Ace", Dir: dir}); err != nil {
		t.Fatal(err)
	}
	task := agent.tasks[0]
	if task.Model != "sonnet" || task.Folder != dir || task.Raw != filepath.Join(dir, "raw") {
		t.Fatalf("task %+v", task)
	}
	for _, want := range []string{`"name":"Aster Example"`, `"message":"aka Ace"`, `"target_raw_dir":"` + task.Raw + `"`, `"expected_files":["01.jpg","02.jpg","03.jpg"]`} {
		if !strings.Contains(task.Prompt, want) {
			t.Errorf("prompt lacks %s: %s", want, task.Prompt)
		}
	}
}

func TestFindFallsBackWhenADownloadedPhotoIsNotAValidImage(t *testing.T) {
	bad := &fakeAgent{manifest: manifestOf("01.jpg", "bogus.jpg", "03.jpg")}
	good := &fakeAgent{manifest: threeFiles}
	images := &fakeImages{failCheck: "bogus.jpg"}
	finder := newFinder(t, []domain.Provider{codex, claude}, map[string]port.Agent{"codex": bad, "claude": good}, images, &fakeSources{})

	result, err := finder.Find(context.Background(), domain.Subject{Name: "Aster", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "claude:sonnet" || len(images.optimized) != 3 {
		t.Fatalf("provider %q, optimized %d photos", result.Provider, len(images.optimized))
	}
}

func TestFindReportsEveryProviderFailureWhenAllFail(t *testing.T) {
	agents := map[string]port.Agent{
		"codex":  &fakeAgent{err: errors.New("codex exited 1: rate limit")},
		"claude": &fakeAgent{err: errors.New("claude exited 1: down")},
	}
	finder := newFinder(t, []domain.Provider{codex, claude}, agents, &fakeImages{}, &fakeSources{})

	_, err := finder.Find(context.Background(), domain.Subject{Name: "Aster", Dir: t.TempDir()})
	want := "all configured subscription AI providers failed: codex (m1): codex exited 1: rate limit | claude (sonnet): claude exited 1: down"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}

func TestFindStopsWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	first := &fakeAgent{err: errors.New("killed"), onRun: cancel}
	second := &fakeAgent{manifest: threeFiles}
	finder := newFinder(t, []domain.Provider{codex, claude}, map[string]port.Agent{"codex": first, "claude": second}, &fakeImages{}, &fakeSources{})

	_, err := finder.Find(ctx, domain.Subject{Name: "Aster", Dir: t.TempDir()})
	if !errors.Is(err, context.Canceled) || len(second.tasks) != 0 {
		t.Fatalf("err %v, second provider ran %d times", err, len(second.tasks))
	}
}

func TestNewFinderRefusesProvidersItCannotRun(t *testing.T) {
	agents := map[string]port.Agent{"claude": &fakeAgent{}}
	if _, err := NewFinder(nil, agents, &fakeImages{}, &fakeSources{}); err == nil {
		t.Error("no providers accepted")
	}
	_, err := NewFinder([]domain.Provider{claude, {CLI: "gemini"}}, agents, &fakeImages{}, &fakeSources{})
	if err == nil || !strings.Contains(err.Error(), "gemini") {
		t.Errorf("unknown CLI: %v", err)
	}
}
