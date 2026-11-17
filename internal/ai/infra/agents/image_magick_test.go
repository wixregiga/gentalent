package agents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func newTestMagick(t *testing.T) ImageMagick {
	t.Helper()
	magick, err := NewImageMagick()
	if err != nil {
		t.Fatalf("ImageMagick is required: %v", err)
	}
	return magick
}

func makeImage(t *testing.T, path, color string) {
	t.Helper()
	magick := newTestMagick(t)
	if out, err := exec.Command(magick.convert, "-size", "1200x900", "xc:"+color, path).CombinedOutput(); err != nil {
		t.Fatalf("convert: %v: %s", err, out)
	}
}

func imageSize(t *testing.T, path string) (int, int) {
	t.Helper()
	out, err := exec.Command(newTestMagick(t).identify, "-format", "%w %h", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var size [2]int
	for i, field := range strings.Fields(string(out)) {
		size[i], _ = strconv.Atoi(field)
	}
	return size[0], size[1]
}

func TestCheckAcceptsARealImageInTheRawFolder(t *testing.T) {
	raw := t.TempDir()
	makeImage(t, filepath.Join(raw, "01.jpg"), "navy")
	if err := newTestMagick(t).Check(context.Background(), filepath.Join(raw, "01.jpg")); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRefusesWhatIsNotAnImageInsideTheRawFolder(t *testing.T) {
	raw, outside := t.TempDir(), t.TempDir()
	makeImage(t, filepath.Join(outside, "secret.jpg"), "gold")
	write := func(name, content string) string {
		path := filepath.Join(raw, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	link := filepath.Join(raw, "link.jpg")
	if err := os.Symlink(filepath.Join(outside, "secret.jpg"), link); err != nil {
		t.Fatal(err)
	}
	subfolder := filepath.Join(raw, "dir.jpg")
	if err := os.Mkdir(subfolder, 0o755); err != nil {
		t.Fatal(err)
	}
	refused := map[string]string{
		"missing":         filepath.Join(raw, "absent.jpg"),
		"empty":           write("empty.jpg", ""),
		"text":            write("text.jpg", "<html>not an image</html>"),
		"symlink outside": link,
		"folder":          subfolder,
	}
	magick := newTestMagick(t)
	for label, path := range refused {
		if err := magick.Check(context.Background(), path); err == nil {
			t.Errorf("%s accepted", label)
		}
	}
}

func TestOptimizeShrinksLargePhotosAndKeepsSmallOnes(t *testing.T) {
	dir := t.TempDir()
	large, small := filepath.Join(dir, "large.jpg"), filepath.Join(dir, "small.jpg")
	makeImage(t, large, "teal")
	if out, err := exec.Command(newTestMagick(t).convert, "-size", "300x200", "xc:red", small).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	magick := newTestMagick(t)
	for source, wantWidth := range map[string]int{large: 960, small: 300} {
		destination := filepath.Join(dir, "out_"+filepath.Base(source))
		if err := magick.Optimize(context.Background(), source, destination); err != nil {
			t.Fatal(err)
		}
		if width, _ := imageSize(t, destination); width != wantWidth {
			t.Errorf("%s: width %d, want %d", filepath.Base(source), width, wantWidth)
		}
	}
}

func TestStitchJoinsThreePhotosIntoOneBorderedStrip(t *testing.T) {
	dir := t.TempDir()
	var photos []string
	for i, color := range []string{"navy", "crimson", "gold"} {
		photo := filepath.Join(dir, "0"+strconv.Itoa(i+1)+".jpg")
		makeImage(t, photo, color)
		photos = append(photos, photo)
	}
	strip := filepath.Join(dir, "strip.jpg")
	if err := newTestMagick(t).Stitch(context.Background(), photos, strip); err != nil {
		t.Fatal(err)
	}
	width, height := imageSize(t, strip)
	if width != 3*(640+12) || height != 492 {
		t.Fatalf("strip is %dx%d, want %dx492", width, height, 3*(640+12))
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".strip-") {
			t.Errorf("scratch folder %s left behind", entry.Name())
		}
	}
}

func TestStitchFailsWhenAPhotoCannotBeRead(t *testing.T) {
	dir := t.TempDir()
	err := newTestMagick(t).Stitch(context.Background(), []string{filepath.Join(dir, "gone.jpg")}, filepath.Join(dir, "strip.jpg"))
	if err == nil {
		t.Fatal("a missing photo produced a strip")
	}
}

func TestToolsComeFromPathThenFromTheFallbackFolder(t *testing.T) {
	onPath, fallback := t.TempDir(), t.TempDir()
	for dir, name := range map[string]string{onPath: "from-path", fallback: "from-fallback"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", onPath)
	if got, err := resolveTool("from-path", fallback); err != nil || got != filepath.Join(onPath, "from-path") {
		t.Errorf("path tool: %q, %v", got, err)
	}
	if got, err := resolveTool("from-fallback", fallback); err != nil || got != filepath.Join(fallback, "from-fallback") {
		t.Errorf("fallback tool: %q, %v", got, err)
	}
	if _, err := resolveTool("nowhere", fallback); err == nil || !strings.Contains(err.Error(), "nowhere") {
		t.Errorf("missing tool: %v", err)
	}
}
