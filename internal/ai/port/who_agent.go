package port

import (
	"context"

	"gentalent/internal/ai/domain"
)

type WhoAgent interface {
	Identify(ctx context.Context, task Task) (domain.WhoAnswer, error)
}
