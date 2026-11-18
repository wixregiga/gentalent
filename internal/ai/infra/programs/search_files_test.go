package programs

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"gentalent/internal/ai/domain"
)

func TestSearchFilesRecordTheFinishedSearch(t *testing.T) {
	subject := domain.Subject{Name: "Aster Example", Dir: t.TempDir()}
	manifest, err := domain.NewManifest("Aster Example", "Representative photos.", []domain.PhotoSource{
		{File: "01.jpg", PageURL: "https://example.org/p/1", ImageURL: "https://example.org/i/1.jpg", Caption: "Shot 1"},
		{File: "02.png", PageURL: "https://example.org/p/2", ImageURL: "https://example.org/i/2.png", Caption: "Café <stage> & lights"},
		{File: "03.webp", PageURL: "https://example.org/p/3", ImageURL: "https://example.org/i/3.webp", Caption: "Shot 3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := domain.Result{
		Photos:   []string{subject.PhotoPath(1), subject.PhotoPath(2), subject.PhotoPath(3)},
		Strip:    subject.StripPath(),
		Sources:  subject.SourcesPath(),
		Provider: "claude:sonnet",
		Summary:  manifest.Summary,
	}
	if err := (SearchFiles{}).Record(subject, manifest, result); err != nil {
		t.Fatal(err)
	}

	sources, err := os.ReadFile(subject.SourcesPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Aster Example: photos\nProvider: claude:sonnet\nSummary: Representative photos.\n\n",
		"02_aster_example.jpg\nCaption: Café <stage> & lights\nSource page: https://example.org/p/2\nImage URL: https://example.org/i/2.png\n",
	} {
		if !strings.Contains(string(sources), want) {
			t.Errorf("B234 SOURCES.txt lacks %q:\n%s", want, sources)
		}
	}

	written, err := os.ReadFile(subject.ManifestPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "Café <stage> & lights") {
		t.Errorf("B234 manifest.json keeps text unescaped, as the Python bot writes it:\n%s", written)
	}
	var got any
	if err := json.Unmarshal(written, &got); err != nil {
		t.Fatal(err)
	}
	photo := func(file, page, image, caption string) map[string]any {
		return map[string]any{"file": file, "source_page_url": page, "image_url": image, "caption": caption}
	}
	want := map[string]any{
		"canonical_name": "Aster Example",
		"summary":        "Representative photos.",
		"provider_used":  "claude:sonnet",
		"strip_path":     subject.StripPath(),
		"photos": []any{
			photo("01_aster_example.jpg", "https://example.org/p/1", "https://example.org/i/1.jpg", "Shot 1"),
			photo("02_aster_example.jpg", "https://example.org/p/2", "https://example.org/i/2.png", "Café <stage> & lights"),
			photo("03_aster_example.jpg", "https://example.org/p/3", "https://example.org/i/3.webp", "Shot 3"),
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("B234 manifest.json\ngot  %v\nwant %v", got, want)
	}
}
