package chat

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

const (
	referenceWords      = 6
	suggestionRatio     = 0.85
	suggestionMinLength = 5
	firstNameMinLength  = 3
	rangeLimit          = 50
)

var (
	numberReference = regexp.MustCompile(`^#?(\d+)(?:\s*-\s*#?(\d+))?(?:$|[\s.,:;!?])`)
	possessive      = regexp.MustCompile(`(?i)['’]s$`)
	leadingNoise    = ".:;,-\u2013\u2014 "
)

type reference struct {
	refs       []domain.Ref
	rest       string
	suggestion string
	badRange   bool
}

type names []port.Name

func (known names) leading(text string) reference {
	text = strings.TrimSpace(text)
	if match := numberReference.FindStringSubmatch(text); match != nil {
		return numbered(match, strings.TrimLeft(text[len(match[0]):], leadingNoise))
	}
	words := strings.Fields(text)
	for count := min(len(words), referenceWords); count > 0; count-- {
		key := personKey(strings.Join(words[:count], " "))
		if key == "" {
			continue
		}
		for _, name := range known {
			if name.Alias == string(key) {
				rest := strings.TrimLeft(strings.Join(words[count:], " "), leadingNoise)
				return reference{refs: []domain.Ref{{Person: name.Person}}, rest: rest}
			}
		}
	}
	return reference{rest: text, suggestion: known.nearMiss(words)}
}

func numbered(match []string, rest string) reference {
	low, _ := strconv.Atoi(match[1])
	high := low
	if match[2] != "" {
		high, _ = strconv.Atoi(match[2])
	}
	if low < 1 || high < low || high-low >= rangeLimit {
		return reference{badRange: true}
	}
	refs := make([]domain.Ref, 0, high-low+1)
	for number := low; number <= high; number++ {
		refs = append(refs, domain.Ref{Number: domain.NominationNumber(number)})
	}
	return reference{refs: refs, rest: rest}
}

func (known names) nearMiss(words []string) string {
	if len(words) > 0 {
		first := personKey(possessive.ReplaceAllString(words[0], ""))
		if utf8.RuneCountInString(string(first)) >= firstNameMinLength {
			for _, name := range known {
				if fields := strings.Fields(name.Display); len(fields) > 0 && personKey(fields[0]) == first {
					return name.Display
				}
			}
		}
	}
	best, bestRatio := "", suggestionRatio
	for count := 1; count <= min(len(words), referenceWords); count++ {
		key := string(personKey(strings.Join(words[:count], " ")))
		if utf8.RuneCountInString(key) < suggestionMinLength {
			continue
		}
		for _, name := range known {
			if ratio := similarity(key, name.Alias); ratio >= bestRatio {
				best, bestRatio = name.Display, ratio
			}
		}
	}
	return best
}
