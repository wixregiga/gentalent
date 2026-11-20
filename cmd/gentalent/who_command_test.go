package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeWhoClaude = `#!/bin/sh
convert -size 64x48 xc:navy photo.jpg
cat <<'ANSWER'
{"found":true,"name":"Ada Lovelace","blurb":"She wrote the first published computer program.","source_url":"https://example.org/ada","photo_file":"photo.jpg"}
ANSWER
`

func TestWhoRunsOneSearchThroughTheRealProgramAndImageTool(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(fakeWhoClaude), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	config := writeConfig(t, dir, func(values map[string]any) {
		values["ai_providers"] = []map[string]string{{"cli": "codex", "model": "test"}, {"cli": "claude", "model": "sonnet"}}
	})

	code, out, errs := gentalent(t, "--config", config, "who", "the woman who wrote the first computer program", "--cli", "claude")
	if code != 0 {
		t.Fatalf("who: %d %q", code, errs)
	}
	var shown whoJSON
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !shown.Found || shown.Name != "Ada Lovelace" || shown.Source != "https://example.org/ada" || shown.ProviderUsed != "claude:sonnet" {
		t.Fatalf("the answer names one public figure with its source: %+v", shown)
	}
	if folder := filepath.Dir(shown.PhotoPath); filepath.Dir(folder) != filepath.Join(dir, ".state", "media", "who-search") {
		t.Fatalf("the search runs in its own folder under media_dir: %s", shown.PhotoPath)
	}
	if info, err := os.Stat(shown.PhotoPath); err != nil || info.Size() == 0 {
		t.Fatalf("the one photo was optimized into the folder: %v", err)
	}

	code, _, errs = gentalent(t, "--config", config, "who", "the woman who wrote the first computer program", "--cli", "agy")
	if code != 1 || !strings.Contains(errs, "agy is not listed in ai_providers in config.json") {
		t.Fatalf("--cli names a configured provider: %d %q", code, errs)
	}
}
