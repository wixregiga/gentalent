package command

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type Poller struct {
	Platform    port.PollReader
	Store       port.PollStore
	Group       domain.Group
	NeedsLinked func(text string) bool
}

func (p Poller) Poll(ctx context.Context) error {
	baseline, err := p.Store.Baseline(ctx)
	if err != nil {
		return err
	}
	if baseline.ArmedAt.IsZero() {
		return fmt.Errorf("run arm first, in the same dry/live mode, before processing messages")
	}
	if err := p.Platform.VerifyChat(ctx); err != nil {
		return err
	}
	cursor := baseline.Cursor
	seen := map[string]bool{cursor: true}
	for {
		page, err := p.Platform.Messages(ctx, cursor)
		if err != nil {
			return err
		}
		next := page.NewestCursor
		if page.HasMore {
			switch {
			case cursor == "":
				return fmt.Errorf("no baseline cursor and history spans multiple pages, establish a fresh baseline")
			case next == "" || next == cursor:
				return fmt.Errorf("platform did not advance a pagination cursor, stopped without guessing")
			case seen[next]:
				return fmt.Errorf("platform repeated a pagination cursor, stopped without guessing")
			}
		} else if next == "" {
			next = cursor
		} else if next != cursor && seen[next] {
			return fmt.Errorf("platform repeated a pagination cursor, stopped without guessing")
		}
		items, err := p.inboxItems(ctx, page.Messages, baseline.ArmedAt)
		if err != nil {
			return err
		}
		if err := p.Store.SavePage(ctx, cursor, next, items); err != nil {
			return fmt.Errorf("save inbox page and cursor: %w", err)
		}
		if !page.HasMore {
			return nil
		}
		seen[next] = true
		cursor = next
	}
}

func (p Poller) inboxItems(ctx context.Context, messages []domain.Message, armedAt time.Time) ([]port.InboxItem, error) {
	slices.SortStableFunc(messages, func(a, b domain.Message) int {
		return cmp.Or(a.SentAt.Compare(b.SentAt), cmp.Compare(a.SortKey, b.SortKey))
	})
	var items []port.InboxItem
	for _, raw := range messages {
		if !p.Group.Accepts(raw, armedAt) {
			continue
		}
		own, err := p.fromBot(ctx, raw)
		if err != nil {
			return nil, err
		}
		if own {
			continue
		}
		text := domain.PlainText(raw.Text)
		if strings.TrimSpace(text) == "" {
			continue
		}
		message := domain.Incoming{
			ID: raw.ID, Sender: raw.Sender, Text: text, SentAt: raw.SentAt, Mentions: raw.Mentions,
		}
		if raw.LinkedID != "" && (p.NeedsLinked == nil || p.NeedsLinked(message.Text)) {
			message.Linked, err = p.linked(ctx, raw.LinkedID)
			if err != nil {
				return nil, err
			}
		}
		items = append(items, port.InboxItem{Message: message, SortKey: raw.SortKey})
	}
	return items, nil
}

func (p Poller) linked(ctx context.Context, id string) (*domain.Linked, error) {
	raw, err := p.Platform.Retrieve(ctx, id)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return &domain.Linked{ID: id, Missing: true}, nil
	}
	own, err := p.fromBot(ctx, raw)
	if err != nil {
		return nil, err
	}
	text := domain.PlainText(raw.Text)
	if strings.TrimSpace(text) == "" && len(raw.Attachments) != 0 {
		text = "[photo]"
	}
	return &domain.Linked{ID: raw.ID, Sender: raw.Sender, Text: text, SentAt: raw.SentAt, FromBot: own}, nil
}

func (p Poller) fromBot(ctx context.Context, raw domain.Message) (bool, error) {
	if domain.HasReplyPrefix(raw.Text) {
		return true, nil
	}
	known, err := p.Store.KnownReplyID(ctx, raw.ID)
	if known || err != nil || raw.Sender != p.Group.Owner {
		return known, err
	}
	for _, attachment := range raw.Attachments {
		if attachment.Name == "" {
			continue
		}
		known, err := p.Store.KnownAttachment(ctx, attachment.Name)
		if known || err != nil {
			return known, err
		}
	}
	return false, nil
}
