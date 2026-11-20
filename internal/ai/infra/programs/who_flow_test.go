package programs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gentalent/internal/ai/app/command"
	"gentalent/internal/ai/domain"
	"gentalent/internal/ai/port"
)

func printWho(wrapper, answer string) string {
	return "cat <<'PAYLOAD'\n" + strings.Replace(wrapper, "%s", answer, 1) + "\nPAYLOAD"
}

func paintPhoto(t *testing.T, name string) string {
	t.Helper()
	return newTestMagick(t).convert + ` -size 1200x900 xc:navy "` + name + `"`
}

func describe(t *testing.T) domain.Description {
	t.Helper()
	return domain.Description{Text: "the woman who wrote the first computer program", Dir: filepath.Join(t.TempDir(), "who", "3")}
}

func TestIdentifyFallsBackFromAFailingCodexToClaudeWithARealPhoto(t *testing.T) {
	setSecretKeys(t)
	installFake(t, "codex", `echo "codex rate limit" >&2; exit 1`)
	claude := installFake(t, "claude", paintPhoto(t, "photo.jpg")+"\n"+printWho(`{"type":"result","structured_output":%s}`, whoJSON))
	agy := installFake(t, "agy", `exit 9`)
	identifier, err := command.NewIdentifier(
		[]domain.Provider{{CLI: "codex", Model: "m"}, {CLI: "claude", Model: "sonnet"}, {CLI: "agy", Model: "g"}},
		WhoAgents(), newTestMagick(t))
	if err != nil {
		t.Fatal(err)
	}
	description := describe(t)

	got, err := identifier.Identify(context.Background(), description)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Identification{Found: true, Name: "Ada Lovelace", Blurb: "English mathematician who wrote the first published program.",
		Source: "https://en.wikipedia.org/wiki/Ada_Lovelace", Photo: filepath.Join(description.Dir, "ada_lovelace.jpg"), Provider: "claude:sonnet"}
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if width, height := imageSize(t, got.Photo); width != 960 || height != 720 {
		t.Errorf("photo is %dx%d, want it optimized to 960x720", width, height)
	}
	if _, err := os.Stat(filepath.Join(agy.dir, "args")); err == nil {
		t.Error("agy ran although claude succeeded")
	}
	requireEnvironmentHasNoKeys(t, claude)
}

func TestIdentifyKeepsTheFigureWhenItsPhotoIsNotAnImage(t *testing.T) {
	installFake(t, "claude", `echo "<html>blocked</html>" > photo.jpg`+"\n"+printWho("%s", whoJSON))
	identifier, err := command.NewIdentifier([]domain.Provider{{CLI: "claude", Model: "sonnet"}}, WhoAgents(), newTestMagick(t))
	if err != nil {
		t.Fatal(err)
	}
	description := describe(t)

	got, err := identifier.Identify(context.Background(), description)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found || got.Name != "Ada Lovelace" || got.Photo != "" {
		t.Fatalf("got %+v", got)
	}
	if _, err := os.Stat(filepath.Join(description.Dir, "ada_lovelace.jpg")); !os.IsNotExist(err) {
		t.Errorf("an unusable photo was optimized: %v", err)
	}
}

func TestIdentifyNeverUsesAPhotoAFailedProviderLeftBehind(t *testing.T) {
	installFake(t, "codex", paintPhoto(t, "photo.jpg")+"\n"+`echo "codex crashed" >&2; exit 1`)
	installFake(t, "claude", printWho("%s", whoJSON))
	identifier, err := command.NewIdentifier([]domain.Provider{{CLI: "codex", Model: "m"}, {CLI: "claude", Model: "sonnet"}}, WhoAgents(), newTestMagick(t))
	if err != nil {
		t.Fatal(err)
	}

	got, err := identifier.Identify(context.Background(), describe(t))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found || got.Provider != "claude:sonnet" || got.Photo != "" {
		t.Fatalf("claude saved no photo, so none may be used, got %+v", got)
	}
}

func TestIdentifyRefusesMalformedAndEscapingAnswersAndAcceptsANoMatch(t *testing.T) {
	installFake(t, "codex", `echo "Probably Ada Lovelace, I think."`)
	installFake(t, "claude", printWho("%s", strings.Replace(whoJSON, `"photo.jpg"`, `"../../secret.jpg"`, 1)))
	installFake(t, "agy", printWho("%s", noMatchJSON))
	identifier, err := command.NewIdentifier(
		[]domain.Provider{{CLI: "codex", Model: "m"}, {CLI: "claude", Model: "sonnet"}, {CLI: "agy", Model: "g"}},
		WhoAgents(), newTestMagick(t))
	if err != nil {
		t.Fatal(err)
	}

	got, err := identifier.Identify(context.Background(), describe(t))
	if err != nil {
		t.Fatal(err)
	}
	if got != (domain.Identification{Provider: "agy:g"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestIdentifyGivesUpWithEveryReasonWhenNoProviderAnswers(t *testing.T) {
	installFake(t, "claude", `echo "claude is down" >&2; exit 1`)
	installFake(t, "agy", `echo "not json"`)
	identifier, err := command.NewIdentifier([]domain.Provider{{CLI: "claude", Model: "sonnet"}, {CLI: "agy", Model: "g"}}, WhoAgents(), newTestMagick(t))
	if err != nil {
		t.Fatal(err)
	}
	_, err = identifier.Identify(context.Background(), describe(t))
	if err == nil || !strings.Contains(err.Error(), "claude (sonnet): claude exited 1: claude is down | agy (g): ") {
		t.Fatalf("got %v", err)
	}
}

func TestIdentifyWritesOnlyInsideTheSearchFolder(t *testing.T) {
	installFake(t, "agy", paintPhoto(t, "photo.png")+"\n"+printWho("%s", strings.Replace(whoJSON, "photo.jpg", "photo.png", 1)))
	parent := t.TempDir()
	description := domain.Description{Text: "the first programmer", Dir: filepath.Join(parent, "3")}
	identifier, err := command.NewIdentifier([]domain.Provider{{CLI: "agy", Model: "g"}}, map[string]port.WhoAgent{"agy": Agy{}}, newTestMagick(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := identifier.Identify(context.Background(), description)
	if err != nil || got.Photo != filepath.Join(description.Dir, "ada_lovelace.jpg") {
		t.Fatalf("got %+v, %v", got, err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 || entries[0].Name() != "3" {
		t.Fatalf("parent folder holds %v, %v", entries, err)
	}
}

func TestEveryProviderIdentifiesWithTheWhoSchemaNotThePhotoSchema(t *testing.T) {
	setSecretKeys(t)
	claude := installFake(t, "claude", printWho(`{"type":"result","structured_output":%s}`, whoJSON))
	task := taskIn(t)
	if _, err := (Claude{}).Identify(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	want := []string{"-p", "--model", "the-model", "--effort", "low", "--no-session-persistence", "--disable-slash-commands",
		"--permission-mode", "bypassPermissions", "--add-dir", task.Raw, "--output-format", "json", "--json-schema", whoSchema, "the prompt"}
	if got := claude.args(t); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("claude arguments %q", got)
	}
	requireEnvironmentHasNoKeys(t, claude)

	agy := installFake(t, "agy", printWho("%s", whoJSON))
	task = taskIn(t)
	if _, err := (Agy{}).Identify(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	schema := filepath.Join(task.Folder, "who.schema.json")
	want = []string{"-p", "--model", "the-model", "--effort", "low", "--dangerously-skip-permissions", "--disable-slash-commands",
		"--add-dir", task.Raw, "--output-format", "json", "--json-schema", schema, "the prompt"}
	if got := agy.args(t); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("agy arguments %q", got)
	}
	if written, err := os.ReadFile(schema); err != nil || string(written) != whoSchema {
		t.Fatalf("agy schema file %q, %v", written, err)
	}
	requireEnvironmentHasNoKeys(t, agy)

	codex := installFake(t, "codex", strings.Replace(codexWritesOutputFile, manifestJSON, whoJSON, 1))
	task = taskIn(t)
	answer, err := Codex{}.Identify(context.Background(), task)
	if err != nil || answer.Name != "Ada Lovelace" {
		t.Fatalf("codex answer %+v, %v", answer, err)
	}
	schema = filepath.Join(task.Folder, "who.schema.json")
	want = []string{"exec", "-m", "the-model", "-c", `model_reasoning_effort="low"`, "--ephemeral", "--skip-git-repo-check",
		"--sandbox", "workspace-write", "-c", "sandbox_workspace_write.network_access=true", "-C", task.Raw,
		"--output-schema", schema, "-o", filepath.Join(task.Folder, "agent_output.json"), "-"}
	if got := codex.args(t); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("codex arguments %q", got)
	}
	if got := codex.record(t, "stdin"); got != "the prompt" {
		t.Fatalf("codex stdin %q", got)
	}
	if written, err := os.ReadFile(schema); err != nil || string(written) != whoSchema {
		t.Fatalf("codex schema file %q, %v", written, err)
	}
	requireEnvironmentHasNoKeys(t, codex)
}

func TestEveryProviderIdentifiesFromAnEmptyRawFolder(t *testing.T) {
	for name, agent := range WhoAgents() {
		installFake(t, name, printWho("%s", noMatchJSON))
		task := taskIn(t)
		if err := os.MkdirAll(task.Raw, 0o755); err != nil {
			t.Fatal(err)
		}
		stale := filepath.Join(task.Raw, "photo.jpg")
		if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := agent.Identify(context.Background(), task); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Errorf("%s left an earlier provider's photo behind", name)
		}
	}
}

func TestACancelledWhoSearchStopsTheProviderAtOnce(t *testing.T) {
	fake := installFake(t, "codex", `sleep 60`)
	identifier, err := command.NewIdentifier([]domain.Provider{{CLI: "codex", Model: "m"}, {CLI: "claude", Model: "sonnet"}}, WhoAgents(), newTestMagick(t))
	if err != nil {
		t.Fatal(err)
	}
	claude := installFake(t, "claude", printWho("%s", whoJSON))
	description := describe(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := identifier.Identify(ctx, description)
		done <- err
	}()
	for {
		if _, err := os.Stat(filepath.Join(fake.dir, "cwd")); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	started := time.Now()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("the cancelled provider kept running")
	}
	if _, err := os.Stat(filepath.Join(claude.dir, "args")); err == nil {
		t.Error("a cancelled who search moved on to the next provider")
	}
}

func TestAWhoSearchProviderPastItsTimeoutIsKilledAndReported(t *testing.T) {
	installFake(t, "claude", `sleep 60`)
	started := time.Now()
	_, err := runClaude(context.Background(), taskIn(t), job{schema: whoSchema, timeout: 200 * time.Millisecond})
	if err == nil || err.Error() != "claude timed out after 200ms" {
		t.Fatalf("got %v", err)
	}
	if time.Since(started) > 10*time.Second {
		t.Fatal("the provider was not killed")
	}
}
