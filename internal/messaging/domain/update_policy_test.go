package domain

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNoUnattendedPlatformUpdates(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	policy := NewUpdatePolicy("beeper-api-v5-r1", 5, "5.0.0")
	obs := PlatformObservation{
		Platform:       "beeper",
		CurrentVersion: "4.3.200",
		SpecVersion:    "5.0.0",
		Pending: &PendingUpdate{
			Version:  "4.3.210",
			FileName: "Beeper-4.3.210-x86_64.AppImage",
			SHA512:   "abc123==",
			Verified: true,
			Source:   PendingSourceCache,
		},
		ObservedAt: now,
	}
	got, err := policy.AssessUpdate(AssessRequest{
		Mode:        UpdateModeObserve,
		Observation: obs,
		State:       PlatformUpdateState{Platform: "beeper", LastKnownVersion: "4.3.200", LastKnownSpecVersion: "5.0.0"},
	})
	if err != nil {
		t.Fatalf("AssessUpdate: %v", err)
	}
	if got.AutoInstallAllowed {
		t.Fatal("pending update allowed automatic installation")
	}
	if got.ManagedUpdateAllowed {
		t.Fatal("observe mode should not authorize managed update installation")
	}
	if !got.KeepRunning {
		t.Fatal("observe mode stopped GenTalent for a pending update")
	}
	if got.Pending == nil || got.Pending.Version != "4.3.210" {
		t.Fatalf("pending update not surfaced: %+v", got.Pending)
	}
	if got.NextState.LastKnownVersion != "4.3.200" {
		t.Fatalf("pending update altered running version: %q", got.NextState.LastKnownVersion)
	}
}

func TestIncompatibleManagedUpdateRequiresRollback(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 5, 0, 0, time.UTC)
	policy := NewUpdatePolicy("beeper-api-v5-r1", 5, "5.0.0")
	for _, tc := range []struct {
		name string
		obs  PlatformObservation
		want string
	}{
		{
			name: "spec major version bump",
			obs: PlatformObservation{
				Platform:       "beeper",
				CurrentVersion: "4.3.220",
				SpecVersion:    "6.0.0",
				ObservedAt:     now,
			},
			want: "major version 6",
		},
		{
			name: "contract issues",
			obs: PlatformObservation{
				Platform:       "beeper",
				CurrentVersion: "4.3.220",
				SpecVersion:    "5.0.0",
				ContractIssues: []string{"SendStatus values changed"},
				ObservedAt:     now,
			},
			want: "SendStatus values changed",
		},
		{
			name: "sent reply check issues",
			obs: PlatformObservation{
				Platform:           "beeper",
				CurrentVersion:     "4.3.220",
				SpecVersion:        "5.0.0",
				SentRepliesChecked: 2,
				SentReplyIssues:    []string{"Message 739 text differs from what was sent."},
				ObservedAt:         now,
			},
			want: "text differs",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := policy.VerifyAppliedUpdate(VerifyRequest{
				PreviousVersion: "4.3.200",
				TargetVersion:   "4.3.220",
				Observation:     tc.obs,
				State:           PlatformUpdateState{Platform: "beeper", LastKnownVersion: "4.3.200", LastKnownSpecVersion: "5.0.0"},
				CheckedAt:       now,
			})
			if err != nil {
				t.Fatalf("VerifyAppliedUpdate: %v", err)
			}
			if got.Compatible {
				t.Fatal("incompatible update marked compatible")
			}
			if !got.RequiresRollback || got.RollbackToVersion != "4.3.200" {
				t.Fatalf("expected rollback to 4.3.200, got requires=%v to=%q", got.RequiresRollback, got.RollbackToVersion)
			}
			if !strings.Contains(strings.Join(got.IncompatibleReasons, "; "), tc.want) {
				t.Fatalf("reasons %v missing %q", got.IncompatibleReasons, tc.want)
			}
		})
	}
}

func TestIncompatibleUpdateRequiresCodeChanges(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 10, 0, 0, time.UTC)
	policy := NewUpdatePolicy("beeper-api-v5-r1", 5, "5.0.0")
	got, err := policy.VerifyAppliedUpdate(VerifyRequest{
		PreviousVersion: "4.3.200",
		TargetVersion:   "4.3.220",
		Observation: PlatformObservation{
			Platform:       "beeper",
			CurrentVersion: "4.3.220",
			SpecVersion:    "6.0.0",
			ContractIssues: []string{"Message.sendStatus became required"},
			ObservedAt:     now,
		},
		State:     PlatformUpdateState{Platform: "beeper", LastKnownVersion: "4.3.200", LastKnownSpecVersion: "5.0.0"},
		CheckedAt: now,
	})
	if err != nil {
		t.Fatalf("VerifyAppliedUpdate: %v", err)
	}
	if !got.MarkRequiresCodeChanges || got.Compatibility != CompatibilityIncompatible {
		t.Fatalf("expected requires code changes, got mark=%v compat=%s", got.MarkRequiresCodeChanges, got.Compatibility)
	}
	if got.BlockedEntry == nil || got.BlockedEntry.Version != "4.3.220" || got.BlockedEntry.AdapterRevision != "beeper-api-v5-r1" {
		t.Fatalf("blocked entry = %+v", got.BlockedEntry)
	}
	if len(got.NextState.BlockedVersions) != 1 || got.NextState.BlockedVersions[0].Version != "4.3.220" {
		t.Fatalf("next state blocked versions = %+v", got.NextState.BlockedVersions)
	}
	if got.NextState.LastKnownVersion != "4.3.200" {
		t.Fatalf("rollback next state should stay at previous version 4.3.200, got %q", got.NextState.LastKnownVersion)
	}
}

func TestBlockedVersionRefusedForManagedUpdate(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 15, 0, 0, time.UTC)
	policy := NewUpdatePolicy("beeper-api-v5-r1", 5, "5.0.0")
	state := PlatformUpdateState{
		Platform:             "beeper",
		LastKnownVersion:     "4.3.200",
		LastKnownSpecVersion: "5.0.0",
		BlockedVersions: []BlockedVersion{
			{
				Version:         "4.3.220",
				AdapterRevision: "beeper-api-v5-r1",
				SpecVersion:     "6.0.0",
				Source:          "managed",
				Reasons:         []string{"spec major version 6 differs from expected 5"},
				MarkedAt:        now.Add(-time.Hour),
			},
		},
	}
	got, err := policy.AssessUpdate(AssessRequest{
		Mode:             UpdateModeManaged,
		CandidateVersion: "4.3.220",
		Observation: PlatformObservation{
			Platform:       "beeper",
			CurrentVersion: "4.3.200",
			SpecVersion:    "5.0.0",
			ObservedAt:     now,
		},
		State: state,
	})
	if err != nil {
		t.Fatalf("AssessUpdate: %v", err)
	}
	if got.ManagedUpdateAllowed || !got.CandidateBlocked || !got.RequiresCodeChanges {
		t.Fatalf("blocked candidate should be refused: %+v", got)
	}
	if !strings.Contains(got.RefusalReason, "4.3.220") || !strings.Contains(got.RefusalReason, "--force") {
		t.Fatalf("refusal reason = %q", got.RefusalReason)
	}
}

func TestExternalIncompatibleUpdateKeepsRunning(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 20, 0, 0, time.UTC)
	policy := NewUpdatePolicy("beeper-api-v5-r1", 5, "5.0.0")
	got, err := policy.AssessUpdate(AssessRequest{
		Mode: UpdateModeObserve,
		Observation: PlatformObservation{
			Platform:       "beeper",
			CurrentVersion: "4.4.0",
			SpecVersion:    "6.0.0",
			ContractIssues: []string{"SendStatus values changed"},
			ObservedAt:     now,
		},
		State: PlatformUpdateState{
			Platform:             "beeper",
			LastKnownVersion:     "4.3.200",
			LastKnownSpecVersion: "5.0.0",
		},
	})
	if err != nil {
		t.Fatalf("AssessUpdate: %v", err)
	}
	if !got.KeepRunning {
		t.Fatal("external incompatible update stopped GenTalent instead of keeping running")
	}
	if !got.ExternalUpdateDetected || !got.RequiresCodeChanges || got.Compatibility != CompatibilityIncompatible {
		t.Fatalf("expected external incompatible state, got %+v", got)
	}
	if got.NextState.LastExternalDriftVersion != "4.4.0" {
		t.Fatalf("external drift version = %q, want 4.4.0", got.NextState.LastExternalDriftVersion)
	}
	if !slices.ContainsFunc(got.NextState.BlockedVersions, func(b BlockedVersion) bool {
		return b.Version == "4.4.0" && b.Source == "external"
	}) {
		t.Fatalf("expected external incompatible version blocked for managed updates: %+v", got.NextState.BlockedVersions)
	}
}

func TestMinorSpecBumpIsCompatibleWithReviewNotice(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 25, 0, 0, time.UTC)
	policy := NewUpdatePolicy("beeper-api-v5-r1", 5, "5.0.0")
	got, err := policy.VerifyAppliedUpdate(VerifyRequest{
		PreviousVersion: "4.3.200",
		TargetVersion:   "4.3.210",
		Observation: PlatformObservation{
			Platform:       "beeper",
			CurrentVersion: "4.3.210",
			SpecVersion:    "5.1.0",
			ObservedAt:     now,
		},
		State:     PlatformUpdateState{Platform: "beeper", LastKnownVersion: "4.3.200", LastKnownSpecVersion: "5.0.0"},
		CheckedAt: now,
	})
	if err != nil {
		t.Fatalf("VerifyAppliedUpdate: %v", err)
	}
	if !got.Compatible || got.RequiresRollback || got.MarkRequiresCodeChanges {
		t.Fatalf("minor spec bump with passing contract should be compatible: %+v", got)
	}
	if got.Compatibility != CompatibilityReview || !strings.Contains(got.ReviewNotice, "doc/beeper-api.md") {
		t.Fatalf("expected review notice for minor bump, got compat=%s notice=%q", got.Compatibility, got.ReviewNotice)
	}
}

func TestAdapterRevisionChangeClearsOldBlocksAndNewerVersionIsFresh(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 30, 0, 0, time.UTC)
	state := PlatformUpdateState{
		Platform:             "beeper",
		LastKnownVersion:     "4.3.200",
		LastKnownSpecVersion: "5.0.0",
		BlockedVersions: []BlockedVersion{
			{
				Version:         "4.3.220",
				AdapterRevision: "beeper-api-v5-r1",
				SpecVersion:     "6.0.0",
				Reasons:         []string{"spec major version 6"},
				MarkedAt:        now.Add(-2 * time.Hour),
			},
		},
	}

	sameRev := NewUpdatePolicy("beeper-api-v5-r1", 5, "5.0.0")
	freshCandidate, err := sameRev.AssessUpdate(AssessRequest{
		Mode:             UpdateModeManaged,
		CandidateVersion: "4.3.225",
		Observation: PlatformObservation{
			Platform:       "beeper",
			CurrentVersion: "4.3.200",
			SpecVersion:    "5.0.0",
			ObservedAt:     now,
		},
		State: state,
	})
	if err != nil {
		t.Fatalf("AssessUpdate fresh candidate: %v", err)
	}
	if !freshCandidate.ManagedUpdateAllowed || freshCandidate.CandidateBlocked {
		t.Fatalf("newer platform version should be evaluated fresh: %+v", freshCandidate)
	}

	upgradedRev := NewUpdatePolicy("beeper-api-v6-r1", 6, "6.0.0")
	cleared, err := upgradedRev.AssessUpdate(AssessRequest{
		Mode:             UpdateModeManaged,
		CandidateVersion: "4.3.220",
		Observation: PlatformObservation{
			Platform:       "beeper",
			CurrentVersion: "4.3.200",
			SpecVersion:    "5.0.0",
			ObservedAt:     now,
		},
		State: state,
	})
	if err != nil {
		t.Fatalf("AssessUpdate upgraded revision: %v", err)
	}
	if !cleared.ManagedUpdateAllowed || cleared.CandidateBlocked || len(cleared.ClearedBlockedVersions) != 1 || len(cleared.NextState.BlockedVersions) != 0 {
		t.Fatalf("adapter revision upgrade should clear old blocks: %+v", cleared)
	}
}

func TestForceBypassesBlockedVersionForManagedUpdate(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 35, 0, 0, time.UTC)
	policy := NewUpdatePolicy("beeper-api-v5-r1", 5, "5.0.0")
	state := PlatformUpdateState{
		Platform:             "beeper",
		LastKnownVersion:     "4.3.200",
		LastKnownSpecVersion: "5.0.0",
		BlockedVersions: []BlockedVersion{
			{
				Version:         "4.3.220",
				AdapterRevision: "beeper-api-v5-r1",
				Reasons:         []string{"contract check failed"},
				MarkedAt:        now.Add(-time.Hour),
			},
		},
	}
	got, err := policy.AssessUpdate(AssessRequest{
		Mode:             UpdateModeManaged,
		CandidateVersion: "4.3.220",
		Force:            true,
		Observation: PlatformObservation{
			Platform:       "beeper",
			CurrentVersion: "4.3.200",
			SpecVersion:    "5.0.0",
			ObservedAt:     now,
		},
		State: state,
	})
	if err != nil {
		t.Fatalf("AssessUpdate with force: %v", err)
	}
	if !got.ManagedUpdateAllowed || !got.CandidateBlocked || !got.ForceBypassedBlock {
		t.Fatalf("force should bypass blocked version: %+v", got)
	}
}
