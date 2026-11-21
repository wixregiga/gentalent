package command

import (
	"context"
	"testing"
	"time"

	"gentalent/internal/messaging/domain"
)

type fakePlatformObserver struct {
	revision      string
	expectedMajor int
	expectedSpec  string
	obs           domain.PlatformObservation
}

func (f *fakePlatformObserver) PlatformName() string        { return "beeper" }
func (f *fakePlatformObserver) AdapterRevision() string     { return f.revision }
func (f *fakePlatformObserver) ExpectedSpecMajor() int      { return f.expectedMajor }
func (f *fakePlatformObserver) ExpectedSpecVersion() string { return f.expectedSpec }
func (f *fakePlatformObserver) ObservePlatform(context.Context) (domain.PlatformObservation, error) {
	return f.obs, nil
}

type memoryPlatformStateStore struct {
	state domain.PlatformUpdateState
	saves int
}

func (m *memoryPlatformStateStore) Load(_ context.Context, platform string) (domain.PlatformUpdateState, error) {
	if m.state.Platform == "" {
		return domain.PlatformUpdateState{Platform: platform}, nil
	}
	return m.state, nil
}

func (m *memoryPlatformStateStore) Save(_ context.Context, state domain.PlatformUpdateState) error {
	m.state = state
	m.saves++
	return nil
}

func TestPlatformUpdateCoordinatorPersistsVerifyRollbackAndObserveDrift(t *testing.T) {
	now := time.Date(2026, 11, 21, 14, 0, 0, 0, time.UTC)
	observer := &fakePlatformObserver{
		revision:      "beeper-api-v5-r1",
		expectedMajor: 5,
		expectedSpec:  "5.0.0",
		obs: domain.PlatformObservation{
			Platform:       "beeper",
			CurrentVersion: "4.3.200",
			SpecVersion:    "5.0.0",
			ObservedAt:     now,
		},
	}
	store := &memoryPlatformStateStore{}
	coord := PlatformUpdateCoordinator{Platform: observer, State: store, Now: func() time.Time { return now }}

	if _, err := coord.ObserveAndRecord(t.Context(), nil, 0); err != nil {
		t.Fatalf("initial ObserveAndRecord: %v", err)
	}
	if store.saves != 1 || store.state.LastKnownVersion != "4.3.200" {
		t.Fatalf("expected initial save at 4.3.200, saves=%d state=%+v", store.saves, store.state)
	}

	if _, err := coord.ObserveAndRecord(t.Context(), nil, 0); err != nil {
		t.Fatalf("repeat ObserveAndRecord: %v", err)
	}
	if store.saves != 1 {
		t.Fatalf("unchanged observation wrote state %d times", store.saves)
	}

	observer.obs.CurrentVersion = "4.3.220"
	observer.obs.SpecVersion = "6.0.0"
	verification, err := coord.VerifyManagedUpdate(t.Context(), "4.3.200", "4.3.220", nil, 0)
	if err != nil {
		t.Fatalf("VerifyManagedUpdate: %v", err)
	}
	if verification.Compatible || !verification.RequiresRollback || !verification.MarkRequiresCodeChanges {
		t.Fatalf("expected incompatible verify requiring rollback: %+v", verification)
	}

	observer.obs.CurrentVersion = "4.3.200"
	observer.obs.SpecVersion = "5.0.0"
	assessment, err := coord.AssessManagedUpdate(t.Context(), "4.3.220", false, nil, 0)
	if err != nil {
		t.Fatalf("AssessManagedUpdate: %v", err)
	}
	if assessment.ManagedUpdateAllowed || !assessment.CandidateBlocked {
		t.Fatalf("blocked version should be refused: %+v", assessment)
	}

	if _, err := coord.RecordRollback(t.Context(), "4.3.200", "4.3.220", "6.0.0", []string{"doctor --contract failed"}); err != nil {
		t.Fatalf("RecordRollback: %v", err)
	}
	if store.state.LastKnownVersion != "4.3.200" || len(store.state.BlockedVersions) != 1 || store.state.BlockedVersions[0].Reasons[0] != "doctor --contract failed" {
		t.Fatalf("unexpected rollback state: %+v", store.state)
	}
}
