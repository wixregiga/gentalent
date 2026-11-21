package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	mcommand "gentalent/internal/messaging/app/command"
	mquery "gentalent/internal/messaging/app/query"
	mdomain "gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/infra/beeper"
	mfiles "gentalent/internal/messaging/infra/files"
	"gentalent/internal/shared/sqlitedb"
)

type assessmentJSON struct {
	Platform               string               `json:"platform"`
	CurrentVersion         string               `json:"current_version,omitempty"`
	SpecVersion            string               `json:"spec_version,omitempty"`
	ExpectedSpecMajor      int                  `json:"expected_spec_major"`
	ExpectedSpecVersion    string               `json:"expected_spec_version,omitempty"`
	AdapterRevision        string               `json:"adapter_revision"`
	Compatibility          string               `json:"compatibility"`
	IncompatibleReasons    []string             `json:"incompatible_reasons,omitempty"`
	ReviewNotice           string               `json:"review_notice,omitempty"`
	Pending                *pendingUpdateJSON   `json:"pending,omitempty"`
	ExternalUpdateDetected bool                 `json:"external_update_detected"`
	AutoInstallAllowed     bool                 `json:"auto_install_allowed"`
	KeepRunning            bool                 `json:"keep_running"`
	RequiresCodeChanges    bool                 `json:"requires_code_changes"`
	ManagedUpdateAllowed   bool                 `json:"managed_update_allowed"`
	CandidateVersion       string               `json:"candidate_version,omitempty"`
	CandidateBlocked       bool                 `json:"candidate_blocked"`
	BlockedBy              *blockedVersionJSON  `json:"blocked_by,omitempty"`
	ForceBypassedBlock     bool                 `json:"force_bypassed_block"`
	RefusalReason          string               `json:"refusal_reason,omitempty"`
	ActiveBlockedVersions  []blockedVersionJSON `json:"active_blocked_versions,omitempty"`
	ClearedBlockedVersions []blockedVersionJSON `json:"cleared_blocked_versions,omitempty"`
	ConnectionError        string               `json:"connection_error,omitempty"`
}

type pendingUpdateJSON struct {
	Version           string `json:"version,omitempty"`
	FileName          string `json:"file_name,omitempty"`
	FilePath          string `json:"file_path,omitempty"`
	SHA512            string `json:"sha512,omitempty"`
	Verified          bool   `json:"verified"`
	VerificationError string `json:"verification_error,omitempty"`
	Source            string `json:"source,omitempty"`
}

type blockedVersionJSON struct {
	Version         string   `json:"version"`
	AdapterRevision string   `json:"adapter_revision"`
	SpecVersion     string   `json:"spec_version,omitempty"`
	Source          string   `json:"source,omitempty"`
	Reasons         []string `json:"reasons,omitempty"`
	MarkedAt        string   `json:"marked_at,omitempty"`
}

type verificationJSON struct {
	Platform                string              `json:"platform"`
	PreviousVersion         string              `json:"previous_version,omitempty"`
	TargetVersion           string              `json:"target_version,omitempty"`
	CurrentVersion          string              `json:"current_version,omitempty"`
	SpecVersion             string              `json:"spec_version,omitempty"`
	AdapterRevision         string              `json:"adapter_revision"`
	Compatible              bool                `json:"compatible"`
	Compatibility           string              `json:"compatibility"`
	IncompatibleReasons     []string            `json:"incompatible_reasons,omitempty"`
	ReviewNotice            string              `json:"review_notice,omitempty"`
	RequiresRollback        bool                `json:"requires_rollback"`
	RollbackToVersion       string              `json:"rollback_to_version,omitempty"`
	MarkRequiresCodeChanges bool                `json:"mark_requires_code_changes"`
	BlockedEntry            *blockedVersionJSON `json:"blocked_entry,omitempty"`
}

func checkUpdates(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "check-updates")
	platformFlag := flags.String("platform", beeper.PlatformName, "platform adapter to inspect")
	live := flags.Bool("live", false, "also check recent live delivered replies against the platform contract")
	jsonOut := flags.Bool("json", false, "print machine-readable JSON")
	record := flags.Bool("record", false, "persist observed platform state")
	cacheDir := flags.String("cache-dir", "", "override updater pending cache directory")
	if _, err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := validatePlatformFlag(*platformFlag); err != nil {
		return err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	baseClient, err := beeperClient(cfg, mdomain.Group{})
	if err != nil {
		return err
	}
	observer := beeper.NewUpdateObserver(baseClient, beeperUpdaterCacheDir(*cacheDir))
	store := mfiles.NewPlatformStateStore(cfg.platformStateDir())

	var replyIssues []string
	var repliesChecked int
	if *live {
		repliesChecked, replyIssues, err = collectLiveReplyIssues(ctx, cfg, baseClient)
		if err != nil {
			return err
		}
	}

	var assessment mdomain.Assessment
	if *record {
		assessment, err = mcommand.PlatformUpdateCoordinator{
			Platform: observer,
			State:    store,
			Now:      time.Now,
		}.ObserveAndRecord(ctx, replyIssues, repliesChecked)
	} else {
		assessment, err = mquery.PlatformUpdateStatus{
			Platform: observer,
			State:    store,
		}.Check(ctx, replyIssues, repliesChecked)
	}
	if err != nil {
		return err
	}

	if *jsonOut {
		if err := printJSON(call.out, toAssessmentJSON(assessment)); err != nil {
			return err
		}
	} else {
		printAssessmentHuman(call, assessment)
	}
	if assessment.ConnectionError != "" {
		return errors.New(assessment.ConnectionError)
	}
	return nil
}

func platformUpdate(ctx context.Context, call invocation, args []string) error {
	if len(args) == 0 {
		return usageError{errors.New("platform-update requires an action: assess, verify, or record-rollback")}
	}
	switch args[0] {
	case "-h", "--help":
		fmt.Fprintln(call.errs, "usage: gentalent [--config PATH] platform-update <assess|verify|record-rollback> [flags]")
		return flag.ErrHelp
	case "assess":
		return platformUpdateAssess(ctx, call, args[1:])
	case "verify":
		return platformUpdateVerify(ctx, call, args[1:])
	case "record-rollback":
		return platformUpdateRecordRollback(ctx, call, args[1:])
	default:
		return usageError{fmt.Errorf("unknown platform-update action %q", args[0])}
	}
}

func platformUpdateAssess(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "platform-update assess")
	platformFlag := flags.String("platform", beeper.PlatformName, "platform adapter to inspect")
	candidate := flags.String("candidate", "", "candidate version to install")
	force := flags.Bool("force", false, "allow a blocked version")
	live := flags.Bool("live", false, "also check recent live delivered replies")
	jsonOut := flags.Bool("json", false, "print machine-readable JSON")
	cacheDir := flags.String("cache-dir", "", "override updater pending cache directory")
	if _, err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := validatePlatformFlag(*platformFlag); err != nil {
		return err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	baseClient, err := beeperClient(cfg, mdomain.Group{})
	if err != nil {
		return err
	}
	var replyIssues []string
	var repliesChecked int
	if *live {
		repliesChecked, replyIssues, err = collectLiveReplyIssues(ctx, cfg, baseClient)
		if err != nil {
			return err
		}
	}
	observer := beeper.NewUpdateObserver(baseClient, beeperUpdaterCacheDir(*cacheDir))
	store := mfiles.NewPlatformStateStore(cfg.platformStateDir())
	assessment, err := mcommand.PlatformUpdateCoordinator{
		Platform: observer,
		State:    store,
		Now:      time.Now,
	}.AssessManagedUpdate(ctx, *candidate, *force, replyIssues, repliesChecked)
	if err != nil {
		return err
	}
	if *jsonOut {
		if err := printJSON(call.out, toAssessmentJSON(assessment)); err != nil {
			return err
		}
	} else {
		printAssessmentHuman(call, assessment)
	}
	if !assessment.ManagedUpdateAllowed {
		return errors.New(assessment.RefusalReason)
	}
	return nil
}

func platformUpdateVerify(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "platform-update verify")
	platformFlag := flags.String("platform", beeper.PlatformName, "platform adapter to inspect")
	previous := flags.String("previous", "", "previous installed platform version")
	target := flags.String("target", "", "target installed platform version")
	live := flags.Bool("live", false, "also check recent live delivered replies")
	jsonOut := flags.Bool("json", false, "print machine-readable JSON")
	cacheDir := flags.String("cache-dir", "", "override updater pending cache directory")
	if _, err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := validatePlatformFlag(*platformFlag); err != nil {
		return err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	baseClient, err := beeperClient(cfg, mdomain.Group{})
	if err != nil {
		return err
	}
	var replyIssues []string
	var repliesChecked int
	if *live {
		repliesChecked, replyIssues, err = collectLiveReplyIssues(ctx, cfg, baseClient)
		if err != nil {
			return err
		}
	}
	observer := beeper.NewUpdateObserver(baseClient, beeperUpdaterCacheDir(*cacheDir))
	store := mfiles.NewPlatformStateStore(cfg.platformStateDir())
	verification, err := mcommand.PlatformUpdateCoordinator{
		Platform: observer,
		State:    store,
		Now:      time.Now,
	}.VerifyManagedUpdate(ctx, *previous, *target, replyIssues, repliesChecked)
	if err != nil {
		return err
	}
	if *jsonOut {
		if err := printJSON(call.out, toVerificationJSON(verification)); err != nil {
			return err
		}
	} else {
		printVerificationHuman(call, verification)
	}
	if !verification.Compatible {
		return fmt.Errorf("platform update verification failed: %s", strings.Join(verification.IncompatibleReasons, "; "))
	}
	return nil
}

func platformUpdateRecordRollback(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "platform-update record-rollback")
	platformFlag := flags.String("platform", beeper.PlatformName, "platform adapter to inspect")
	previous := flags.String("previous", "", "previous version restored")
	failed := flags.String("failed", "", "failed version to mark blocked")
	specVersion := flags.String("spec-version", "", "observed spec version on the failed target")
	reason := flags.String("reason", "", "failure reason to record")
	jsonOut := flags.Bool("json", false, "print machine-readable JSON")
	if _, err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := validatePlatformFlag(*platformFlag); err != nil {
		return err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	var reasons []string
	if strings.TrimSpace(*reason) != "" {
		reasons = []string{strings.TrimSpace(*reason)}
	}
	store := mfiles.NewPlatformStateStore(cfg.platformStateDir())
	state, err := mcommand.PlatformUpdateCoordinator{
		Platform: beeper.NewUpdateObserver(nil, ""),
		State:    store,
		Now:      time.Now,
	}.RecordRollback(ctx, *previous, *failed, *specVersion, reasons)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(call.out, toPlatformStatusJSON(state))
	}
	fmt.Fprintf(call.out, "Recorded rollback to %s; blocked %s (%s).\n", state.LastKnownVersion, *failed, beeper.AdapterRevision)
	return nil
}

func validatePlatformFlag(name string) error {
	if name != beeper.PlatformName {
		return usageError{fmt.Errorf("unsupported platform %q", name)}
	}
	return nil
}

func beeperUpdaterCacheDir(override string) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return expandHome(trimmed)
	}
	if dir := strings.TrimSpace(os.Getenv("BEEPER_UPDATER_CACHE_DIR")); dir != "" {
		return expandHome(dir)
	}
	return beeper.DefaultUpdaterCacheDir()
}

func collectLiveReplyIssues(ctx context.Context, cfg config, baseClient *beeper.Client) (int, []string, error) {
	if baseClient == nil {
		return 0, nil, nil
	}
	if err := baseClient.CheckConnection(ctx); err != nil {
		return 0, nil, nil
	}
	groups, multi, err := cfg.selectGroups("")
	if err != nil {
		return 0, nil, err
	}
	var issues []string
	totalChecked := 0
	for _, g := range groups {
		path := g.databases(true).messaging
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return totalChecked, nil, err
		}
		replies, closers, err := openMessaging(ctx, path, sqlitedb.ReadOnly)
		if err != nil {
			return totalChecked, nil, err
		}
		client, err := beeperClient(cfg, g.group())
		if err != nil {
			_ = closers.close()
			return totalChecked, nil, err
		}
		checked, replyIssues, err := (mquery.Doctor{Platform: client, Replies: replies}).SentReplyIssues(ctx)
		_ = closers.close()
		totalChecked += checked
		if err != nil {
			issues = append(issues, prefixIssue(g.Name, multi, err.Error()))
			continue
		}
		for _, issue := range replyIssues {
			issues = append(issues, prefixIssue(g.Name, multi, issue))
		}
	}
	return totalChecked, issues, nil
}

func prefixIssue(group string, multi bool, issue string) string {
	if multi && group != "" {
		return fmt.Sprintf("group %s: %s", group, issue)
	}
	return issue
}

func toAssessmentJSON(a mdomain.Assessment) assessmentJSON {
	out := assessmentJSON{
		Platform:               a.Platform,
		CurrentVersion:         a.CurrentVersion,
		SpecVersion:            a.SpecVersion,
		ExpectedSpecMajor:      a.ExpectedSpecMajor,
		ExpectedSpecVersion:    a.ExpectedSpecVersion,
		AdapterRevision:        a.AdapterRevision,
		Compatibility:          string(a.Compatibility),
		IncompatibleReasons:    a.IncompatibleReasons,
		ReviewNotice:           a.ReviewNotice,
		ExternalUpdateDetected: a.ExternalUpdateDetected,
		AutoInstallAllowed:     a.AutoInstallAllowed,
		KeepRunning:            a.KeepRunning,
		RequiresCodeChanges:    a.RequiresCodeChanges,
		ManagedUpdateAllowed:   a.ManagedUpdateAllowed,
		CandidateVersion:       a.CandidateVersion,
		CandidateBlocked:       a.CandidateBlocked,
		ForceBypassedBlock:     a.ForceBypassedBlock,
		RefusalReason:          a.RefusalReason,
		ActiveBlockedVersions:  toBlockedVersionsJSON(a.ActiveBlockedVersions),
		ClearedBlockedVersions: toBlockedVersionsJSON(a.ClearedBlockedVersions),
		ConnectionError:        a.ConnectionError,
	}
	if a.Pending != nil {
		out.Pending = &pendingUpdateJSON{
			Version:           a.Pending.Version,
			FileName:          a.Pending.FileName,
			FilePath:          a.Pending.FilePath,
			SHA512:            a.Pending.SHA512,
			Verified:          a.Pending.Verified,
			VerificationError: a.Pending.VerificationError,
			Source:            string(a.Pending.Source),
		}
	}
	if a.BlockedBy != nil {
		converted := toBlockedVersionJSON(*a.BlockedBy)
		out.BlockedBy = &converted
	}
	return out
}

func toVerificationJSON(v mdomain.Verification) verificationJSON {
	out := verificationJSON{
		Platform:                v.Platform,
		PreviousVersion:         v.PreviousVersion,
		TargetVersion:           v.TargetVersion,
		CurrentVersion:          v.CurrentVersion,
		SpecVersion:             v.SpecVersion,
		AdapterRevision:         v.AdapterRevision,
		Compatible:              v.Compatible,
		Compatibility:           string(v.Compatibility),
		IncompatibleReasons:     v.IncompatibleReasons,
		ReviewNotice:            v.ReviewNotice,
		RequiresRollback:        v.RequiresRollback,
		RollbackToVersion:       v.RollbackToVersion,
		MarkRequiresCodeChanges: v.MarkRequiresCodeChanges,
	}
	if v.BlockedEntry != nil {
		converted := toBlockedVersionJSON(*v.BlockedEntry)
		out.BlockedEntry = &converted
	}
	return out
}

func toPlatformStatusJSON(state mdomain.PlatformUpdateState) platformStatusJSON {
	out := platformStatusJSON{
		Platform:                 state.Platform,
		LastKnownVersion:         state.LastKnownVersion,
		LastKnownSpecVersion:     state.LastKnownSpecVersion,
		LastManagedVersion:       state.LastManagedVersion,
		LastExternalDriftVersion: state.LastExternalDriftVersion,
		BlockedVersions:          toBlockedVersionsJSON(state.BlockedVersions),
	}
	if !state.LastCheckedAt.IsZero() {
		out.LastCheckedAt = state.LastCheckedAt.UTC().Format(time.RFC3339Nano)
	}
	if state.LastExternalDriftVersion != "" {
		out.RequiresCodeChanges = true
	}
	for _, blocked := range state.BlockedVersions {
		if blocked.AdapterRevision == beeper.AdapterRevision {
			out.RequiresCodeChanges = true
			break
		}
	}
	return out
}

func toBlockedVersionsJSON(entries []mdomain.BlockedVersion) []blockedVersionJSON {
	if len(entries) == 0 {
		return nil
	}
	out := make([]blockedVersionJSON, len(entries))
	for i, entry := range entries {
		out[i] = toBlockedVersionJSON(entry)
	}
	return out
}

func toBlockedVersionJSON(entry mdomain.BlockedVersion) blockedVersionJSON {
	out := blockedVersionJSON{
		Version:         entry.Version,
		AdapterRevision: entry.AdapterRevision,
		SpecVersion:     entry.SpecVersion,
		Source:          entry.Source,
		Reasons:         entry.Reasons,
	}
	if !entry.MarkedAt.IsZero() {
		out.MarkedAt = entry.MarkedAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

func printAssessmentHuman(call invocation, a mdomain.Assessment) {
	if a.ConnectionError != "" {
		fmt.Fprintf(call.out, "Platform %s is unreachable (%s).\n", a.Platform, a.ConnectionError)
	} else {
		fmt.Fprintf(call.out, "Platform %s version %s (API spec %s, adapter %s): %s.\n", a.Platform, a.CurrentVersion, a.SpecVersion, a.AdapterRevision, a.Compatibility)
	}
	if a.ExternalUpdateDetected {
		fmt.Fprintf(call.out, "External platform update detected (now %s).\n", a.CurrentVersion)
	}
	if a.ReviewNotice != "" {
		fmt.Fprintf(call.out, "Review note: %s\n", a.ReviewNotice)
	}
	if len(a.IncompatibleReasons) > 0 {
		fmt.Fprintf(call.out, "Incompatible (code changes required): %s\n", strings.Join(a.IncompatibleReasons, "; "))
	}
	if a.Pending != nil {
		if a.Pending.Verified {
			fmt.Fprintf(call.out, "Pending update: %s (%s, SHA-512 verified; run mise run update-platform %s).\n", a.Pending.Version, a.Pending.FileName, a.Platform)
		} else {
			fmt.Fprintf(call.out, "Pending update %s failed verification: %s.\n", a.Pending.Version, a.Pending.VerificationError)
		}
	}
	if a.CandidateVersion != "" {
		if a.ManagedUpdateAllowed {
			fmt.Fprintf(call.out, "Managed update to %s is allowed.\n", a.CandidateVersion)
		} else if a.RefusalReason != "" {
			fmt.Fprintf(call.out, "Managed update to %s refused: %s.\n", a.CandidateVersion, a.RefusalReason)
		}
	}
	if len(a.ActiveBlockedVersions) > 0 {
		versions := make([]string, len(a.ActiveBlockedVersions))
		for i, blocked := range a.ActiveBlockedVersions {
			versions[i] = blocked.Version
		}
		fmt.Fprintf(call.out, "Blocked versions requiring code changes: %s.\n", strings.Join(versions, ", "))
	}
}

func printVerificationHuman(call invocation, v mdomain.Verification) {
	if v.Compatible {
		fmt.Fprintf(call.out, "Platform %s update to %s verified (API spec %s): %s.\n", v.Platform, v.CurrentVersion, v.SpecVersion, v.Compatibility)
		if v.ReviewNotice != "" {
			fmt.Fprintf(call.out, "Review note: %s\n", v.ReviewNotice)
		}
		return
	}
	fmt.Fprintf(call.out, "Platform %s update to %s is incompatible (%s); rollback to %s required.\n", v.Platform, v.TargetVersion, strings.Join(v.IncompatibleReasons, "; "), v.RollbackToVersion)
}
