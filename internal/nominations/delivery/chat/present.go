package chat

import (
	"errors"
	"fmt"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type wording struct {
	bot   *Bot
	group *domain.Group
	typed map[domain.PersonID]string
}

func present[R any](b *Bot, key string, typed map[domain.PersonID]string, words func(wording, R) []port.Reply) port.Present[R] {
	return func(group *domain.Group, result R, err error) ([]port.Reply, error) {
		w := wording{bot: b, group: group, typed: typed}
		var refusal domain.Refusal
		if errors.As(err, &refusal) {
			text, worded := w.refusal(refusal)
			if !worded {
				return nil, err
			}
			return keyed(key, []port.Reply{{Text: text}}), nil
		}
		if err != nil {
			return nil, err
		}
		return keyed(key, words(w, result)), nil
	}
}

func keyed(key string, replies []port.Reply) []port.Reply {
	for i := range replies {
		replies[i].Key = fmt.Sprintf("%s/%d", key, i+1)
	}
	return replies
}

func eventKey(kind string, at time.Time) string {
	return kind + "/" + at.UTC().Format(time.RFC3339Nano)
}

func (w wording) nomination(number domain.NominationNumber) domain.Nomination {
	nomination, _ := w.group.Nomination(number)
	return nomination
}

func (w wording) name(number domain.NominationNumber) string {
	return literal(w.nomination(number).Person.Name) + fmt.Sprintf(" (#%d)", number)
}

func (w wording) person(id domain.PersonID) string {
	for _, nomination := range w.group.Nominations() {
		if nomination.Person.ID == id {
			return literal(nomination.Person.Name)
		}
	}
	return literal(w.typed[id])
}

func (w wording) member(id domain.MemberID) string {
	member, _ := w.bot.member(string(id))
	return "@" + literal(member.Name)
}

func (w wording) votesIn(nomination domain.Nomination) string {
	return fmt.Sprintf("%d/%d votes in.", len(nomination.Votes), len(w.bot.config.Members))
}

func (w wording) listed(numbers []domain.NominationNumber) []string {
	listed := make([]string, len(numbers))
	for i, number := range numbers {
		nomination := w.nomination(number)
		listed[i] = fmt.Sprintf("Nomination %d: %s, %d/%d votes in", number, literal(nomination.Person.Name), len(nomination.Votes), len(w.bot.config.Members))
	}
	return listed
}
