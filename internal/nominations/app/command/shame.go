package command

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type Shame struct {
	MessageID string
	By        domain.MemberID
	Target    domain.MemberID
	About     domain.Ref
	Reason    string
	Shamed    domain.ShamedMessage
	At        time.Time
}

type ShameGiven struct {
	ID      domain.ShameID
	GifPath string
}

type ShameHandler struct {
	Groups port.GroupStore
	Gifs   port.GifBank
}

func (h ShameHandler) Handle(ctx context.Context, cmd Shame, present port.Present[ShameGiven]) error {
	return h.Groups.Apply(ctx, cmd.MessageID, func(group *domain.Group) ([]port.Reply, error) {
		gif, err := h.Gifs.Pick(group.GifUses())
		if err != nil {
			return nil, fmt.Errorf("pick a shame GIF: %w", err)
		}
		shame := domain.Shame{By: cmd.By, Target: cmd.Target, Reason: cmd.Reason, Shamed: cmd.Shamed, MessageID: cmd.MessageID, At: cmd.At}
		if gif != "" {
			shame.GIF = filepath.Base(gif)
		}
		given, err := group.Shame(shame, cmd.About)
		return present(group, ShameGiven{ID: given.ID, GifPath: gif}, err)
	})
}
