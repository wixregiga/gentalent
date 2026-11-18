package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type PhotoSource struct {
	File     string
	PageURL  string
	ImageURL string
	Caption  string
}

type Manifest struct {
	CanonicalName string
	Summary       string
	Photos        []PhotoSource
}

func NewManifest(canonicalName, summary string, photos []PhotoSource) (Manifest, error) {
	if len(photos) != PhotoCount {
		return Manifest{}, fmt.Errorf("manifest requires exactly %d photos, got %d", PhotoCount, len(photos))
	}
	checked := make([]PhotoSource, len(photos))
	for i, photo := range photos {
		file, err := boundedText("raw_filename", photo.File, 120)
		if err != nil {
			return Manifest{}, err
		}
		if strings.ContainsAny(file, `/\`) || file == "." || file == ".." {
			return Manifest{}, fmt.Errorf("raw_filename must be a relative filename inside the raw folder: %q", file)
		}
		pageURL, err := boundedText("source_page_url", photo.PageURL, 2000)
		if err != nil {
			return Manifest{}, err
		}
		imageURL, err := boundedText("image_url", photo.ImageURL, 2000)
		if err != nil {
			return Manifest{}, err
		}
		caption, err := boundedText("caption", photo.Caption, 300)
		if err != nil {
			return Manifest{}, err
		}
		checked[i] = PhotoSource{File: file, PageURL: pageURL, ImageURL: imageURL, Caption: caption}
	}
	name, err := boundedText("canonical_name", canonicalName, 120)
	if err != nil {
		return Manifest{}, err
	}
	about, err := boundedText("summary", summary, 1000)
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{CanonicalName: name, Summary: about, Photos: checked}, nil
}

func boundedText(field, value string, limit int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if n := utf8.RuneCountInString(trimmed); n < 1 || n > limit {
		return "", fmt.Errorf("%s must be 1 to %d characters", field, limit)
	}
	return trimmed, nil
}
