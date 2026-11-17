package programs

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gentalent/internal/ai/port"
)

func taskIn(t *testing.T) port.Task {
	t.Helper()
	folder := t.TempDir()
	return port.Task{Model: "the-model", Prompt: "the prompt", Folder: folder, Raw: filepath.Join(folder, "raw")}
}

func requireEnvironmentHasNoKeys(t *testing.T, fake fakeProgram) {
	t.Helper()
	environment := fake.record(t, "env")
	for _, name := range strippedEnvironment {
		if strings.Contains(environment, name+"=") {
			t.Errorf("%s reached the provider", name)
		}
	}
	if !strings.Contains(environment, "PATH=") {
		t.Error("the rest of the environment was dropped")
	}
}

func setSecretKeys(t *testing.T) {
	for _, name := range strippedEnvironment {
		t.Setenv(name, "secret-paid-key")
	}
}

func TestClaudeRunsSandboxFreeInTheRawFolderWithSubscriptionLoginOnly(t *testing.T) {
	setSecretKeys(t)
	fake := installFake(t, "claude", printManifest(`{"type":"result","structured_output":%s}`))
	task := taskIn(t)

	manifest, err := Claude{}.Run(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.CanonicalName != "Aster Example" || len(manifest.Photos) != 3 {
		t.Fatalf("manifest %+v", manifest)
	}
	want := []string{"-p", "--model", "the-model", "--effort", "low", "--no-session-persistence", "--disable-slash-commands",
		"--permission-mode", "bypassPermissions", "--add-dir", task.Raw, "--output-format", "json", "--json-schema", manifestSchema, "the prompt"}
	if got := fake.args(t); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("arguments %q", got)
	}
	if got := strings.TrimSpace(fake.record(t, "cwd")); got != task.Raw {
		t.Fatalf("ran in %s", got)
	}
	requireEnvironmentHasNoKeys(t, fake)
}

func TestAgyUsesTheSchemaFileAndPlainOutput(t *testing.T) {
	setSecretKeys(t)
	fake := installFake(t, "agy", printManifest("%s"))
	task := taskIn(t)

	if _, err := (Agy{}).Run(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	schema := filepath.Join(task.Folder, "manifest.schema.json")
	want := []string{"-p", "--model", "the-model", "--effort", "low", "--dangerously-skip-permissions", "--disable-slash-commands",
		"--add-dir", task.Raw, "--output-format", "json", "--json-schema", schema, "the prompt"}
	if got := fake.args(t); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("arguments %q", got)
	}
	if written, err := os.ReadFile(schema); err != nil || string(written) != manifestSchema {
		t.Fatalf("schema file %q, %v", written, err)
	}
	requireEnvironmentHasNoKeys(t, fake)
}

const codexWritesOutputFile = `while [ $# -gt 0 ]; do if [ "$1" = -o ]; then out="$2"; fi; shift; done
cat > "$out" <<'PAYLOAD'
` + manifestJSON + `
PAYLOAD
echo "progress noise on stdout"`

func TestCodexSandboxesToTheRawFolderAndPrefersItsOutputFile(t *testing.T) {
	setSecretKeys(t)
	fake := installFake(t, "codex", codexWritesOutputFile)
	task := taskIn(t)

	manifest, err := Codex{}.Run(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.CanonicalName != "Aster Example" {
		t.Fatalf("manifest %+v", manifest)
	}
	schema := filepath.Join(task.Folder, "manifest.schema.json")
	want := []string{"exec", "-m", "the-model", "-c", `model_reasoning_effort="low"`, "--ephemeral", "--skip-git-repo-check",
		"--sandbox", "workspace-write", "-c", "sandbox_workspace_write.network_access=true", "-C", task.Raw,
		"--output-schema", schema, "-o", filepath.Join(task.Folder, "agent_output.json"), "-"}
	if got := fake.args(t); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("arguments %q", got)
	}
	if got := fake.record(t, "stdin"); got != "the prompt" {
		t.Fatalf("stdin %q", got)
	}
	if got := strings.TrimSpace(fake.record(t, "cwd")); got != task.Raw {
		t.Fatalf("ran in %s", got)
	}
	requireEnvironmentHasNoKeys(t, fake)
}

func TestCodexReadsStdoutWhenItWritesNoOutputFile(t *testing.T) {
	installFake(t, "codex", printManifest("%s"))
	if _, err := (Codex{}).Run(context.Background(), taskIn(t)); err != nil {
		t.Fatal(err)
	}
}

func TestCodexIgnoresAStaleOutputFileFromAnEarlierProvider(t *testing.T) {
	installFake(t, "codex", `echo "no json here"`)
	task := taskIn(t)
	if err := os.MkdirAll(task.Folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(task.Folder, "agent_output.json"), []byte(manifestJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (Codex{}).Run(context.Background(), task); err == nil {
		t.Fatal("a stale output file was used")
	}
}

func TestEveryProviderStartsFromAnEmptyRawFolder(t *testing.T) {
	for name, agent := range map[string]port.Agent{"codex": Codex{}, "claude": Claude{}, "agy": Agy{}} {
		installFake(t, name, printManifest("%s"))
		task := taskIn(t)
		if err := os.MkdirAll(filepath.Join(task.Raw, "keep"), 0o755); err != nil {
			t.Fatal(err)
		}
		stale := filepath.Join(task.Raw, "01.jpg")
		if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := agent.Run(context.Background(), task); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Errorf("%s left the earlier provider's file behind", name)
		}
		if _, err := os.Stat(filepath.Join(task.Raw, "keep")); err != nil {
			t.Errorf("%s removed a folder: %v", name, err)
		}
	}
}

func TestAProviderThatExitsNonZeroReportsItsFirstFourHundredCharacters(t *testing.T) {
	installFake(t, "claude", `echo "rate limit hit" >&2; head -c 600 /dev/zero | tr '\0' x >&2; exit 3`)
	_, err := Claude{}.Run(context.Background(), taskIn(t))
	if err == nil || !strings.HasPrefix(err.Error(), "claude exited 3: rate limit hit") {
		t.Fatalf("got %v", err)
	}
	if len(err.Error()) > len("claude exited 3: ")+400 {
		t.Fatalf("error text not cut: %d characters", len(err.Error()))
	}
}

func TestAProviderThatPrintsOnlyToStdoutBeforeFailingStillExplainsItself(t *testing.T) {
	installFake(t, "agy", `echo "login expired"; exit 1`)
	_, err := Agy{}.Run(context.Background(), taskIn(t))
	if err == nil || !strings.Contains(err.Error(), "agy exited 1: login expired") {
		t.Fatalf("got %v", err)
	}
}

func TestAMissingProgramIsReportedByName(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := Claude{}.Run(context.Background(), taskIn(t))
	if err == nil || err.Error() != "claude CLI is not installed or on PATH" {
		t.Fatalf("got %v", err)
	}
}

func TestACancelledSearchStopsTheProviderAtOnce(t *testing.T) {
	fake := installFake(t, "claude", `sleep 60`)
	task := taskIn(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := Claude{}.Run(ctx, task)
		done <- err
	}()
	for {
		if _, err := os.Stat(filepath.Join(fake.dir, "cwd")); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestAProviderPastItsTimeoutIsKilledAndReported(t *testing.T) {
	installFake(t, "sleeper", `sleep 60`)
	exe, err := exec.LookPath("sleeper")
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = runProgram(context.Background(), call{exe: exe, timeout: 200 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "sleeper timed out") {
		t.Fatalf("got %v", err)
	}
	if time.Since(started) > 10*time.Second {
		t.Fatal("the program was not killed")
	}
}
