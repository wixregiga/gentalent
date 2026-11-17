package chat

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"gentalent/internal/nominations/domain"
)

func personKey(name string) domain.PersonID {
	var key strings.Builder
	for _, r := range cases.Fold().String(norm.NFKC.String(name)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			key.WriteRune(r)
		}
	}
	return domain.PersonID(key.String())
}
