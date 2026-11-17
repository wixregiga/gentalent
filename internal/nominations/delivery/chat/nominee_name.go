package chat

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const nameLimit = 80

var (
	nominateWord  = regexp.MustCompile(`(?i)^nominate\s+`)
	quotedName    = regexp.MustCompile(`(?s)^["“]([^"”]+)["”](?:\s*[.:;,\-—–]*\s*(.*))?$`)
	clauseBreak   = regexp.MustCompile(`\s+[—–-]\s+|:\s+|[!?]+\s+`)
	sentenceBreak = regexp.MustCompile(`\.(\s+|$)`)
	initial       = regexp.MustCompile(`^[A-Za-z](?:\.[A-Za-z])*$`)
	dottedInitial = regexp.MustCompile(`^[A-Za-z](?:\.[A-Za-z])+$`)
	honorifics    = map[string]bool{"dr": true, "mr": true, "mrs": true, "ms": true, "mx": true, "prof": true, "rev": true,
		"st": true, "mt": true, "capt": true, "col": true, "gen": true, "sgt": true}
	suffixes = map[string]bool{"jr": true, "sr": true}
)

func splitNameAndPitch(raw string) (string, string) {
	text := strings.TrimSpace(raw)
	if loc := nominateWord.FindStringIndex(text); loc != nil {
		text = strings.TrimSpace(text[loc[1]:])
	}
	if text == "" {
		return "", ""
	}
	if quoted := quotedName.FindStringSubmatch(text); quoted != nil {
		return strings.TrimSpace(quoted[1]), strings.TrimSpace(quoted[2])
	}
	if first, rest, multiline := strings.Cut(text, "\n"); multiline {
		name, pitch := splitNameAndPitch(first)
		return name, joinFilled(" ", []string{pitch, strings.TrimSpace(rest)})
	}
	start, end, keepDot := -1, -1, false
	if loc := clauseBreak.FindStringIndex(text); loc != nil {
		start, end = loc[0], loc[1]
	}
	for _, loc := range sentenceBreak.FindAllStringIndex(text, -1) {
		tokens := strings.Fields(text[:loc[0]])
		if len(tokens) == 0 {
			continue
		}
		last := strings.TrimLeft(tokens[len(tokens)-1], "(\"“'")
		lower := strings.ToLower(last)
		if initial.MatchString(last) || honorifics[lower] {
			continue
		}
		if start < 0 || loc[0] < start {
			start, end, keepDot = loc[0], loc[1], suffixes[lower] && len(tokens) > 1
		}
		break
	}
	if start < 0 {
		return strings.TrimSpace(strings.TrimRight(text, ".")), ""
	}
	nameEnd := start
	if keepDot {
		nameEnd++
	}
	return strings.TrimSpace(text[:nameEnd]), strings.TrimSpace(text[end:])
}

func formatName(raw string) string {
	cleaned := strings.Join(strings.Fields(raw), " ")
	shouted := utf8.RuneCountInString(cleaned) > 3 && cleaned == strings.ToUpper(cleaned) && strings.IndexFunc(cleaned, unicode.IsLetter) >= 0
	words := strings.Split(cleaned, " ")
	for i, word := range words {
		pieces := strings.Split(word, "-")
		for j, piece := range pieces {
			pieces[j] = formatPiece(piece, shouted)
		}
		words[i] = strings.Join(pieces, "-")
	}
	return strings.Join(words, " ")
}

func formatPiece(piece string, shouted bool) string {
	core, dot := strings.CutSuffix(piece, ".")
	trailing := ""
	if dot {
		trailing = "."
	}
	if core == "" {
		return piece
	}
	runes := []rune(core)
	switch {
	case dottedInitial.MatchString(core):
		return strings.ToUpper(core) + trailing
	case len(runes) == 1 && unicode.IsLetter(runes[0]) && dot:
		return strings.ToUpper(core) + "."
	case allLower(core) || shouted:
		return strings.ToUpper(string(runes[0])) + strings.ToLower(string(runes[1:])) + trailing
	}
	return core + trailing
}

func allLower(text string) bool {
	cased := false
	for _, r := range text {
		if unicode.IsUpper(r) || unicode.IsTitle(r) {
			return false
		}
		cased = cased || unicode.IsLower(r)
	}
	return cased
}
