package query

import (
	"context"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type PlatformUpdateStatus struct {
	Platform port.PlatformUpdateReader
	State    port.PlatformStateReader
}

func (s PlatformUpdateStatus) Check(ctx context.Context, replyIssues []string, repliesChecked int) (domain.Assessment, error) {
	return s.assess(ctx, domain.UpdateModeObserve, "", false, replyIssues, repliesChecked)
}

func (s PlatformUpdateStatus) AssessManaged(ctx context.Context, candidateVersion string, force bool, replyIssues []string, repliesChecked int) (domain.Assessment, error) {
	return s.assess(ctx, domain.UpdateModeManaged, candidateVersion, force, replyIssues, repliesChecked)
}

func (s PlatformUpdateStatus) ReadState(ctx context.Context, platform string) (domain.PlatformUpdateState, error) {
	return s.State.Load(ctx, platform)
}

func (s PlatformUpdateStatus) assess(ctx context.Context, mode domain.UpdateMode, candidateVersion string, force bool, replyIssues []string, repliesChecked int) (domain.Assessment, error) {
	state, err := s.State.Load(ctx, s.Platform.PlatformName())
	if err != nil {
		return domain.Assessment{}, err
	}
	obs, err := s.Platform.ObservePlatform(ctx)
	if err != nil {
		return domain.Assessment{}, err
	}
	obs.SentReplyIssues = append(obs.SentReplyIssues, replyIssues...)
	obs.SentRepliesChecked = repliesChecked
	policy := domain.NewUpdatePolicy(s.Platform.AdapterRevision(), s.Platform.ExpectedSpecMajor(), s.Platform.ExpectedSpecVersion())
	return policy.AssessUpdate(domain.AssessRequest{
		Mode:             mode,
		Observation:      obs,
		State:            state,
		CandidateVersion: candidateVersion,
		Force:            force,
	})
}
