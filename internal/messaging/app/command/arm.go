package command

import (
	"context"
	"fmt"
	"time"

	"gentalent/internal/messaging/port"
)

type Armer struct {
	Platform port.HistoryReader
	Store    port.BaselineStore
	Now      func() time.Time
}

func (a Armer) Arm(ctx context.Context, ifUnarmed bool) (bool, error) {
	baseline, err := a.Store.Baseline(ctx)
	if err != nil {
		return false, err
	}
	if !baseline.ArmedAt.IsZero() {
		return alreadyArmed(ifUnarmed)
	}
	if err := a.Platform.VerifyChat(ctx); err != nil {
		return false, err
	}
	page, err := a.Platform.Messages(ctx, "")
	if err != nil {
		return false, err
	}
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	armed, err := a.Store.Arm(ctx, port.Baseline{Cursor: page.NewestCursor, ArmedAt: now().UTC()})
	if err == nil && !armed {
		return alreadyArmed(ifUnarmed)
	}
	return armed, err
}

func alreadyArmed(ifUnarmed bool) (bool, error) {
	if ifUnarmed {
		return false, nil
	}
	return false, fmt.Errorf("already armed. Restart run normally; rearming could discard missed commands")
}
