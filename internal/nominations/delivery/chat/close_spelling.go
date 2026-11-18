package chat

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"gentalent/internal/nominations/domain"
)

const (
	closePrefixMinLength   = 3
	closeSpellingMinLength = 4
	closeSpellingRatio     = 0.75
)

func (known names) closeSpelled(words []string) []domain.PersonID {
	if len(known.openPersons()) < 2 {
		return nil
	}
	typed := make([]string, min(len(words), referenceWords))
	for i := range typed {
		typed[i] = closeFold(words[i])
	}
	var spelled []domain.PersonID
	for _, name := range known {
		if name.Open && !slices.Contains(spelled, name.Person) && spellsClosely(typed, name.Display) {
			spelled = append(spelled, name.Person)
		}
	}
	return spelled
}

func (known names) openPersons() []domain.PersonID {
	var open []domain.PersonID
	for _, name := range known {
		if name.Open && !slices.Contains(open, name.Person) {
			open = append(open, name.Person)
		}
	}
	return open
}

func spellsClosely(typed []string, display string) bool {
	var words []string
	for _, field := range strings.Fields(display) {
		if word := closeFold(field); word != "" {
			words = append(words, word)
		}
	}
	if len(typed) == 0 || len(words) == 0 {
		return false
	}
	if utf8.RuneCountInString(typed[0]) >= closePrefixMinLength {
		for _, word := range words {
			if strings.HasPrefix(word, typed[0]) {
				return true
			}
		}
	}
	whole := strings.Join(words, "")
	for count := 1; count <= len(typed); count++ {
		key := strings.Join(typed[:count], "")
		if utf8.RuneCountInString(key) < closeSpellingMinLength {
			continue
		}
		if similarity(key, whole) >= closeSpellingRatio {
			return true
		}
		for _, word := range words {
			if similarity(key, word) >= closeSpellingRatio {
				return true
			}
		}
	}
	return false
}

func closeFold(word string) string {
	var bare strings.Builder
	for _, r := range norm.NFD.String(word) {
		if !unicode.Is(unicode.Mn, r) {
			bare.WriteRune(r)
		}
	}
	var folded strings.Builder
	for _, r := range cases.Fold().String(bare.String()) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			folded.WriteRune(r)
		}
	}
	return folded.String()
}
