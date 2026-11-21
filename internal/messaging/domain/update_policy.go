package domain

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

type UpdateSource string

const (
	PendingSourceNone     UpdateSource = ""
	PendingSourceCache    UpdateSource = "updater-cache"
	PendingSourceDownload UpdateSource = "download"
)

type CompatibilityStatus string

const (
	CompatibilityOK           CompatibilityStatus = "compatible"
	CompatibilityReview       CompatibilityStatus = "compatible_review_recommended"
	CompatibilityIncompatible CompatibilityStatus = "incompatible_requires_code_changes"
	CompatibilityUnknown      CompatibilityStatus = "unknown"
)

type UpdateMode string

const (
	UpdateModeObserve UpdateMode = "observe"
	UpdateModeManaged UpdateMode = "managed"
)

type PendingUpdate struct {
	Version           string
	FileName          string
	FilePath          string
	SHA512            string
	Verified          bool
	VerificationError string
	Source            UpdateSource
}

type PlatformObservation struct {
	Platform           string
	CurrentVersion     string
	HeaderVersion      string
	SpecVersion        string
	ContractIssues     []string
	SentReplyIssues    []string
	SentRepliesChecked int
	ConnectionError    string
	Pending            *PendingUpdate
	ObservedAt         time.Time
}

type BlockedVersion struct {
	Version         string
	AdapterRevision string
	SpecVersion     string
	Source          string
	Reasons         []string
	MarkedAt        time.Time
}

type PlatformUpdateState struct {
	Platform                 string
	LastKnownVersion         string
	LastKnownSpecVersion     string
	LastManagedVersion       string
	LastExternalDriftVersion string
	LastCheckedAt            time.Time
	BlockedVersions          []BlockedVersion
}

type AssessRequest struct {
	Mode             UpdateMode
	Observation      PlatformObservation
	State            PlatformUpdateState
	CandidateVersion string
	Force            bool
}

type Assessment struct {
	Platform               string
	CurrentVersion         string
	SpecVersion            string
	ExpectedSpecMajor      int
	ExpectedSpecVersion    string
	AdapterRevision        string
	Compatibility          CompatibilityStatus
	IncompatibleReasons    []string
	ReviewNotice           string
	Pending                *PendingUpdate
	ExternalUpdateDetected bool
	AutoInstallAllowed     bool
	KeepRunning            bool
	RequiresCodeChanges    bool
	ManagedUpdateAllowed   bool
	CandidateVersion       string
	CandidateBlocked       bool
	BlockedBy              *BlockedVersion
	ForceBypassedBlock     bool
	RefusalReason          string
	ActiveBlockedVersions  []BlockedVersion
	ClearedBlockedVersions []BlockedVersion
	ConnectionError        string
	NextState              PlatformUpdateState
}

type VerifyRequest struct {
	PreviousVersion string
	TargetVersion   string
	Observation     PlatformObservation
	State           PlatformUpdateState
	CheckedAt       time.Time
}

type Verification struct {
	Platform                string
	PreviousVersion         string
	TargetVersion           string
	CurrentVersion          string
	SpecVersion             string
	AdapterRevision         string
	Compatible              bool
	Compatibility           CompatibilityStatus
	IncompatibleReasons     []string
	ReviewNotice            string
	RequiresRollback        bool
	RollbackToVersion       string
	MarkRequiresCodeChanges bool
	BlockedEntry            *BlockedVersion
	NextState               PlatformUpdateState
}

type UpdatePolicy struct {
	AdapterRevision     string
	ExpectedSpecMajor   int
	ExpectedSpecVersion string
}

func NewUpdatePolicy(adapterRevision string, expectedSpecMajor int, expectedSpecVersion string) UpdatePolicy {
	return UpdatePolicy{
		AdapterRevision:     adapterRevision,
		ExpectedSpecMajor:   expectedSpecMajor,
		ExpectedSpecVersion: expectedSpecVersion,
	}
}

func ParseSemver(raw string) (major, minor, patch int, ok bool) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "v")
	if cleaned == "" {
		return 0, 0, 0, false
	}
	if cut := strings.IndexAny(cleaned, "-+"); cut >= 0 {
		cleaned = cleaned[:cut]
	}
	parts := strings.Split(cleaned, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, false
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil || maj < 0 {
		return 0, 0, 0, false
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil || min < 0 {
		return 0, 0, 0, false
	}
	pat := 0
	if len(parts) == 3 {
		pat, err = strconv.Atoi(parts[2])
		if err != nil || pat < 0 {
			return 0, 0, 0, false
		}
	}
	return maj, min, pat, true
}

func (p UpdatePolicy) AssessUpdate(req AssessRequest) (Assessment, error) {
	obs := req.Observation
	platformName := firstNonEmpty(obs.Platform, req.State.Platform)
	activeBlocked, clearedBlocked := p.partitionBlocked(req.State.BlockedVersions)

	next := clonePlatformState(req.State)
	next.Platform = platformName
	next.BlockedVersions = slices.Clone(activeBlocked)
	if !obs.ObservedAt.IsZero() {
		next.LastCheckedAt = obs.ObservedAt
	}

	compat, reasons, reviewNotice := p.evaluateCompatibility(obs, req.State.LastKnownSpecVersion)
	externalDetected := obs.ConnectionError == "" &&
		req.State.LastKnownVersion != "" &&
		obs.CurrentVersion != "" &&
		obs.CurrentVersion != req.State.LastKnownVersion

	candidate := strings.TrimSpace(req.CandidateVersion)
	if candidate == "" && obs.Pending != nil {
		candidate = strings.TrimSpace(obs.Pending.Version)
	}

	var blockedBy *BlockedVersion
	if candidate != "" {
		if match, found := findBlocked(activeBlocked, candidate); found {
			copyMatch := cloneBlockedVersion(match)
			blockedBy = &copyMatch
		}
	}

	res := Assessment{
		Platform:               platformName,
		CurrentVersion:         obs.CurrentVersion,
		SpecVersion:            obs.SpecVersion,
		ExpectedSpecMajor:      p.ExpectedSpecMajor,
		ExpectedSpecVersion:    p.ExpectedSpecVersion,
		AdapterRevision:        p.AdapterRevision,
		Compatibility:          compat,
		IncompatibleReasons:    reasons,
		ReviewNotice:           reviewNotice,
		Pending:                clonePending(obs.Pending),
		ExternalUpdateDetected: externalDetected,
		AutoInstallAllowed:     false,
		KeepRunning:            true,
		RequiresCodeChanges:    compat == CompatibilityIncompatible,
		ManagedUpdateAllowed:   false,
		CandidateVersion:       candidate,
		CandidateBlocked:       blockedBy != nil,
		BlockedBy:              blockedBy,
		ActiveBlockedVersions:  activeBlocked,
		ClearedBlockedVersions: clearedBlocked,
		ConnectionError:        obs.ConnectionError,
	}

	if req.Mode == UpdateModeObserve {
		if obs.ConnectionError == "" {
			if compat == CompatibilityIncompatible {
				res.RequiresCodeChanges = true
				if obs.CurrentVersion != "" {
					next.LastExternalDriftVersion = obs.CurrentVersion
					entry := BlockedVersion{
						Version:         obs.CurrentVersion,
						AdapterRevision: p.AdapterRevision,
						SpecVersion:     obs.SpecVersion,
						Source:          "external",
						Reasons:         slices.Clone(reasons),
						MarkedAt:        obs.ObservedAt,
					}
					next.BlockedVersions = upsertBlockedVersion(next.BlockedVersions, entry)
					res.ActiveBlockedVersions = slices.Clone(next.BlockedVersions)
				}
			} else if obs.CurrentVersion != "" {
				next.LastKnownVersion = obs.CurrentVersion
				if obs.SpecVersion != "" {
					next.LastKnownSpecVersion = obs.SpecVersion
				}
				next.LastExternalDriftVersion = ""
				next.BlockedVersions = removeBlockedVersion(next.BlockedVersions, obs.CurrentVersion)
				res.ActiveBlockedVersions = slices.Clone(next.BlockedVersions)
			}
		}
	}

	if req.Mode == UpdateModeManaged {
		switch {
		case candidate == "":
			res.RefusalReason = "no candidate platform version specified"
		case obs.Pending != nil && candidate == obs.Pending.Version && obs.Pending.VerificationError != "":
			res.RefusalReason = obs.Pending.VerificationError
		case obs.Pending != nil && candidate == obs.Pending.Version && !obs.Pending.Verified:
			res.RefusalReason = fmt.Sprintf("pending update %s has not passed integrity verification", candidate)
		case blockedBy != nil && !req.Force:
			res.RequiresCodeChanges = true
			res.RefusalReason = formatBlockedRefusal(*blockedBy)
		case blockedBy != nil && req.Force:
			res.ManagedUpdateAllowed = true
			res.ForceBypassedBlock = true
		default:
			res.ManagedUpdateAllowed = true
		}
	}

	res.NextState = next

	if err := errors.Join(
		p.AssertNoUnattendedPlatformUpdates(req, res),
		p.AssertBlockedVersionRefusedForManagedUpdate(req, res),
		p.AssertExternalIncompatibleUpdateKeepsRunning(req, res),
	); err != nil {
		return Assessment{}, err
	}
	return res, nil
}

func (p UpdatePolicy) VerifyAppliedUpdate(req VerifyRequest) (Verification, error) {
	obs := req.Observation
	platformName := firstNonEmpty(obs.Platform, req.State.Platform)
	checkedAt := req.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = obs.ObservedAt
	}

	activeBlocked, _ := p.partitionBlocked(req.State.BlockedVersions)
	next := clonePlatformState(req.State)
	next.Platform = platformName
	next.BlockedVersions = slices.Clone(activeBlocked)
	if !checkedAt.IsZero() {
		next.LastCheckedAt = checkedAt
	}

	var reasons []string
	var reviewNotice string
	compat := CompatibilityOK

	if obs.ConnectionError != "" {
		reasons = append(reasons, fmt.Sprintf("platform API unreachable after update: %s", obs.ConnectionError))
	} else {
		reasons = append(reasons, obs.ContractIssues...)
		reasons = append(reasons, obs.SentReplyIssues...)
		if p.ExpectedSpecMajor > 0 && strings.TrimSpace(obs.SpecVersion) == "" {
			reasons = append(reasons, "API spec info.version is missing")
		} else if obs.SpecVersion != "" {
			major, _, _, ok := ParseSemver(obs.SpecVersion)
			if !ok {
				reasons = append(reasons, fmt.Sprintf("API spec version %q could not be parsed", obs.SpecVersion))
			} else if p.ExpectedSpecMajor > 0 && major != p.ExpectedSpecMajor {
				reasons = append(reasons, fmt.Sprintf("API spec major version %d differs from expected major version %d (observed %s)", major, p.ExpectedSpecMajor, obs.SpecVersion))
			}
		}
		if req.TargetVersion != "" && obs.CurrentVersion != "" && obs.CurrentVersion != req.TargetVersion {
			reasons = append(reasons, fmt.Sprintf("expected platform version %s after update, observed %s", req.TargetVersion, obs.CurrentVersion))
		}
	}

	target := firstNonEmpty(req.TargetVersion, obs.CurrentVersion)
	res := Verification{
		Platform:        platformName,
		PreviousVersion: req.PreviousVersion,
		TargetVersion:   req.TargetVersion,
		CurrentVersion:  obs.CurrentVersion,
		SpecVersion:     obs.SpecVersion,
		AdapterRevision: p.AdapterRevision,
	}

	if len(reasons) == 0 {
		if p.hasMinorOrPatchBump(obs.SpecVersion, req.State.LastKnownSpecVersion) {
			compat = CompatibilityReview
			reviewNotice = fmt.Sprintf("API spec changed within major version %d (%s); review doc/beeper-api.md and desktop-api-go.", p.ExpectedSpecMajor, obs.SpecVersion)
		}
		res.Compatible = true
		res.Compatibility = compat
		res.ReviewNotice = reviewNotice
		if target != "" {
			next.BlockedVersions = removeBlockedVersion(next.BlockedVersions, target)
			next.LastKnownVersion = target
			next.LastManagedVersion = target
		}
		if obs.SpecVersion != "" {
			next.LastKnownSpecVersion = obs.SpecVersion
		}
		next.LastExternalDriftVersion = ""
	} else {
		res.Compatible = false
		res.Compatibility = CompatibilityIncompatible
		res.IncompatibleReasons = reasons
		res.RequiresRollback = true
		res.RollbackToVersion = req.PreviousVersion
		res.MarkRequiresCodeChanges = true

		blockedVersion := firstNonEmpty(target, "unknown")
		entry := BlockedVersion{
			Version:         blockedVersion,
			AdapterRevision: p.AdapterRevision,
			SpecVersion:     obs.SpecVersion,
			Source:          "managed",
			Reasons:         slices.Clone(reasons),
			MarkedAt:        checkedAt,
		}
		res.BlockedEntry = &entry
		next.BlockedVersions = upsertBlockedVersion(next.BlockedVersions, entry)
		if req.PreviousVersion != "" {
			next.LastKnownVersion = req.PreviousVersion
		}
	}

	res.NextState = next

	if err := errors.Join(
		p.AssertIncompatibleManagedUpdateRequiresRollback(res),
		p.AssertIncompatibleUpdateRequiresCodeChanges(res),
	); err != nil {
		return Verification{}, err
	}
	return res, nil
}

func (p UpdatePolicy) RecordFailedAttempt(state PlatformUpdateState, previousVersion, failedVersion, specVersion string, reasons []string, at time.Time) PlatformUpdateState {
	activeBlocked, _ := p.partitionBlocked(state.BlockedVersions)
	next := clonePlatformState(state)
	next.BlockedVersions = slices.Clone(activeBlocked)
	if !at.IsZero() {
		next.LastCheckedAt = at
	}
	if previousVersion != "" {
		next.LastKnownVersion = previousVersion
		next.LastManagedVersion = previousVersion
	}
	if failedVersion != "" {
		existing, found := findBlocked(next.BlockedVersions, failedVersion)
		recordedReasons := slices.Clone(reasons)
		if len(recordedReasons) == 0 && found && len(existing.Reasons) > 0 {
			recordedReasons = slices.Clone(existing.Reasons)
		}
		if len(recordedReasons) == 0 {
			recordedReasons = []string{"managed platform update verification failed"}
		}
		recordedSpec := firstNonEmpty(specVersion, existing.SpecVersion)
		next.BlockedVersions = upsertBlockedVersion(next.BlockedVersions, BlockedVersion{
			Version:         failedVersion,
			AdapterRevision: p.AdapterRevision,
			SpecVersion:     recordedSpec,
			Source:          "managed",
			Reasons:         recordedReasons,
			MarkedAt:        at,
		})
	}
	return next
}

func (p UpdatePolicy) AssertNoUnattendedPlatformUpdates(req AssessRequest, res Assessment) error {
	if res.AutoInstallAllowed {
		return errors.New("unattended platform updates are not allowed")
	}
	if req.Mode == UpdateModeObserve && res.ManagedUpdateAllowed {
		return errors.New("observe mode must not authorize managed update installation")
	}
	if req.Mode == UpdateModeObserve &&
		req.Observation.Pending != nil &&
		req.Observation.Pending.Version != "" &&
		req.State.LastKnownVersion != "" &&
		res.NextState.LastKnownVersion == req.Observation.Pending.Version &&
		req.Observation.CurrentVersion != req.Observation.Pending.Version {
		return errors.New("pending update must wait for explicit operator action")
	}
	return nil
}

func (p UpdatePolicy) AssertIncompatibleManagedUpdateRequiresRollback(res Verification) error {
	if !res.Compatible && !res.RequiresRollback {
		return errors.New("incompatible managed update must require rollback")
	}
	return nil
}

func (p UpdatePolicy) AssertIncompatibleUpdateRequiresCodeChanges(res Verification) error {
	if !res.Compatible && (!res.MarkRequiresCodeChanges || res.BlockedEntry == nil) {
		return errors.New("incompatible platform update must be marked as requiring code changes")
	}
	return nil
}

func (p UpdatePolicy) AssertBlockedVersionRefusedForManagedUpdate(req AssessRequest, res Assessment) error {
	if req.Mode == UpdateModeManaged && res.CandidateBlocked && !req.Force && res.ManagedUpdateAllowed {
		return errors.New("blocked platform version must be refused for managed update without force")
	}
	return nil
}

func (p UpdatePolicy) AssertExternalIncompatibleUpdateKeepsRunning(req AssessRequest, res Assessment) error {
	if req.Mode == UpdateModeObserve && res.Compatibility == CompatibilityIncompatible {
		if !res.KeepRunning || !res.RequiresCodeChanges {
			return errors.New("external incompatible update must keep GenTalent running while warning that code changes are required")
		}
	}
	return nil
}

func (p UpdatePolicy) evaluateCompatibility(obs PlatformObservation, lastKnownSpec string) (CompatibilityStatus, []string, string) {
	if obs.ConnectionError != "" {
		return CompatibilityUnknown, nil, ""
	}
	var reasons []string
	reasons = append(reasons, obs.ContractIssues...)
	reasons = append(reasons, obs.SentReplyIssues...)
	if obs.SpecVersion != "" {
		major, _, _, ok := ParseSemver(obs.SpecVersion)
		if !ok {
			reasons = append(reasons, fmt.Sprintf("API spec version %q could not be parsed", obs.SpecVersion))
		} else if p.ExpectedSpecMajor > 0 && major != p.ExpectedSpecMajor {
			reasons = append(reasons, fmt.Sprintf("API spec major version %d differs from expected major version %d (observed %s)", major, p.ExpectedSpecMajor, obs.SpecVersion))
		}
	}
	if len(reasons) > 0 {
		return CompatibilityIncompatible, reasons, ""
	}
	if p.hasMinorOrPatchBump(obs.SpecVersion, lastKnownSpec) {
		notice := fmt.Sprintf("API spec changed within major version %d (%s); review doc/beeper-api.md and desktop-api-go.", p.ExpectedSpecMajor, obs.SpecVersion)
		return CompatibilityReview, nil, notice
	}
	return CompatibilityOK, nil, ""
}

func (p UpdatePolicy) hasMinorOrPatchBump(observed, lastKnownSpec string) bool {
	if observed == "" {
		return false
	}
	obsMaj, obsMin, obsPat, obsOK := ParseSemver(observed)
	if !obsOK {
		return false
	}
	for _, baseline := range []string{p.ExpectedSpecVersion, lastKnownSpec} {
		if baseline == "" {
			continue
		}
		baseMaj, baseMin, basePat, baseOK := ParseSemver(baseline)
		if baseOK && obsMaj == baseMaj && (obsMin != baseMin || obsPat != basePat) {
			return true
		}
	}
	return false
}

func (p UpdatePolicy) partitionBlocked(entries []BlockedVersion) (active, cleared []BlockedVersion) {
	for _, entry := range entries {
		cloned := cloneBlockedVersion(entry)
		if entry.AdapterRevision == p.AdapterRevision {
			active = append(active, cloned)
		} else {
			cleared = append(cleared, cloned)
		}
	}
	return active, cleared
}

func formatBlockedRefusal(blocked BlockedVersion) string {
	detail := strings.Join(blocked.Reasons, "; ")
	if detail == "" {
		detail = "previous compatibility check failed"
	}
	return fmt.Sprintf("platform version %s requires code changes for adapter revision %s (%s); pass --force to override", blocked.Version, blocked.AdapterRevision, detail)
}

func clonePlatformState(state PlatformUpdateState) PlatformUpdateState {
	cloned := state
	cloned.BlockedVersions = make([]BlockedVersion, len(state.BlockedVersions))
	for i, entry := range state.BlockedVersions {
		cloned.BlockedVersions[i] = cloneBlockedVersion(entry)
	}
	return cloned
}

func cloneBlockedVersion(entry BlockedVersion) BlockedVersion {
	cloned := entry
	cloned.Reasons = slices.Clone(entry.Reasons)
	return cloned
}

func clonePending(pending *PendingUpdate) *PendingUpdate {
	if pending == nil {
		return nil
	}
	copied := *pending
	return &copied
}

func findBlocked(entries []BlockedVersion, version string) (BlockedVersion, bool) {
	for _, entry := range entries {
		if entry.Version == version {
			return entry, true
		}
	}
	return BlockedVersion{}, false
}

func upsertBlockedVersion(entries []BlockedVersion, entry BlockedVersion) []BlockedVersion {
	cloned := cloneBlockedVersion(entry)
	for i, existing := range entries {
		if existing.Version == entry.Version && existing.AdapterRevision == entry.AdapterRevision {
			updated := slices.Clone(entries)
			updated[i] = cloned
			return updated
		}
	}
	return append(slices.Clone(entries), cloned)
}

func removeBlockedVersion(entries []BlockedVersion, version string) []BlockedVersion {
	if version == "" || len(entries) == 0 {
		return entries
	}
	var kept []BlockedVersion
	for _, entry := range entries {
		if entry.Version != version {
			kept = append(kept, cloneBlockedVersion(entry))
		}
	}
	return kept
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
