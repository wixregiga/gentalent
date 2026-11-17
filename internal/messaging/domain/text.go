package domain

import (
	"html"
	"regexp"
	"strings"
)

const ReplyPrefix = "@gentalent-bot says:"

var lineTags = regexp.MustCompile("(?i)</?(?:br|p|div|li|ul|ol|blockquote|h[1-6])\\b[^>]*>")
var inlineTags = regexp.MustCompile("</?[A-Za-z][^>]*>")

func PlainText(text string) string {
	return html.UnescapeString(inlineTags.ReplaceAllString(lineTags.ReplaceAllString(text, " "), ""))
}

func HasReplyPrefix(text string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(PlainText(text))), ReplyPrefix)
}

func SameText(stored, sent string) bool {
	words := func(text string) string {
		plain := strings.Map(func(r rune) rune {
			if strings.ContainsRune("*_`~\\", r) {
				return -1
			}
			return r
		}, PlainText(text))
		return strings.Join(strings.Fields(plain), " ")
	}
	return words(stored) == words(sent)
}
