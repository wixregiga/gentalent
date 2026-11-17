package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeProgram struct {
	dir string
}

func installFake(t *testing.T, name, body string) fakeProgram {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %[1]s/args
env > %[1]s/env
pwd > %[1]s/cwd
cat > %[1]s/stdin
%[2]s
`, dir, body)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return fakeProgram{dir: dir}
}

func (f fakeProgram) record(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (f fakeProgram) args(t *testing.T) []string {
	return strings.Split(strings.TrimSuffix(f.record(t, "args"), "\n"), "\n")
}

const manifestJSON = `{"canonical_name":"Aster Example","summary":"Representative photos.","photos":[` +
	`{"raw_filename":"01.jpg","source_page_url":"https://example.org/p/1","image_url":"https://example.org/i/1.jpg","caption":"Shot 1"},` +
	`{"raw_filename":"02.jpg","source_page_url":"https://example.org/p/2","image_url":"https://example.org/i/2.jpg","caption":"Shot 2"},` +
	`{"raw_filename":"03.jpg","source_page_url":"https://example.org/p/3","image_url":"https://example.org/i/3.jpg","caption":"Shot 3"}]}`

func printManifest(wrapper string) string {
	return fmt.Sprintf("cat <<'PAYLOAD'\n"+wrapper+"\nPAYLOAD", manifestJSON)
}
