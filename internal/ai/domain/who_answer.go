package domain

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	nameLimit   = 120
	blurbLimit  = 600
	sourceLimit = 2000
)

type WhoAnswer struct {
	Found     bool
	Name      string
	Blurb     string
	Source    string
	PhotoFile string
}

func NewWhoAnswer(found bool, name, blurb, source, photoFile string) (WhoAnswer, error) {
	if !found {
		return WhoAnswer{}, nil
	}
	checkedName, err := oneLine("name", name, nameLimit)
	if err != nil {
		return WhoAnswer{}, err
	}
	checkedBlurb, err := oneLine("blurb", blurb, blurbLimit)
	if err != nil {
		return WhoAnswer{}, err
	}
	file := strings.TrimSpace(photoFile)
	if file != "" {
		if file, err = rawFilename("photo_file", file); err != nil {
			return WhoAnswer{}, err
		}
	}
	return WhoAnswer{Found: true, Name: checkedName, Blurb: checkedBlurb, Source: sourceLink(source), PhotoFile: file}, nil
}

func oneLine(field, value string, limit int) (string, error) {
	folded := strings.Join(strings.Fields(value), " ")
	if strings.ContainsFunc(folded, unicode.IsControl) {
		return "", fmt.Errorf("%s must not hold control characters", field)
	}
	if n := utf8.RuneCountInString(folded); n < 1 || n > limit {
		return "", fmt.Errorf("%s must be 1 to %d characters", field, limit)
	}
	return folded, nil
}

func sourceLink(value string) string {
	link := strings.TrimSpace(value)
	if link == "" || len(link) > sourceLimit || strings.ContainsFunc(link, unicode.IsSpace) {
		return ""
	}
	parsed, err := url.Parse(link)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return ""
	}
	return link
}
