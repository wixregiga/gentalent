package domain

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	PhotoCount   = 3
	StripFile    = "strip.jpg"
	SourcesFile  = "SOURCES.txt"
	ManifestFile = "manifest.json"

	rawFolder    = "raw"
	slugLimit    = 64
	fallbackSlug = "nominee"
)

type Subject struct {
	Name  string
	Pitch string
	Dir   string
}

func (s Subject) Raw() string {
	return filepath.Join(s.Dir, rawFolder)
}

func (s Subject) PhotoPath(number int) string {
	return filepath.Join(s.Dir, fmt.Sprintf("%02d_%s.jpg", number, slug(s.Name, fallbackSlug)))
}

func (s Subject) StripPath() string {
	return filepath.Join(s.Dir, StripFile)
}

func (s Subject) SourcesPath() string {
	return filepath.Join(s.Dir, SourcesFile)
}

func (s Subject) ManifestPath() string {
	return filepath.Join(s.Dir, ManifestFile)
}

func slug(name, fallback string) string {
	var b strings.Builder
	lastWasSeparator := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastWasSeparator = false
		} else if !lastWasSeparator {
			b.WriteByte('_')
			lastWasSeparator = true
		}
	}
	trimmed := strings.Trim(b.String(), "_")
	if len(trimmed) > slugLimit {
		trimmed = trimmed[:slugLimit]
	}
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
