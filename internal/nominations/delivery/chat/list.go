package chat

import (
	"context"
	"fmt"
	"strings"
)

func (b *Bot) list(ctx context.Context, from Member, msg Message, match []string) error {
	mode := strings.ToLower(strings.TrimSpace(match[1]))
	if mode == "" {
		mode = "winners"
	}
	if mode != "winners" && mode != "losers" {
		return b.say(ctx, msg, "Use !gtlist, !gtlist winners, or !gtlist losers.")
	}
	standing, err := b.standing.Handle(ctx)
	if err != nil {
		return err
	}
	title, mark, more, names := "Winners", "🏆", "More: !gtlist losers · !gtstatus", standing.Certified
	if mode == "losers" {
		title, mark, more, names = "Losers", "🚫", "More: !gtlist winners · !gtstatus", standing.Rejected
	}
	entries := []string{fmt.Sprintf("%s (%d)", title, len(names))}
	for _, name := range names {
		entries = append(entries, mark+" "+literal(name))
	}
	if len(names) == 0 {
		entries = append(entries, "None yet")
	}
	voting := []string{"Now voting: none"}
	if len(standing.Open) > 0 {
		voting = voting[:0]
		for _, open := range standing.Open {
			voting = append(voting, fmt.Sprintf("Now voting: %s (#%d, %d/%d votes in)", literal(open.Person), open.Number, len(open.Votes), len(b.config.Members)))
		}
	}
	return b.say(ctx, msg, sections(lines(entries...), lines(append(voting, more)...)))
}
