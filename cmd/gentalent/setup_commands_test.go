package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCopiesTheExampleOnceAndNeverOverwrites(t *testing.T) {
	example, err := os.ReadFile(filepath.Join("..", "..", exampleConfig))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, exampleConfig), example, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	path := filepath.Join(root, "setup", "config.json")

	code, out, errs := gentalent(t, "--config", path, "init")
	if code != 0 || out != "Created "+path+" - fill in the confirmed group and roster IDs.\n" {
		t.Fatalf("B191 init: %d %q %q", code, out, errs)
	}
	written, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(written, example) {
		t.Fatalf("B191 init writes the example verbatim: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("B190 the config is private: %v %v", info, err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errs = gentalent(t, "--config", path, "init")
	if kept, _ := os.ReadFile(path); code != 1 || !strings.Contains(errs, "config already exists; no overwrite") || string(kept) != "{}" {
		t.Fatalf("B191 init never overwrites: %d %q %q", code, errs, kept)
	}
}

func TestSetSecretSavesThePipedTokenPrivately(t *testing.T) {
	dir := t.TempDir()
	config := writeConfig(t, dir, nil)
	run := func(input string) (int, string, string) {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		if _, err := writer.WriteString(input); err != nil {
			t.Fatal(err)
		}
		writer.Close()
		var out, errs bytes.Buffer
		code := execute(t.Context(), []string{"--config", config, "set-secret"}, invocation{in: reader, out: &out, errs: &errs})
		return code, out.String(), errs.String()
	}

	code, out, errs := run("  beeper-token-value \n")
	if code != 0 || out != "Token saved privately.\n" || !strings.HasPrefix(errs, "Beeper access token (hidden; do not paste into chat): ") {
		t.Fatalf("B192 set-secret: %d %q %q", code, out, errs)
	}
	token := filepath.Join(dir, "secret", "beeper.token")
	saved, err := os.ReadFile(token)
	if err != nil || string(saved) != "beeper-token-value" {
		t.Fatalf("B192 the stripped token is saved: %q %v", saved, err)
	}
	if info, err := os.Stat(token); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("B215 the token file is private: %v %v", info, err)
	}
	if code, _, errs := run("\n"); code != 1 || !strings.Contains(errs, "token was empty") {
		t.Fatalf("B192 an empty token is refused: %d %q", code, errs)
	}
	if saved, _ := os.ReadFile(token); string(saved) != "beeper-token-value" {
		t.Fatal("an empty token replaced the saved one")
	}
}
