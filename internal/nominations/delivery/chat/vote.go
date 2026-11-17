package chat

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"gentalent/internal/nominations/app/command"
	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

var (
	voteStart = regexp.MustCompile(`(?i)(?:^|\s)!!(yes|y|no|n|clear|c)(?:$|[\s!.,;:\-—–])`)
	voteWords = map[string]domain.Vote{"yes": domain.Yes, "y": domain.Yes, "no": domain.No, "n": domain.No, "clear": domain.NotVoted, "c": domain.NotVoted}
)

const rangeUsage = "Name Nominations by number, e.g. !!Y 12 or !!Y 12-14."

func (b *Bot) vote(ctx context.Context, from Member, msg Message, match []string) error {
	text := strings.TrimSpace(msg.Text)
	known, err := b.names.Handle(ctx)
	if err != nil {
		return err
	}
	starts := voteStart.FindAllStringSubmatchIndex(text, -1)
	var ballots []domain.Ballot
	for i, start := range starts {
		end := len(text)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		choice := voteWords[strings.ToLower(text[start[2]:start[3]])]
		named := names(known).leading(text[start[1]:end])
		switch {
		case named.badRange:
			return b.say(ctx, msg, rangeUsage)
		case named.suggestion != "":
			return b.say(ctx, msg, didYouMean(named.suggestion))
		case len(named.refs) == 0:
			ballots = append(ballots, domain.Ballot{Vote: choice})
		}
		for _, ref := range named.refs {
			ballots = append(ballots, domain.Ballot{Nomination: ref, Vote: choice})
		}
	}
	cmd := command.CastVote{MessageID: msg.ID, By: from.ID, Ballots: ballots, At: msg.SentAt}
	return b.castVote.Handle(ctx, cmd, present(b, msg.ID, nil, func(w wording, outcomes []domain.VoteOutcome) []port.Reply {
		return w.voteReplies(outcomes)
	}))
}

func (w wording) voteReplies(outcomes []domain.VoteOutcome) []port.Reply {
	if len(outcomes) == 0 {
		return nil
	}
	parts := make([]string, len(outcomes))
	for i, outcome := range outcomes {
		parts[i] = w.voteText(outcome)
	}
	return []port.Reply{{Text: sections(parts...)}}
}

func (w wording) voteText(outcome domain.VoteOutcome) string {
	nomination := w.nomination(outcome.Number)
	voter, name := w.member(outcome.Member), w.name(outcome.Number)
	cast := nomination.Votes[outcome.Member]
	switch {
	case outcome.Change == domain.VoteCleared:
		return lines(voter+" cleared their vote on "+name+".", w.votesIn(nomination))
	case outcome.Change == domain.VoteChanged:
		return lines(fmt.Sprintf("%s changed their vote to %s on %s.", voter, strings.ToUpper(string(cast)), name), w.votesIn(nomination))
	}
	headline := voter + " has voted for " + name + " to be a generational talent!"
	if cast == domain.No {
		headline = voter + " has voted against " + name + " being a generational talent."
	}
	if !outcome.Locks {
		return lines(headline, w.votesIn(nomination))
	}
	members := len(w.bot.config.Members)
	return lines(headline, fmt.Sprintf("%d/%d votes in. Result in 1 minute.", members, members),
		"Finish now: !final ("+w.member(nomination.Nominator)+")")
}

func didYouMean(suggestion string) string {
	return "Did you mean " + literal(suggestion) + "? Name them exactly as the bot shows them."
}
