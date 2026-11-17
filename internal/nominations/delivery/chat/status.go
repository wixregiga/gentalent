package chat

import (
	"context"
	"fmt"
	"math"
	"strings"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

func (b *Bot) status(ctx context.Context, from Member, msg Message, match []string) error {
	text, err := b.StatusReport(ctx)
	if err != nil {
		return err
	}
	return b.say(ctx, msg, text)
}

func (b *Bot) statusText(standing port.Standing) string {
	record := fmt.Sprintf("Record: %d certified, %d rejected", len(standing.Certified), len(standing.Rejected))
	if len(standing.Open) == 0 {
		last := ""
		if standing.Latest.Number != 0 {
			last = fmt.Sprintf("Last: %s (#%d), %s", literal(standing.Latest.Person), standing.Latest.Number, standing.Latest.Status)
		}
		return sections(lines("No vote right now", last, record), lines(nominateKeys, "More: !gtlist · !gthelp"))
	}
	summary := make([]string, 0, len(standing.Open))
	var finish []string
	for _, open := range standing.Open {
		summary = append(summary, b.openLine(open))
		if !open.LockedAt.IsZero() {
			nominator, _ := b.member(string(open.Nominator))
			finish = append(finish, fmt.Sprintf("Finish now: !final %d (@%s)", open.Number, literal(nominator.Name)))
		}
	}
	keys := []string{voteKeys}
	if len(standing.Open) > 1 {
		keys = []string{fmt.Sprintf("Vote: !!Y %d yes · !!N %d no · !!C %d clear", standing.Open[0].Number, standing.Open[0].Number, standing.Open[0].Number)}
	}
	if only := standing.Open[0]; len(standing.Open) == 1 && only.LockedAt.IsZero() && only.Photos == domain.NoPhotos {
		keys = append(keys, "Add photos: !!gt "+literal(only.Person))
	}
	return sections(lines(append(summary, record)...), lines(append(append(keys, finish...), "More: !gtlist · !gthelp")...))
}

func (b *Bot) openLine(open port.NominationView) string {
	members := len(b.config.Members)
	if !open.LockedAt.IsZero() {
		left := max(0, int(math.Round((domain.ResultDelay - b.config.Now().Sub(open.LockedAt)).Seconds())))
		return fmt.Sprintf("Nomination %d: %s, %d/%d votes in, locked. Result in about %ds.", open.Number, literal(open.Person), members, members, left)
	}
	ballot := make([]string, 0, members)
	for _, member := range b.config.Members {
		switch open.Votes[member.ID] {
		case domain.Yes:
			ballot = append(ballot, "✅ "+literal(member.Name))
		case domain.No:
			ballot = append(ballot, "❌ "+literal(member.Name))
		default:
			ballot = append(ballot, "⏳ "+literal(member.Name))
		}
	}
	return fmt.Sprintf("Nomination %d: %s, %d/%d votes in: %s", open.Number, literal(open.Person), len(open.Votes), members, strings.Join(ballot, " · "))
}
