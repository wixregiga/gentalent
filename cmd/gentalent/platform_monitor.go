package main

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	mcommand "gentalent/internal/messaging/app/command"
	mdomain "gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

const defaultPlatformCheckInterval = 15 * time.Minute

type monitorLogState struct {
	currentVersion       string
	specVersion          string
	compatibility        mdomain.CompatibilityStatus
	requiresCodeChanges  bool
	externalDriftVersion string
	pendingVersion       string
	pendingError         string
	connectionError      string
}

type platformMonitor struct {
	mu                sync.Mutex
	coordinator       mcommand.PlatformUpdateCoordinator
	headers           []port.HeaderVersionReader
	checkReplies      func(context.Context) (int, []string, error)
	checkEvery        time.Duration
	now               func() time.Time
	log               *slog.Logger
	inFlight          bool
	checkedOnce       bool
	lastCheckAt       time.Time
	lastHeaderVersion string
	hasSnapshot       bool
	lastSnapshot      monitorLogState
	lastError         string
}

func newPlatformMonitor(
	coordinator mcommand.PlatformUpdateCoordinator,
	headers []port.HeaderVersionReader,
	checkReplies func(context.Context) (int, []string, error),
	checkEvery time.Duration,
	log *slog.Logger,
) *platformMonitor {
	if checkEvery <= 0 {
		checkEvery = defaultPlatformCheckInterval
	}
	return &platformMonitor{
		coordinator:  coordinator,
		headers:      headers,
		checkReplies: checkReplies,
		checkEvery:   checkEvery,
		now:          time.Now,
		log:          log,
	}
}

func (m *platformMonitor) tick(ctx context.Context) {
	if m == nil || ctx.Err() != nil {
		return
	}
	m.mu.Lock()
	if m.inFlight {
		m.mu.Unlock()
		return
	}
	observedHeader := m.currentHeaderVersion()
	headerChanged := observedHeader != "" && m.lastHeaderVersion != "" && observedHeader != m.lastHeaderVersion
	if observedHeader != "" && m.lastHeaderVersion == "" && m.checkedOnce {
		headerChanged = true
	}
	due := !m.checkedOnce || headerChanged || m.nowTime().Sub(m.lastCheckAt) >= m.checkEvery
	if !due {
		m.mu.Unlock()
		return
	}
	m.inFlight = true
	m.mu.Unlock()

	var replyIssues []string
	var repliesChecked int
	var replyErr error
	if m.checkReplies != nil {
		repliesChecked, replyIssues, replyErr = m.checkReplies(ctx)
	}
	var assessment mdomain.Assessment
	var err error
	if replyErr != nil {
		err = replyErr
	} else {
		assessment, err = m.coordinator.ObserveAndRecord(ctx, replyIssues, repliesChecked)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.inFlight = false
	m.checkedOnce = true
	m.lastCheckAt = m.nowTime()

	if err != nil {
		if m.lastError != err.Error() {
			m.lastError = err.Error()
			m.log.WarnContext(ctx, "platform update check failed", "error", err)
		}
		return
	}
	m.lastError = ""

	if assessment.CurrentVersion != "" {
		m.lastHeaderVersion = assessment.CurrentVersion
	} else if observedHeader != "" {
		m.lastHeaderVersion = observedHeader
	}

	next := snapshotFromAssessment(assessment)
	m.logTransition(ctx, next, assessment)
	m.lastSnapshot = next
	m.hasSnapshot = true
}

func (m *platformMonitor) currentHeaderVersion() string {
	first := ""
	for _, reader := range m.headers {
		if reader == nil {
			continue
		}
		version := strings.TrimSpace(reader.LastObservedVersion())
		if version == "" {
			continue
		}
		if m.lastHeaderVersion != "" && version != m.lastHeaderVersion {
			return version
		}
		if first == "" {
			first = version
		}
	}
	return first
}

func (m *platformMonitor) nowTime() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

func snapshotFromAssessment(a mdomain.Assessment) monitorLogState {
	state := monitorLogState{
		currentVersion:       a.CurrentVersion,
		specVersion:          a.SpecVersion,
		compatibility:        a.Compatibility,
		requiresCodeChanges:  a.RequiresCodeChanges,
		externalDriftVersion: a.NextState.LastExternalDriftVersion,
		connectionError:      a.ConnectionError,
	}
	if a.Pending != nil {
		state.pendingVersion = a.Pending.Version
		state.pendingError = a.Pending.VerificationError
	}
	return state
}

func (m *platformMonitor) logTransition(ctx context.Context, next monitorLogState, a mdomain.Assessment) {
	compatChanged := !m.hasSnapshot ||
		next.currentVersion != m.lastSnapshot.currentVersion ||
		next.specVersion != m.lastSnapshot.specVersion ||
		next.compatibility != m.lastSnapshot.compatibility ||
		next.requiresCodeChanges != m.lastSnapshot.requiresCodeChanges ||
		next.externalDriftVersion != m.lastSnapshot.externalDriftVersion ||
		next.connectionError != m.lastSnapshot.connectionError

	if compatChanged {
		switch {
		case a.ConnectionError != "":
			m.log.WarnContext(ctx, "platform update check could not reach platform API",
				"platform", a.Platform,
				"error", a.ConnectionError,
			)
		case a.Compatibility == mdomain.CompatibilityIncompatible || a.RequiresCodeChanges:
			m.log.WarnContext(ctx, "platform update requires code changes; GenTalent keeps running",
				"platform", a.Platform,
				"version", a.CurrentVersion,
				"spec_version", a.SpecVersion,
				"external_update_detected", a.ExternalUpdateDetected,
				"reasons", strings.Join(a.IncompatibleReasons, "; "),
			)
		case a.Compatibility == mdomain.CompatibilityReview:
			m.log.InfoContext(ctx, "platform API minor or patch update detected; review recommended",
				"platform", a.Platform,
				"version", a.CurrentVersion,
				"spec_version", a.SpecVersion,
				"notice", a.ReviewNotice,
			)
		default:
			m.log.InfoContext(ctx, "platform compatibility verified",
				"platform", a.Platform,
				"version", a.CurrentVersion,
				"spec_version", a.SpecVersion,
				"adapter_revision", a.AdapterRevision,
			)
		}
	}

	pendingChanged := (!m.hasSnapshot ||
		next.pendingVersion != m.lastSnapshot.pendingVersion ||
		next.pendingError != m.lastSnapshot.pendingError) &&
		(next.pendingVersion != "" || next.pendingError != "")

	if pendingChanged && a.Pending != nil {
		if a.Pending.VerificationError != "" {
			m.log.WarnContext(ctx, "pending platform update failed verification",
				"platform", a.Platform,
				"pending_version", a.Pending.Version,
				"error", a.Pending.VerificationError,
			)
		} else {
			m.log.InfoContext(ctx, "pending platform update waiting for operator action",
				"platform", a.Platform,
				"current_version", a.CurrentVersion,
				"pending_version", a.Pending.Version,
			)
		}
	}
}
