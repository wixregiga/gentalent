package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeClaude = `#!/bin/sh
for n in 1 2 3; do convert -size 64x48 xc:navy "0$n.jpg"; done
cat <<'MANIFEST'
{"canonical_name":"Grace Brewster Hopper","summary":"A fictional pioneer.","photos":[
{"raw_filename":"01.jpg","source_page_url":"https://example.org/p/1","image_url":"https://example.org/i/1.jpg","caption":"Shot 1"},
{"raw_filename":"02.jpg","source_page_url":"https://example.org/p/2","image_url":"https://example.org/i/2.jpg","caption":"Shot 2"},
{"raw_filename":"03.jpg","source_page_url":"https://example.org/p/3","image_url":"https://example.org/i/3.jpg","caption":"Shot 3"}]}
MANIFEST
`

func TestGenerateMediaRunsOneSearchWithTheChosenProvider(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(fakeClaude), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	config := writeConfig(t, dir, func(values map[string]any) {
		values["ai_providers"] = []map[string]string{{"cli": "codex", "model": "test"}, {"cli": "claude", "model": "sonnet"}}
	})

	code, out, errs := gentalent(t, "--config", config, "generate-media", "Grace Hopper", "--pitch", "Wrote the first compiler", "--cli", "claude")
	if code != 0 {
		t.Fatalf("B203 generate-media: %d %q", code, errs)
	}
	var shown mediaJSON
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	folder := filepath.Dir(shown.StripPath)
	if filepath.Dir(folder) != filepath.Join(dir, ".state", "media", "generate-media") {
		t.Fatalf("B203 the search runs in its own folder under media_dir: %s", folder)
	}
	photo := func(n int) string { return filepath.Join(folder, fmt.Sprintf("0%d_grace_hopper.jpg", n)) }
	want, err := json.Marshal(map[string]any{
		"nominee_name": "Grace Brewster Hopper", "summary": "A fictional pioneer.", "provider_used": "claude:sonnet",
		"strip_path": filepath.Join(folder, "strip.jpg"), "photo_paths": []string{photo(1), photo(2), photo(3)},
		"sources_path": filepath.Join(folder, "SOURCES.txt"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, out, string(want))
	for _, path := range append(shown.PhotoPaths, shown.StripPath, shown.SourcesPath) {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("B203 %s was not written: %v", path, err)
		}
	}

	code, _, errs = gentalent(t, "--config", config, "generate-media", "Grace Hopper", "--cli", "agy")
	if code != 1 || !strings.Contains(errs, "agy is not listed in ai_providers in config.json") {
		t.Fatalf("B219 --cli names a configured provider: %d %q", code, errs)
	}
}
