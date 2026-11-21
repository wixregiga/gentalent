package command

import (
	"context"
	"slices"
	"time"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type PlatformUpdateCoordinator struct {
	Platform port.PlatformUpdateReader
	State    port.PlatformStateStore
	Now      func() time.Time
}

func (c PlatformUpdateCoordinator) ObserveAndRecord(ctx context.Context, replyIssues []string, repliesChecked int) (domain.Assessment, error) {
	state, err := c.State.Load(ctx, c.Platform.PlatformName())
	if err != nil {
		return domain.Assessment{}, err
	}
	obs, err := c.Platform.ObservePlatform(ctx)
	if err != nil {
		return domain.Assessment{}, err
	}
	obs.SentReplyIssues = append(obs.SentReplyIssues, replyIssues...)
	obs.SentRepliesChecked = repliesChecked
	if obs.ObservedAt.IsZero() {
		obs.ObservedAt = c.now()
	}

	policy := c.policy()
	assessment, err := policy.AssessUpdate(domain.AssessRequest{
		Mode:        domain.UpdateModeObserve,
		Observation: obs,
		State:       state,
	})
	if err != nil {
		return domain.Assessment{}, err
	}
	if stateChanged(state, assessment.NextState) {
		if err := c.State.Save(ctx, assessment.NextState); err != nil {
			return domain.Assessment{}, err
		}
	}
	return assessment, nil
}

func (c PlatformUpdateCoordinator) AssessManagedUpdate(ctx context.Context, candidateVersion string, force bool, replyIssues []string, repliesChecked int) (domain.Assessment, error) {
	state, err := c.State.Load(ctx, c.Platform.PlatformName())
	if err != nil {
		return domain.Assessment{}, err
	}
	obs, err := c.Platform.ObservePlatform(ctx)
	if err != nil {
		return domain.Assessment{}, err
	}
	obs.SentReplyIssues = append(obs.SentReplyIssues, replyIssues...)
	obs.SentRepliesChecked = repliesChecked
	if obs.ObservedAt.IsZero() {
		obs.ObservedAt = c.now()
	}

	policy := c.policy()
	assessment, err := policy.AssessUpdate(domain.AssessRequest{
		Mode:             domain.UpdateModeManaged,
		Observation:      obs,
		State:            state,
		CandidateVersion: candidateVersion,
		Force:            force,
	})
	if err != nil {
		return domain.Assessment{}, err
	}
	if len(assessment.ClearedBlockedVersions) > 0 {
		if err := c.State.Save(ctx, assessment.NextState); err != nil {
			return domain.Assessment{}, err
		}
	}
	return assessment, nil
}

func (c PlatformUpdateCoordinator) VerifyManagedUpdate(ctx context.Context, previousVersion, targetVersion string, replyIssues []string, repliesChecked int) (domain.Verification, error) {
	state, err := c.State.Load(ctx, c.Platform.PlatformName())
	if err != nil {
		return domain.Verification{}, err
	}
	obs, err := c.Platform.ObservePlatform(ctx)
	if err != nil {
		return domain.Verification{}, err
	}
	obs.SentReplyIssues = append(obs.SentReplyIssues, replyIssues...)
	obs.SentRepliesChecked = repliesChecked
	checkedAt := obs.ObservedAt
	if checkedAt.IsZero() {
		checkedAt = c.now()
	}

	policy := c.policy()
	verification, err := policy.VerifyAppliedUpdate(domain.VerifyRequest{
		PreviousVersion: previousVersion,
		TargetVersion:   targetVersion,
		Observation:     obs,
		State:           state,
		CheckedAt:       checkedAt,
	})
	if err != nil {
		return domain.Verification{}, err
	}
	if err := c.State.Save(ctx, verification.NextState); err != nil {
		return domain.Verification{}, err
	}
	return verification, nil
}

func (c PlatformUpdateCoordinator) RecordRollback(ctx context.Context, previousVersion, failedVersion, specVersion string, reasons []string) (domain.PlatformUpdateState, error) {
	state, err := c.State.Load(ctx, c.Platform.PlatformName())
	if err != nil {
		return domain.PlatformUpdateState{}, err
	}
	state.Platform = c.Platform.PlatformName()
	next := c.policy().RecordFailedAttempt(state, previousVersion, failedVersion, specVersion, reasons, c.now())
	if err := c.State.Save(ctx, next); err != nil {
		return domain.PlatformUpdateState{}, err
	}
	return next, nil
}

func (c PlatformUpdateCoordinator) policy() domain.UpdatePolicy {
	return domain.NewUpdatePolicy(c.Platform.AdapterRevision(), c.Platform.ExpectedSpecMajor(), c.Platform.ExpectedSpecVersion())
}

func (c PlatformUpdateCoordinator) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().UTC()
}

func stateChanged(prev, next domain.PlatformUpdateState) bool {
	return prev.Platform != next.Platform ||
		prev.LastKnownVersion != next.LastKnownVersion ||
		prev.LastKnownSpecVersion != next.LastKnownSpecVersion ||
		prev.LastManagedVersion != next.LastManagedVersion ||
		prev.LastExternalDriftVersion != next.LastExternalDriftVersion ||
		!slices.EqualFunc(prev.BlockedVersions, next.BlockedVersions, blockedVersionsEqual)
}

func blockedVersionsEqual(a, b domain.BlockedVersion) bool {
	return a.Version == b.Version &&
		a.AdapterRevision == b.AdapterRevision &&
		a.SpecVersion == b.SpecVersion &&
		a.Source == b.Source &&
		slices.Equal(a.Reasons, b.Reasons)
}
