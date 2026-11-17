package chat

import (
	"context"
	"fmt"
)

const (
	voteKeys     = "Vote: !!Y yes · !!N no · !!C clear"
	nominateKeys = "Nominate: !gt Name. Pitch  \nWith photos: !!gt Name. Pitch"
)

func (b *Bot) help(ctx context.Context, from Member, msg Message, match []string) error {
	members := len(b.config.Members)
	rule := "Any NO denies forever."
	renominate := ""
	if b.config.Settings.AllowRenomination {
		rule = "Any NO rejects."
		renominate = "Renominate the rejected: !gtrenominate Name. Pitch"
	}
	return b.say(ctx, msg, sections(
		lines(
			"How GenTalent works",
			fmt.Sprintf("%d/%d YES certifies. %s", members, members, rule),
			"Change or clear your vote until the last vote is in.",
			"Then the result posts in 1 minute, or the nominator sends !final.",
		),
		lines(
			nominateKeys,
			voteKeys,
			"Name a Nomination by number or name: !!Y 12 · !!N 12-14 · !!Y Ada Lovelace",
			renominate,
			"Finish early: !final (nominator) · !final 12",
			"Status: !gtstatus",
			"Lists: !gtlist · !gtlist losers",
			"Shame: reply to a message with !gtshame reason, or !gtshame @Name reason",
			"Ideas for the bot: !gtfeedback your idea",
		),
	))
}
