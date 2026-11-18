package chat

import (
	"context"
	"fmt"

	"gentalent/internal/nominations/domain"
)

func (m Message) replyReference() (ref domain.Ref, unknownBotMessage bool) {
	switch {
	case m.ReplyTo == nil:
		return domain.Ref{}, false
	case m.ReplyTo.Announces != 0:
		return domain.Ref{Number: domain.NominationNumber(m.ReplyTo.Announces)}, false
	}
	return domain.Ref{}, m.ReplyTo.FromBot
}

func (r *Reader) nameOne(ctx context.Context, command string) (string, error) {
	standing, err := r.standing.Handle(ctx)
	if err != nil || len(standing.Open) == 0 {
		return "", err
	}
	listing := []string{fmt.Sprintf("Which Nomination? Name one, e.g. %s %d", command, standing.Open[0].Number)}
	for _, open := range standing.Open {
		listing = append(listing, listedNomination(open.Number, open.Person, len(open.Votes), len(r.config.Members)))
	}
	return lines(listing...), nil
}

func listedNomination(number domain.NominationNumber, person string, votes, members int) string {
	return fmt.Sprintf("Nomination %d: %s, %d/%d votes in", number, literal(person), votes, members)
}

func (b *Bot) refuseUnnamed(ctx context.Context, msg Message, command string) (bool, error) {
	text, err := b.nameOne(ctx, command)
	if err != nil || text == "" {
		return false, err
	}
	return true, b.say(ctx, msg, text)
}
