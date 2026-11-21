package port

import (
	"context"

	"gentalent/internal/messaging/domain"
)

type PlatformUpdateReader interface {
	PlatformName() string
	AdapterRevision() string
	ExpectedSpecMajor() int
	ExpectedSpecVersion() string
	ObservePlatform(context.Context) (domain.PlatformObservation, error)
}

type PlatformStateReader interface {
	Load(ctx context.Context, platform string) (domain.PlatformUpdateState, error)
}

type PlatformStateStore interface {
	PlatformStateReader
	Save(ctx context.Context, state domain.PlatformUpdateState) error
}

type HeaderVersionReader interface {
	LastObservedVersion() string
}
