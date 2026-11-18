package programs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gentalent/internal/ai/domain"
)

type SearchFiles struct{}

type manifestFile struct {
	CanonicalName string      `json:"canonical_name"`
	Summary       string      `json:"summary"`
	ProviderUsed  string      `json:"provider_used"`
	StripPath     string      `json:"strip_path"`
	Photos        []photoFile `json:"photos"`
}

type photoFile struct {
	File          string `json:"file"`
	SourcePageURL string `json:"source_page_url"`
	ImageURL      string `json:"image_url"`
	Caption       string `json:"caption"`
}

func (SearchFiles) Record(subject domain.Subject, manifest domain.Manifest, result domain.Result) error {
	sources := []string{manifest.CanonicalName + ": photos", "Provider: " + result.Provider, "Summary: " + manifest.Summary, ""}
	record := manifestFile{CanonicalName: manifest.CanonicalName, Summary: manifest.Summary, ProviderUsed: result.Provider, StripPath: result.Strip}
	for i, photo := range manifest.Photos {
		saved := filepath.Base(result.Photos[i])
		sources = append(sources, saved, "Caption: "+photo.Caption, "Source page: "+photo.PageURL, "Image URL: "+photo.ImageURL, "")
		record.Photos = append(record.Photos, photoFile{File: saved, SourcePageURL: photo.PageURL, ImageURL: photo.ImageURL, Caption: photo.Caption})
	}
	if err := writeFile(subject.SourcesPath(), []byte(strings.Join(sources, "\n"))); err != nil {
		return err
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(record); err != nil {
		return fmt.Errorf("encoding %s: %w", subject.ManifestPath(), err)
	}
	return writeFile(subject.ManifestPath(), encoded.Bytes())
}

func writeFile(path string, content []byte) error {
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
