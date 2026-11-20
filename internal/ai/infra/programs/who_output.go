package programs

import (
	"encoding/json"
	"errors"

	"gentalent/internal/ai/domain"
)

const whoSchema = `{"type":"object","properties":{` +
	`"found":{"type":"boolean","description":"true only when one clear, widely known public figure fits the description."},` +
	`"name":{"type":"string","description":"Their usual full name; empty when not found."},` +
	`"blurb":{"type":"string","description":"One or two sentences about their life, under 300 characters; empty when not found."},` +
	`"source_url":{"type":"string","description":"The page the answer relies on; empty when none."},` +
	`"photo_file":{"type":"string","description":"Filename of the one photo saved in raw_dir; empty when none."}},` +
	`"required":["found","name","blurb","source_url","photo_file"],"additionalProperties":false}`

type whoPayload struct {
	Found     *bool  `json:"found"`
	Name      string `json:"name"`
	Blurb     string `json:"blurb"`
	SourceURL string `json:"source_url"`
	PhotoFile string `json:"photo_file"`
}

func isWhoAnswer(object map[string]any) bool {
	_, hasFound := object["found"]
	return hasFound
}

func decodeWhoAnswer(output string) (domain.WhoAnswer, error) {
	payload, err := parseJSON(output)
	if err != nil {
		return domain.WhoAnswer{}, err
	}
	object, err := agentObject(payload, isWhoAnswer)
	if err != nil {
		return domain.WhoAnswer{}, err
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return domain.WhoAnswer{}, err
	}
	var decoded whoPayload
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return domain.WhoAnswer{}, err
	}
	if decoded.Found == nil {
		return domain.WhoAnswer{}, errors.New("agent output has no found flag")
	}
	return domain.NewWhoAnswer(*decoded.Found, decoded.Name, decoded.Blurb, decoded.SourceURL, decoded.PhotoFile)
}
