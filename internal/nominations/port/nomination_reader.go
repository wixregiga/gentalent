package port

import "context"

type NominationReader interface {
	Nominations(ctx context.Context) ([]NominationView, error)
}
