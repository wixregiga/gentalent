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

type fakeWhoAgent struct {
	answer domain.WhoAnswer
	err    error
	tasks  []port.Task
	onRun  func()
}

func (a *fakeWhoAgent) Identify(_ context.Context, task port.Task) (domain.WhoAnswer, error) {
	a.tasks = append(a.tasks, task)
	if a.onRun != nil {
		a.onRun()
	}
	return a.answer, a.err
}

type cancellingImages struct {
	fakeImages
	cancel func()
}

func (i *cancellingImages) Check(ctx context.Context, path string) error {
	i.cancel()
	return ctx.Err()
}

var ada = domain.WhoAnswer{Found: true, Name: "Ada Lovelace", Blurb: "English mathematician who wrote the first program.",
	Source: "https://example.org/ada", PhotoFile: "photo.jpg"}

func newIdentifier(t *testing.T, providers []domain.Provider, agents map[string]port.WhoAgent, images port.ImageTool) *Identifier {
	t.Helper()
	identifier, err := NewIdentifier(providers, agents, images)
	if err != nil {
		t.Fatal(err)
	}
	return identifier
}

func TestIdentifyFallsBackThroughProvidersAndOptimizesTheOnePhoto(t *testing.T) {
	failing := &fakeWhoAgent{err: errors.New("codex rate limit")}
	working := &fakeWhoAgent{answer: ada}
	never := &fakeWhoAgent{answer: ada}
	images := &fakeImages{}
	identifier := newIdentifier(t, []domain.Provider{codex, claude, agy},
		map[string]port.WhoAgent{"codex": failing, "claude": working, "agy": never}, images)
	dir := t.TempDir()

	got, err := identifier.Identify(context.Background(), domain.Description{Text: "the first programmer", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	photo := filepath.Join(dir, "ada_lovelace.jpg")
	want := domain.Identification{Found: true, Name: ada.Name, Blurb: ada.Blurb, Source: ada.Source, Photo: photo, Provider: "claude:sonnet"}
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if len(never.tasks) != 0 {
		t.Fatal("a later provider ran after one answered")
	}
	raw := filepath.Join(dir, "raw", "photo.jpg")
	if strings.Join(images.checked, ",") != raw || len(images.optimized) != 1 || images.optimized[0] != [2]string{raw, photo} {
		t.Fatalf("checked %v, optimized %v", images.checked, images.optimized)
	}
	if len(images.stitched) != 0 {
		t.Fatal("a who search built a strip")
	}
}

func TestIdentifyAcceptsANoMatchWithoutAskingAnotherProvider(t *testing.T) {
	first := &fakeWhoAgent{answer: domain.WhoAnswer{}}
	second := &fakeWhoAgent{answer: ada}
	images := &fakeImages{}
	identifier := newIdentifier(t, []domain.Provider{codex, claude}, map[string]port.WhoAgent{"codex": first, "claude": second}, images)

	got, err := identifier.Identify(context.Background(), domain.Description{Text: "my neighbour", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got != (domain.Identification{Provider: "codex:m1"}) || len(second.tasks) != 0 || len(images.checked) != 0 {
		t.Fatalf("got %+v, second provider ran %d times, checked %v", got, len(second.tasks), images.checked)
	}
}

func TestIdentifyKeepsAFoundFigureWhosePhotoIsNotUsable(t *testing.T) {
	first := &fakeWhoAgent{answer: ada}
	second := &fakeWhoAgent{answer: ada}
	images := &fakeImages{failCheck: "photo.jpg"}
	identifier := newIdentifier(t, []domain.Provider{codex, claude}, map[string]port.WhoAgent{"codex": first, "claude": second}, images)

	got, err := identifier.Identify(context.Background(), domain.Description{Text: "the first programmer", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found || got.Name != "Ada Lovelace" || got.Photo != "" || got.Provider != "codex:m1" {
		t.Fatalf("got %+v", got)
	}
	if len(images.optimized) != 0 || len(second.tasks) != 0 {
		t.Fatalf("optimized %v, second provider ran %d times", images.optimized, len(second.tasks))
	}
}

func TestIdentifyChecksNoPhotoWhenTheAgentSavedNone(t *testing.T) {
	noPhoto := ada
	noPhoto.PhotoFile = ""
	images := &fakeImages{}
	identifier := newIdentifier(t, []domain.Provider{claude}, map[string]port.WhoAgent{"claude": &fakeWhoAgent{answer: noPhoto}}, images)

	got, err := identifier.Identify(context.Background(), domain.Description{Text: "the first programmer", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found || got.Photo != "" || len(images.checked) != 0 {
		t.Fatalf("got %+v, checked %v", got, images.checked)
	}
}

func TestIdentifyGivesTheAgentADescriptionAndItsRawFolder(t *testing.T) {
	agent := &fakeWhoAgent{answer: ada}
	identifier := newIdentifier(t, []domain.Provider{claude}, map[string]port.WhoAgent{"claude": agent}, &fakeImages{})
	dir := t.TempDir()
	if _, err := identifier.Identify(context.Background(), domain.Description{Text: "  the woman who wrote the first program \n", Dir: dir}); err != nil {
		t.Fatal(err)
	}
	task := agent.tasks[0]
	if task.Model != "sonnet" || task.Folder != dir || task.Raw != filepath.Join(dir, "raw") {
		t.Fatalf("task %+v", task)
	}
	for _, want := range []string{`"description":"the woman who wrote the first program"`, `"raw_dir":"` + task.Raw + `"`, "Never name a private individual", "set found to false"} {
		if !strings.Contains(task.Prompt, want) {
			t.Errorf("prompt lacks %s: %s", want, task.Prompt)
		}
	}
}

func TestIdentifyReportsEveryProviderFailureWhenAllFail(t *testing.T) {
	agents := map[string]port.WhoAgent{
		"codex":  &fakeWhoAgent{err: errors.New("codex exited 1: rate limit")},
		"claude": &fakeWhoAgent{err: errors.New("claude timed out after 1m30s")},
	}
	identifier := newIdentifier(t, []domain.Provider{codex, claude}, agents, &fakeImages{})

	_, err := identifier.Identify(context.Background(), domain.Description{Text: "the first programmer", Dir: t.TempDir()})
	want := "all configured subscription AI providers failed: codex (m1): codex exited 1: rate limit | claude (sonnet): claude timed out after 1m30s"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}

func TestIdentifyStopsWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	first := &fakeWhoAgent{err: errors.New("killed"), onRun: cancel}
	second := &fakeWhoAgent{answer: ada}
	identifier := newIdentifier(t, []domain.Provider{codex, claude}, map[string]port.WhoAgent{"codex": first, "claude": second}, &fakeImages{})

	_, err := identifier.Identify(ctx, domain.Description{Text: "the first programmer", Dir: t.TempDir()})
	if !errors.Is(err, context.Canceled) || len(second.tasks) != 0 {
		t.Fatalf("err %v, second provider ran %d times", err, len(second.tasks))
	}
}

func TestIdentifyReportsAnInterruptionDuringThePhotoCheckAsNoAnswer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	images := &cancellingImages{cancel: cancel}
	identifier := newIdentifier(t, []domain.Provider{claude}, map[string]port.WhoAgent{"claude": &fakeWhoAgent{answer: ada}}, images)

	got, err := identifier.Identify(ctx, domain.Description{Text: "the first programmer", Dir: t.TempDir()})
	if !errors.Is(err, context.Canceled) || got.Found {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestIdentifyRefusesABlankDescriptionWithoutRunningAnAgent(t *testing.T) {
	agent := &fakeWhoAgent{answer: ada}
	identifier := newIdentifier(t, []domain.Provider{claude}, map[string]port.WhoAgent{"claude": agent}, &fakeImages{})
	if _, err := identifier.Identify(context.Background(), domain.Description{Text: " \n", Dir: t.TempDir()}); err == nil {
		t.Error("blank description accepted")
	}
	if _, err := identifier.Identify(context.Background(), domain.Description{Text: "the first programmer"}); err == nil {
		t.Error("description without a folder accepted")
	}
	if len(agent.tasks) != 0 {
		t.Fatalf("agent ran %d times", len(agent.tasks))
	}
}

func TestNewIdentifierRefusesProvidersItCannotRun(t *testing.T) {
	agents := map[string]port.WhoAgent{"claude": &fakeWhoAgent{}}
	if _, err := NewIdentifier(nil, agents, &fakeImages{}); err == nil {
		t.Error("no providers accepted")
	}
	_, err := NewIdentifier([]domain.Provider{claude, {CLI: "gemini"}}, agents, &fakeImages{})
	if err == nil || !strings.Contains(err.Error(), "gemini") {
		t.Errorf("unknown CLI: %v", err)
	}
	if _, err := NewIdentifier([]domain.Provider{claude}, agents, nil); err == nil {
		t.Error("no image tool accepted")
	}
}
