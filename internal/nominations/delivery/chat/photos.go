package chat

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"gentalent/internal/nominations/app/command"
	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

const slugLimit = 48

func (b *Bot) ClaimPhotoSearches(ctx context.Context) ([]PhotoSearch, error) {
	claimed, err := b.claim.Handle(ctx)
	if err != nil {
		return nil, err
	}
	searches := make([]PhotoSearch, len(claimed))
	for i, nomination := range claimed {
		searches[i] = PhotoSearch{
			Nomination: int(nomination.Number),
			Person:     nomination.Person.Name,
			Pitch:      nomination.Pitch,
			Dir:        filepath.Join(b.config.MediaDir, "nominations", fmt.Sprintf("%d-%s", nomination.Number, slug(nomination.Person.Name))),
		}
	}
	return searches, nil
}

func (b *Bot) AttachPhotos(ctx context.Context, result PhotoResult) error {
	number := domain.NominationNumber(result.Nomination)
	cmd := command.FinishPhotoSearch{Nomination: number, Found: result.Failure == "", Strip: result.Strip}
	if cmd.Found {
		cmd.Exhibit = fmt.Sprintf("AI media (%s) [sources: %s]", result.Provider, result.Sources)
	}
	return b.finish.Handle(ctx, cmd, present(b, fmt.Sprintf("photos/%d", number), nil, func(w wording, nomination domain.Nomination) []port.Reply {
		if !cmd.Found {
			return []port.Reply{{Text: "No photos found for " + w.name(number) + "."}}
		}
		heading := "Photos for " + w.name(number) + "."
		if b.config.PhotoLayout == Stitched {
			return []port.Reply{{Text: heading, Attachment: result.Strip}}
		}
		replies := []port.Reply{{Text: heading}}
		for _, photo := range result.Photos {
			replies = append(replies, port.Reply{Attachment: photo})
		}
		return replies
	}))
}

func (b *Bot) AbandonInterruptedSearches(ctx context.Context) error {
	now := b.config.Now()
	return b.abandon.Handle(ctx, present(b, eventKey("abandoned", now), nil, func(w wording, abandoned []domain.Nomination) []port.Reply {
		replies := make([]port.Reply, len(abandoned))
		for i, nomination := range abandoned {
			replies[i] = port.Reply{Text: "The photo search for " + w.name(nomination.Number) + " was interrupted, so it has no photos."}
		}
		return replies
	}))
}

func slug(name string) string {
	var built strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			built.WriteRune(r)
			dash = false
		} else if !dash && built.Len() > 0 {
			built.WriteByte('-')
			dash = true
		}
	}
	trimmed := strings.Trim(built.String(), "-")
	if len(trimmed) > slugLimit {
		trimmed = strings.Trim(trimmed[:slugLimit], "-")
	}
	if trimmed == "" {
		return "nominee"
	}
	return trimmed
}
