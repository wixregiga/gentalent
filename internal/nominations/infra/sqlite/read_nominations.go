package sqlite

import (
	"context"

	"gentalent/internal/nominations/port"
)

func (r Reader) Nominations(ctx context.Context) ([]port.NominationView, error) {
	return r.views(ctx, "")
}
