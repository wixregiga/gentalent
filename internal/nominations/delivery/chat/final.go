package chat

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gentalent/internal/nominations/app/command"
	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

func (b *Bot) final(ctx context.Context, from Member, msg Message, match []string) error {
	var named []domain.Ref
	if body := strings.TrimSpace(match[1]); body != "" {
		known, err := b.names.Handle(ctx)
		if err != nil {
			return err
		}
		reference := names(known).leading(body)
		switch {
		case reference.refused != "":
			return b.say(ctx, msg, reference.refused)
		case len(reference.refs) == 0:
			return nil
		}
		named = reference.refs
	}
	cmd := command.Finalize{MessageID: msg.ID, By: from.ID, Named: named, At: msg.SentAt}
	return b.finalize.Handle(ctx, cmd, present(b, msg.ID, nil, wording.verdicts))
}

func (w wording) verdicts(finalized []domain.Nomination) []port.Reply {
	if len(finalized) == 0 {
		return nil
	}
	members := len(w.bot.config.Members)
	verdicts := make([]string, len(finalized))
	var strips []port.Reply
	for i, nomination := range finalized {
		if nomination.Status == domain.Certified {
			verdicts[i] = fmt.Sprintf("Congratulations %s! You are officially a Generational Talent!", w.name(nomination.Number))
		} else {
			verdicts[i] = fmt.Sprintf("Rejected: %s did not get a unanimous vote (%d/%d yes).", w.name(nomination.Number), nomination.Count(domain.Yes), members)
		}
		if strip := w.verdictStrip(nomination); strip != "" {
			strips = append(strips, port.Reply{Attachment: strip})
		}
	}
	return append([]port.Reply{{Text: lines(verdicts...)}}, strips...)
}

func (w wording) verdictStrip(nomination domain.Nomination) string {
	if w.bot.config.PhotoLayout != Individual || nomination.Photos != domain.PhotosFound || nomination.Strip == "" {
		return ""
	}
	if info, err := os.Stat(nomination.Strip); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return nomination.Strip
}
