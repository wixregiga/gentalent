package query

import (
	"context"
	"testing"
	"time"

	"gentalent/internal/messaging/domain"
)

type fakePlatformUpdateReader struct {
	obs domain.PlatformObservation
	err error
}

func (f fakePlatformUpdateReader) PlatformName() string        { return "beeper" }
func (f fakePlatformUpdateReader) AdapterRevision() string     { return "beeper-api-v5-r1" }
func (f fakePlatformUpdateReader) ExpectedSpecMajor() int      { return 5 }
func (f fakePlatformUpdateReader) ExpectedSpecVersion() string { return "5.0.0" }
func (f fakePlatformUpdateReader) ObservePlatform(context.Context) (domain.PlatformObservation, error) {
	return f.obs, f.err
}

type fakePlatformStateReader struct {
	state domain.PlatformUpdateState
	err   error
}

func (f fakePlatformStateReader) Load(context.Context, string) (domain.PlatformUpdateState, error) {
	return f.state, f.err
}

func TestPlatformUpdateStatusCheckIncludesReplyIssuesAndPendingUpdate(t *testing.T) {
	now := time.Date(2026, 11, 21, 13, 0, 0, 0, time.UTC)
	status := PlatformUpdateStatus{
		Platform: fakePlatformUpdateReader{
			obs: domain.PlatformObservation{
				Platform:       "beeper",
				CurrentVersion: "4.3.200",
				SpecVersion:    "5.0.0",
				Pending: &domain.PendingUpdate{
					Version:  "4.3.210",
					Verified: true,
					Source:   domain.PendingSourceCache,
				},
				ObservedAt: now,
			},
		},
		State: fakePlatformStateReader{
			state: domain.PlatformUpdateState{
				Platform:             "beeper",
				LastKnownVersion:     "4.3.200",
				LastKnownSpecVersion: "5.0.0",
			},
		},
	}

	got, err := status.Check(t.Context(), []string{"Message 1 text differs"}, 1)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got.AutoInstallAllowed || got.Compatibility != domain.CompatibilityIncompatible || !got.KeepRunning || !got.RequiresCodeChanges {
		t.Fatalf("unexpected assessment: %+v", got)
	}
	if got.Pending == nil || got.Pending.Version != "4.3.210" {
		t.Fatalf("pending update dropped: %+v", got.Pending)
	}
}
