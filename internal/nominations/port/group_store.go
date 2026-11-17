package port

import (
	"context"

	"gentalent/internal/nominations/domain"
)

type Reply struct {
	Key        string
	Text       string
	Attachment string
	Announces  domain.NominationNumber
}

type Change func(*domain.Group) ([]Reply, error)

type Present[R any] func(*domain.Group, R, error) ([]Reply, error)

type GroupStore interface {
	Apply(ctx context.Context, messageID string, change Change) error
}
