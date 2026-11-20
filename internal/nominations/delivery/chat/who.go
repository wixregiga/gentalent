package chat

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gentalent/internal/nominations/app/command"
	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

const (
	whoUsage         = "Find out who someone is: !!gtwho the woman who wrote the first computer program"
	descriptionLimit = 300
	blurbLimit       = 400
	quotedLimit      = 80
)

func (b *Bot) searchWho(ctx context.Context, from Member, msg Message, match []string) error {
	description := strings.Join(strings.Fields(match[1]), " ")
	if description == "" {
		return b.say(ctx, msg, whoUsage)
	}
	if utf8.RuneCountInString(description) > descriptionLimit {
		return b.say(ctx, msg, fmt.Sprintf("A description can be at most %d characters.", descriptionLimit))
	}
	cmd := command.SearchWho{MessageID: msg.ID, By: from.ID, Description: description, At: b.config.Now()}
	return b.whoSearch.Handle(ctx, cmd, present(b, msg.ID, nil, func(w wording, search domain.WhoSearch) []port.Reply {
		return []port.Reply{{Text: "Looking up who that is. The answer follows here."}}
	}))
}

func (b *Bot) ClaimWhoSearches(ctx context.Context) ([]WhoSearch, error) {
	claimed, err := b.claimWho.Handle(ctx)
	if err != nil {
		return nil, err
	}
	searches := make([]WhoSearch, len(claimed))
	for i, search := range claimed {
		searches[i] = WhoSearch{Number: int(search.Number), Description: search.Description, Dir: b.whoDir(search.Number)}
	}
	return searches, nil
}

func (b *Bot) ReceiveWho(ctx context.Context, result WhoResult) error {
	number := domain.WhoSearchNumber(result.Number)
	cmd := command.FinishWhoSearch{Number: number, Status: domain.WhoFailed}
	name := formatName(result.Name)
	switch spelling := spellingOf(name); {
	case result.Failure != "":
	case result.NoMatch:
		cmd.Status = domain.WhoNoMatch
	case spelling != "":
		cmd.Status = domain.WhoFound
		cmd.Figure = domain.PublicFigure{
			Name:     name,
			Spelling: spelling,
			Blurb:    inline(result.Blurb, blurbLimit),
			Source:   strings.TrimSpace(result.Source),
			Photo:    b.whoPhoto(number, result.Photo),
			Provider: result.Provider,
		}
	}
	return b.finishWho.Handle(ctx, cmd, present(b, fmt.Sprintf("who/%d", number), nil, func(w wording, search domain.WhoSearch) []port.Reply {
		return []port.Reply{w.whoAnswer(search)}
	}))
}

func (b *Bot) abandonInterruptedWhoSearches(ctx context.Context, now time.Time) error {
	return b.abandonWho.Handle(ctx, present(b, eventKey("who-abandoned", now), nil, func(w wording, abandoned []domain.WhoSearch) []port.Reply {
		replies := make([]port.Reply, len(abandoned))
		for i, search := range abandoned {
			replies[i] = port.Reply{Text: lines(whoAsked(search), "The search was interrupted, so it has no answer.")}
		}
		return replies
	}))
}

func (r *Reader) WhoSearchesRunning(ctx context.Context) ([]int, error) {
	standing, err := r.standing.Handle(ctx)
	if err != nil {
		return nil, err
	}
	running := make([]int, len(standing.WhoSearching))
	for i, number := range standing.WhoSearching {
		running[i] = int(number)
	}
	return running, nil
}

func (w wording) whoAnswer(search domain.WhoSearch) port.Reply {
	switch search.Status {
	case domain.WhoNoMatch:
		return port.Reply{Text: lines(whoAsked(search), "No reliable public match for that description.")}
	case domain.WhoFound:
		figure := search.Figure
		noPhoto := ""
		if figure.Photo == "" {
			noPhoto = "No photo found."
		}
		return port.Reply{
			Text:       lines(whoAsked(search), "Best match: "+literal(figure.Name)+". "+literal(figure.Blurb), w.whoStanding(figure), noPhoto),
			Attachment: figure.Photo,
		}
	}
	return port.Reply{Text: lines(whoAsked(search), "I couldn't work out who that is. Try again with more detail.")}
}

func (w wording) whoStanding(figure domain.PublicFigure) string {
	person, ok := w.group.Person(figure.Person)
	if !ok {
		return "Not in GenTalent yet."
	}
	in := "Already in"
	if person.Name != figure.Name {
		in += " as " + literal(person.Name)
	}
	latest, ok := w.latestOf(person.ID)
	if !ok {
		return in + "."
	}
	return fmt.Sprintf("%s: %s, #%d", in, latest.Status, latest.Number)
}

func (w wording) latestOf(person domain.PersonID) (domain.Nomination, bool) {
	nominations := w.group.Nominations()
	for i := len(nominations) - 1; i >= 0; i-- {
		if nominations[i].Person == person {
			return nominations[i], true
		}
	}
	return domain.Nomination{}, false
}

func (w wording) whoLimitReached(r domain.Refusal) string {
	if r.Limit <= 0 {
		return "Who searches are turned off for this Group."
	}
	searches := "who searches"
	if r.Limit == 1 {
		searches = "who search"
	}
	return fmt.Sprintf("You've used your %d %s for the last %s. Your next one opens in %s.",
		r.Limit, searches, window(domain.WhoSearchDay), wait(r.Next.Sub(w.bot.config.Now())))
}

func whoAsked(search domain.WhoSearch) string {
	return `Who is "` + literal(inline(search.Description, quotedLimit)) + `"?`
}

func (b *Bot) whoDir(number domain.WhoSearchNumber) string {
	return filepath.Join(b.config.MediaDir, "who", strconv.Itoa(int(number)))
}

func (b *Bot) whoPhoto(number domain.WhoSearchNumber, photo string) string {
	if photo == "" {
		return ""
	}
	inside, err := filepath.Rel(b.whoDir(number), filepath.Clean(photo))
	if err != nil || inside == "." || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.Clean(photo)
}

func wait(length time.Duration) string {
	minutes := int((length + time.Minute - 1) / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	hours, minutes := minutes/60, minutes%60
	var parts []string
	if hours > 0 {
		parts = append(parts, counted(hours, "hour"))
	}
	if minutes > 0 {
		parts = append(parts, counted(minutes, "minute"))
	}
	return strings.Join(parts, " ")
}

func counted(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
