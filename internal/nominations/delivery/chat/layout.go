package chat

import "strings"

const markdownSpecials = "\\`*_[]<>~|"

func lines(items ...string) string {
	return joinFilled("  \n", items)
}

func sections(blocks ...string) string {
	return joinFilled("\n\n", blocks)
}

func joinFilled(separator string, items []string) string {
	filled := make([]string, 0, len(items))
	for _, item := range items {
		if item != "" {
			filled = append(filled, item)
		}
	}
	return strings.Join(filled, separator)
}

func literal(text string) string {
	var escaped strings.Builder
	for _, r := range text {
		if strings.ContainsRune(markdownSpecials, r) {
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(r)
	}
	return escaped.String()
}

func inline(text string, limit int) string {
	flat := []rune(strings.Join(strings.Fields(text), " "))
	if len(flat) <= limit {
		return string(flat)
	}
	return strings.TrimRight(string(flat[:limit-1]), " ") + "…"
}
