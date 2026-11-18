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

const rangeUsage = "Name Nominations by number, e.g. !!Y 12 or !!Y 12-14."

var (
	numberReference = regexp.MustCompile(`^#?(\d+)(?:\s*-\s*#?(\d+))?(?:$|[\s.,:;!?])`)
	possessive      = regexp.MustCompile(`(?i)['’]s$`)
	leadingNoise    = ".:;,-\u2013\u2014 "
)

type reference struct {
	refs    []domain.Ref
	rest    string
	refused string
}

type names []port.Name

func (known names) leading(text string) reference {
	text = strings.TrimSpace(text)
	if named, ok := known.named(text); ok {
		return named
	}
	return known.unnamed(text)
}

func (known names) voting(text string) reference {
	text = strings.TrimSpace(text)
	if named, ok := known.named(text); ok {
		return named
	}
	if spelled := known.closeSpelled(strings.Fields(text)); len(spelled) > 0 {
		return reference{refs: []domain.Ref{{Close: spelled}}}
	}
	return known.unnamed(text)
}

func (known names) named(text string) (reference, bool) {
	if match := numberReference.FindStringSubmatch(text); match != nil {
		return numbered(match, strings.TrimLeft(text[len(match[0]):], leadingNoise)), true
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
				return reference{refs: []domain.Ref{{Person: name.Person}}, rest: rest}, true
			}
		}
	}
	return reference{}, false
}

func (known names) unnamed(text string) reference {
	if suggestion := known.nearMiss(strings.Fields(text)); suggestion != "" {
		return reference{refused: didYouMean(suggestion)}
	}
	return reference{rest: text}
}

func numbered(match []string, rest string) reference {
	low, err := strconv.Atoi(match[1])
	if err != nil {
		return reference{refused: noNomination(match[1])}
	}
	high := low
	if match[2] != "" {
		if high, err = strconv.Atoi(match[2]); err != nil {
			return reference{refused: noNomination(match[2])}
		}
	}
	if low < 1 || high < low || high-low >= rangeLimit {
		return reference{refused: rangeUsage}
	}
	refs := make([]domain.Ref, high-low+1)
	for offset := range refs {
		refs[offset] = domain.Ref{Number: domain.NominationNumber(low + offset)}
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

func didYouMean(suggestion string) string {
	return "Did you mean " + literal(suggestion) + "? Name them exactly as the bot shows them."
}

func noNomination(number string) string {
	return "There is no Nomination " + number + "."
}
