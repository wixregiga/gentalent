package files

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gentalent/internal/messaging/domain"
)

func TestPlatformStateStoreRoundTripAndMissingFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "platforms")
	store := NewPlatformStateStore(dir)

	initial, err := store.Load(t.Context(), "beeper")
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if initial.Platform != "beeper" || initial.LastKnownVersion != "" || len(initial.BlockedVersions) != 0 {
		t.Fatalf("unexpected initial state: %+v", initial)
	}

	now := time.Date(2026, 11, 21, 16, 0, 0, 0, time.UTC)
	want := domain.PlatformUpdateState{
		Platform:                 "beeper",
		LastKnownVersion:         "4.3.200",
		LastKnownSpecVersion:     "5.0.0",
		LastManagedVersion:       "4.3.200",
		LastExternalDriftVersion: "4.4.0",
		LastCheckedAt:            now,
		BlockedVersions: []domain.BlockedVersion{
			{
				Version:         "4.4.0",
				AdapterRevision: "beeper-api-v5-r1",
				SpecVersion:     "6.0.0",
				Source:          "external",
				Reasons:         []string{"API spec major version 6 differs from expected major version 5 (observed 6.0.0)"},
				MarkedAt:        now,
			},
		},
	}
	if err := store.Save(t.Context(), want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(t.Context(), "beeper")
	if err != nil {
		t.Fatalf("Load saved: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded state:\n%+v\nwant:\n%+v", got, want)
	}

	if _, err := store.Load(t.Context(), "../escape"); err == nil {
		t.Fatal("accepted invalid platform name")
	}
}
