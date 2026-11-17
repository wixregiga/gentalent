package worker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf16"

	"gentalent/internal/messaging/domain"
)

func Binding(group domain.Group) string {
	members := slices.Clone(group.Members)
	slices.Sort(members)
	for i, id := range members {
		members[i] = bindingString(id)
	}
	scope := "[" + strings.Join([]string{
		bindingString(group.ChatID),
		bindingString(group.AccountID),
		bindingString(group.Owner),
		"[" + strings.Join(members, ", ") + "]",
	}, ", ") + "]"
	digest := sha256.Sum256([]byte(scope))
	return hex.EncodeToString(digest[:])
}

func bindingString(value string) string {
	var jsonText bytes.Buffer
	encoder := json.NewEncoder(&jsonText)
	encoder.SetEscapeHTML(false)
	encoder.Encode(value)
	var encoded strings.Builder
	for _, r := range strings.TrimSuffix(jsonText.String(), "\n") {
		switch {
		case r < 0x7f:
			encoded.WriteRune(r)
		case r <= 0xffff:
			fmt.Fprintf(&encoded, "\\u%04x", r)
		default:
			a, b := utf16.EncodeRune(r)
			fmt.Fprintf(&encoded, "\\u%04x\\u%04x", a, b)
		}
	}
	return encoded.String()
}
