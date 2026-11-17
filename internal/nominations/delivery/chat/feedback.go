package chat

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"gentalent/internal/nominations/port"
)

const feedbackLimit = 2000

func (b *Bot) recordFeedback(ctx context.Context, from Member, msg Message, match []string) error {
	keyword := strings.ToLower(match[1])
	kind := "feedback"
	if keyword == "suggest" {
		kind = "suggestion"
	}
	text := strings.TrimSpace(match[2])
	switch {
	case text == "":
		return b.say(ctx, msg, fmt.Sprintf("Add your %s after the command, e.g. !gt%s show who voted last.", kind, keyword))
	case utf8.RuneCountInString(text) > feedbackLimit:
		return b.say(ctx, msg, fmt.Sprintf("Keep it under %d characters.", feedbackLimit))
	}
	entry := port.Feedback{MessageID: msg.ID, Sender: from.ID, Kind: kind, Text: text, SaidAt: msg.SentAt}
	thanks := fmt.Sprintf("Thanks @%s, your %s is logged and will be considered for a future release.", literal(from.Name), kind)
	return b.feedback.Handle(ctx, entry, keyed(msg.ID, []port.Reply{{Text: thanks}}))
}
