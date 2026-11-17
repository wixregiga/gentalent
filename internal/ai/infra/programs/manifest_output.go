package programs

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"gentalent/internal/ai/domain"
)

const manifestSchema = `{"type":"object","properties":{"canonical_name":{"type":"string"},"summary":{"type":"string"},` +
	`"photos":{"type":"array","description":"Exactly 3 verified representative photos saved in raw_dir.",` +
	`"items":{"type":"object","properties":{"raw_filename":{"type":"string"},"source_page_url":{"type":"string"},` +
	`"image_url":{"type":"string"},"caption":{"type":"string"}},` +
	`"required":["raw_filename","source_page_url","image_url","caption"],"additionalProperties":false}}},` +
	`"required":["canonical_name","summary","photos"],"additionalProperties":false}`

var fencedObject = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*\\})\\s*```")

type manifestPayload struct {
	CanonicalName string `json:"canonical_name"`
	Summary       string `json:"summary"`
	Photos        []struct {
		File     string `json:"raw_filename"`
		PageURL  string `json:"source_page_url"`
		ImageURL string `json:"image_url"`
		Caption  string `json:"caption"`
	} `json:"photos"`
}

func decodeManifest(output string) (domain.Manifest, error) {
	payload, err := parseJSON(output)
	if err != nil {
		return domain.Manifest{}, err
	}
	object, err := manifestObject(payload)
	if err != nil {
		return domain.Manifest{}, err
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return domain.Manifest{}, err
	}
	var decoded manifestPayload
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return domain.Manifest{}, err
	}
	photos := make([]domain.PhotoSource, len(decoded.Photos))
	for i, photo := range decoded.Photos {
		photos[i] = domain.PhotoSource{File: photo.File, PageURL: photo.PageURL, ImageURL: photo.ImageURL, Caption: photo.Caption}
	}
	return domain.NewManifest(decoded.CanonicalName, decoded.Summary, photos)
}

func parseJSON(text string) (any, error) {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return nil, errors.New("empty agent output")
	}
	if value, err := unmarshal(raw); err == nil {
		return value, nil
	}
	if fenced := fencedObject.FindStringSubmatch(raw); fenced != nil {
		return unmarshal(fenced[1])
	}
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start == -1 || end <= start {
		return nil, errors.New("could not parse a JSON object from the agent output")
	}
	return unmarshal(raw[start : end+1])
}

func unmarshal(text string) (any, error) {
	var value any
	err := json.Unmarshal([]byte(text), &value)
	return value, err
}

func manifestObject(payload any) (map[string]any, error) {
	if text, ok := payload.(string); ok {
		return objectFromText(text)
	}
	object, ok := payload.(map[string]any)
	if !ok {
		return nil, errors.New("CLI response is not a JSON object")
	}
	if inner, ok := object["structured_output"].(map[string]any); ok {
		return inner, nil
	}
	_, hasPhotos := object["photos"]
	_, hasName := object["canonical_name"]
	if hasPhotos && hasName {
		return object, nil
	}
	switch result := object["result"].(type) {
	case map[string]any:
		return result, nil
	case string:
		return objectFromText(result)
	}
	return object, nil
}

func objectFromText(text string) (map[string]any, error) {
	parsed, err := parseJSON(text)
	if err != nil {
		return nil, err
	}
	object, ok := parsed.(map[string]any)
	if !ok {
		return nil, errors.New("agent output is not a JSON object")
	}
	return object, nil
}
