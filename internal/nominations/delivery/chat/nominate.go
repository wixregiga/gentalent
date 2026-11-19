package chat

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"gentalent/internal/nominations/app/command"
	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

var (
	nominationStart   = regexp.MustCompile(`(?i)(?:^|\s)(!{1,2})gt(?:\s|$)`)
	renominationStart = regexp.MustCompile(`(?i)(?:^|\s)(!{1,2})gtrenominate(?:\s|$)`)
)

type segment struct {
	keyword string
	body    string
}

func segments(pattern *regexp.Regexp, text string) []segment {
	starts := pattern.FindAllStringSubmatchIndex(text, -1)
	list := make([]segment, len(starts))
	for i, start := range starts {
		end := len(text)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		list[i] = segment{keyword: strings.TrimSpace(text[start[2]:start[1]]), body: strings.TrimSpace(text[start[1]:end])}
	}
	return list
}

func (b *Bot) nominateAll(ctx context.Context, from Member, msg Message, match []string) error {
	parts := segments(nominationStart, strings.TrimSpace(msg.Text))
	if len(parts) == 1 {
		head, tail, _ := strings.Cut(parts[0].body, " ")
		switch strings.ToLower(head) {
		case "help":
			if strings.TrimSpace(tail) == "" {
				return b.help(ctx, from, msg, nil)
			}
		case "status":
			if strings.TrimSpace(tail) == "" {
				return b.status(ctx, from, msg, nil)
			}
		case "list":
			return b.list(ctx, from, msg, []string{"", strings.TrimSpace(tail)})
		}
	}
	candidates, typed, problem, err := b.candidates(parts)
	if err != nil {
		return err
	}
	if problem != "" {
		return b.say(ctx, msg, problem)
	}
	cmd := command.Nominate{MessageID: msg.ID, By: from.ID, Candidates: candidates, At: b.config.Now()}
	return b.nominate.Handle(ctx, cmd, present(b, msg.ID, typed, func(w wording, outcomes []domain.NominateOutcome) []port.Reply {
		replies := make([]port.Reply, 0, len(outcomes))
		for _, outcome := range outcomes {
			if outcome.PhotosOnly {
				replies = append(replies, port.Reply{Text: lines("Finding photos for "+w.name(outcome.Number)+".", w.votesIn(w.nomination(outcome.Number)))})
				continue
			}
			replies = append(replies, w.announcement(outcome.Number, from, msg, "nominate"))
		}
		return replies
	}))
}

func (b *Bot) renominateAll(ctx context.Context, from Member, msg Message, match []string) error {
	candidates, typed, problem, err := b.candidates(segments(renominationStart, strings.TrimSpace(msg.Text)))
	if err != nil {
		return err
	}
	if problem != "" {
		return b.say(ctx, msg, problem)
	}
	cmd := command.Renominate{MessageID: msg.ID, By: from.ID, Candidates: candidates, At: b.config.Now()}
	return b.renominate.Handle(ctx, cmd, present(b, msg.ID, typed, func(w wording, renominated []domain.Nomination) []port.Reply {
		replies := make([]port.Reply, 0, len(renominated))
		for _, nomination := range renominated {
			replies = append(replies, w.announcement(nomination.Number, from, msg, "renominate"))
		}
		return replies
	}))
}

func (b *Bot) candidates(parts []segment) ([]domain.Candidate, map[domain.PersonID]string, string, error) {
	candidates := make([]domain.Candidate, 0, len(parts))
	typed := map[domain.PersonID]string{}
	for _, part := range parts {
		raw, pitch := splitNameAndPitch(part.body)
		name := formatName(raw)
		spelling := spellingOf(name)
		if name == "" || spelling == "" {
			return nil, nil, fmt.Sprintf("Include a nominee name after %s, e.g. %s Ada Lovelace. Why they're a generational talent.", part.keyword, part.keyword), nil
		}
		if utf8.RuneCountInString(name) > nameLimit {
			return nil, nil, fmt.Sprintf("Put a period after the nominee's name, e.g. %s Ada Lovelace. Why they're a generational talent.", part.keyword), nil
		}
		person, err := b.config.NewPersonID()
		if err != nil {
			return nil, nil, "", err
		}
		typed[person] = name
		candidates = append(candidates, domain.Candidate{Spelling: spelling, NewPerson: person, Name: name, Pitch: pitch, WithPhotos: strings.HasPrefix(part.keyword, "!!")})
	}
	return candidates, typed, "", nil
}

func (w wording) announcement(number domain.NominationNumber, from Member, msg Message, verb string) port.Reply {
	nomination := w.nomination(number)
	year := msg.SentAt.Year()
	if msg.SentAt.IsZero() {
		year = w.bot.config.Now().UTC().Year()
	}
	photos := ""
	if nomination.Photos == domain.PhotosRequested {
		photos = "Photos are on the way."
	}
	return port.Reply{
		Text: sections(
			lines(fmt.Sprintf("%s would like to %s %s as a %d Generational Talent Nominee!", w.member(from.ID), verb, w.name(number), year),
				fmt.Sprintf("%d/%d votes are in.", len(nomination.Votes), len(w.bot.config.Members)), photos),
			lines(w.voteKeys(number), "Rules: !gthelp"),
		),
		Announces: number,
	}
}

func (w wording) voteKeys(number domain.NominationNumber) string {
	if len(w.group.Open()) > 1 {
		return fmt.Sprintf("Vote: !!Y %d yes · !!N %d no · !!C %d clear", number, number, number)
	}
	return "Vote: !!Y yes · !!N no · !!C clear"
}
