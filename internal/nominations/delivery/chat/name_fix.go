package chat

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gentalent/internal/nominations/app/command"
	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

const (
	nameFixUsage    = "Fix a name: !gtfix 12 Ada Lovelace. With new photos: !!gtfix 12 Ada Lovelace"
	nameAnswerUsage = "Answer a fix by its number: !gtfixyes 3 or !gtfixno 3"
)

var (
	nameFixStart  = regexp.MustCompile(`(?i)(?:^|\s)(!{1,2})gtfix(?:\s|$)`)
	nameFixTarget = regexp.MustCompile(`^#?(\d+)[\s.,:;]+(\S.*)$`)
	nameFixNumber = regexp.MustCompile(`^#?(\d+)$`)
)

func (b *Bot) proposeNameFixes(ctx context.Context, from Member, msg Message, match []string) error {
	parts := segments(nameFixStart, strings.TrimSpace(msg.Text))
	proposals := make([]domain.NameFixProposal, 0, len(parts))
	for _, part := range parts {
		target := nameFixTarget.FindStringSubmatch(strings.TrimSpace(part.body))
		if target == nil {
			return b.say(ctx, msg, nameFixUsage)
		}
		number, err := strconv.Atoi(target[1])
		if err != nil {
			return b.say(ctx, msg, noNomination(target[1]))
		}
		name := formatName(target[2])
		spelling := spellingOf(name)
		if spelling == "" {
			return b.say(ctx, msg, nameFixUsage)
		}
		if utf8.RuneCountInString(name) > nameLimit {
			return b.say(ctx, msg, fmt.Sprintf("A name can be at most %d characters.", nameLimit))
		}
		proposals = append(proposals, domain.NameFixProposal{
			Nomination: domain.NominationNumber(number),
			Name:       name,
			Spelling:   spelling,
			WithPhotos: strings.HasPrefix(part.keyword, "!!"),
		})
	}
	cmd := command.ProposeNameFixes{MessageID: msg.ID, By: from.ID, Proposals: proposals, At: b.config.Now()}
	return b.proposeFixes.Handle(ctx, cmd, present(b, msg.ID, nil, func(w wording, attempts []command.NameFixAttempt) []port.Reply {
		replies := make([]port.Reply, 0, len(attempts))
		for _, attempt := range attempts {
			if attempt.Refusal != nil {
				replies = append(replies, port.Reply{Text: w.nameFixRefusal(*attempt.Refusal)})
				continue
			}
			fix, _ := w.group.NameFix(attempt.Fix)
			replies = append(replies, port.Reply{Text: w.nameFixProposed(fix)})
		}
		return replies
	}))
}

func (b *Bot) answerNameFix(ctx context.Context, from Member, msg Message, match []string) error {
	target := nameFixNumber.FindStringSubmatch(strings.TrimSpace(match[2]))
	if target == nil {
		return b.say(ctx, msg, nameAnswerUsage)
	}
	number, err := strconv.Atoi(target[1])
	if err != nil {
		return b.say(ctx, msg, "There is no Fix "+target[1]+".")
	}
	cmd := command.AnswerNameFix{MessageID: msg.ID, By: from.ID, Fix: domain.NameFixNumber(number), Agrees: strings.EqualFold(match[1], "yes"), At: b.config.Now()}
	return b.answerFix.Handle(ctx, cmd, present(b, msg.ID, nil, func(w wording, fix domain.NameFix) []port.Reply {
		return []port.Reply{{Text: w.nameFixAnswered(fix, from)}}
	}))
}

func (b *Bot) ExpireNameFixes(ctx context.Context, now time.Time) error {
	return b.expireFixes.Handle(ctx, command.ExpireNameFixes{Now: now}, present(b, eventKey("lapse", now), nil, func(w wording, lapsed []domain.NameFix) []port.Reply {
		replies := make([]port.Reply, 0, len(lapsed))
		for _, fix := range lapsed {
			replies = append(replies, port.Reply{Text: fmt.Sprintf("Fix %d lapsed without enough agreement. %s keeps that name.", fix.Number, w.name(fix.Nomination))})
		}
		return replies
	}))
}

func (w wording) nameFixProposed(fix domain.NameFix) string {
	if fix.Status == domain.NameFixed {
		return w.nameFixed(fix)
	}
	photos := ""
	if fix.WithPhotos {
		photos = "Photos are searched under the new name once it's fixed."
	}
	return sections(
		lines(
			fmt.Sprintf("Fix %d for #%d: %s → %s? Votes carry over.", fix.Number, fix.Nomination, literal(fix.From), literal(fix.To)),
			fmt.Sprintf("%s proposed it. It needs %d of %d Members.", w.member(fix.Proposer), w.group.AgreementNeeded(), len(w.bot.config.Members)),
			photos,
		),
		lines(
			fmt.Sprintf("Agree: !gtfixyes %d · Stop: !gtfixno %d", fix.Number, fix.Number),
			fmt.Sprintf("It lapses in %s without enough agreement.", window(w.group.NameFixWindow())),
		),
	)
}

func (w wording) nameFixAnswered(fix domain.NameFix, from Member) string {
	switch fix.Status {
	case domain.NameFixed:
		return w.nameFixed(fix)
	case domain.NameFixWithdrawn:
		return fmt.Sprintf("Fix %d withdrawn. %s keeps that name.", fix.Number, w.name(fix.Nomination))
	case domain.NameFixStopped:
		return fmt.Sprintf("Fix %d stopped. %s keeps that name.", fix.Number, w.name(fix.Nomination))
	}
	said := "agrees"
	if fix.Answers[from.ID] == domain.No {
		said = "says no"
	}
	return lines(
		fmt.Sprintf("Fix %d: %s %s. %d of %d agree; it needs %d.", fix.Number, w.member(from.ID), said, fix.Count(domain.Yes), len(w.bot.config.Members), w.group.AgreementNeeded()),
		fmt.Sprintf("Agree: !gtfixyes %d · Stop: !gtfixno %d", fix.Number, fix.Number),
	)
}

func (w wording) nameFixed(fix domain.NameFix) string {
	photos := ""
	if nomination := w.nomination(fix.Nomination); fix.WithPhotos && nomination.Photos == domain.PhotosRequested {
		photos = "Photos are on the way."
	}
	return lines(
		fmt.Sprintf("Name fixed for #%d: %s → %s. Votes and history carried over.", fix.Nomination, literal(fix.From), literal(fix.To)),
		photos,
	)
}

func (w wording) nameFixRefusal(r domain.Refusal) string {
	if r.Rule == domain.PhotosSearchedAtMostOncePerNomination && len(r.Numbers) > 0 {
		return fmt.Sprintf("Photos were already searched for %s. Fix the name alone with !gtfix %d.", w.name(r.Numbers[0]), r.Numbers[0])
	}
	if text, worded := w.refusal(r); worded {
		return text
	}
	return nameFixUsage
}

func (w wording) pendingNameFix(number domain.NameFixNumber) string {
	fix, _ := w.group.NameFix(number)
	return lines(
		fmt.Sprintf("Fix %d is already waiting for #%d: %s → %s.", fix.Number, fix.Nomination, literal(fix.From), literal(fix.To)),
		fmt.Sprintf("Agree: !gtfixyes %d · Stop: !gtfixno %d", fix.Number, fix.Number),
	)
}

func (w wording) endedNameFix(number domain.NameFixNumber) string {
	fix, _ := w.group.NameFix(number)
	return fmt.Sprintf("Fix %d is over: it was %s.", fix.Number, fix.Status)
}

func window(length time.Duration) string {
	if hours := int(length / time.Hour); length%time.Hour == 0 {
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	return length.String()
}
