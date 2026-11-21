package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"

	mcommand "gentalent/internal/messaging/app/command"
	mdomain "gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/infra/beeper"
	mfiles "gentalent/internal/messaging/infra/files"
	"gentalent/internal/shared/sqlitedb"
)

type statusJSON struct {
	Nominations    []nominationJSON      `json:"nominations"`
	Counts         map[mdomain.State]int `json:"outbox_counts"`
	Attention      []attentionJSON       `json:"outbox_needing_attention"`
	PlatformUpdate *platformStatusJSON   `json:"platform_update,omitempty"`
}

type platformStatusJSON struct {
	Platform                 string               `json:"platform"`
	LastKnownVersion         string               `json:"last_known_version,omitempty"`
	LastKnownSpecVersion     string               `json:"last_known_spec_version,omitempty"`
	LastManagedVersion       string               `json:"last_managed_version,omitempty"`
	LastExternalDriftVersion string               `json:"last_external_drift_version,omitempty"`
	LastCheckedAt            string               `json:"last_checked_at,omitempty"`
	RequiresCodeChanges      bool                 `json:"requires_code_changes,omitempty"`
	BlockedVersions          []blockedVersionJSON `json:"blocked_versions,omitempty"`
}

type nominationJSON struct {
	Number int    `json:"number"`
	Person string `json:"person"`
	Status string `json:"status"`
	Votes  int    `json:"votes"`
}

type attentionJSON struct {
	ID     string        `json:"id"`
	Status mdomain.State `json:"status"`
	Kind   string        `json:"kind"`
	Tag    string        `json:"tag"`
	Text   string        `json:"text"`
}

type feedbackJSON struct {
	SaidAt   string `json:"said_at"`
	SenderID string `json:"sender_id"`
	Kind     string `json:"kind"`
	Text     string `json:"text"`
}

func readOnlyFlags(call invocation, name string, args []string) (config, []groupConfig, bool, bool, error) {
	flags := newFlags(call, name)
	live := flags.Bool("live", false, "use the real ballot")
	groupFlag := flags.String("group", "", "only this configured group")
	if _, err := parseFlags(flags, args); err != nil {
		return config{}, nil, false, false, err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return config{}, nil, false, false, err
	}
	groups, multi, err := cfg.selectGroups(*groupFlag)
	return cfg, groups, multi, *live, err
}

func status(ctx context.Context, call invocation, args []string) error {
	cfg, groups, multi, live, err := readOnlyFlags(call, "status", args)
	if err != nil {
		return err
	}
	platformStatus, err := readPlatformStatus(ctx, cfg)
	if err != nil {
		return err
	}
	if !multi {
		report, err := readGroupStatus(ctx, groups[0], live)
		if err != nil {
			return err
		}
		report.PlatformUpdate = platformStatus
		return printJSON(call.out, report)
	}
	reports := make(map[string]statusJSON, len(groups))
	for _, g := range groups {
		report, err := readGroupStatus(ctx, g, live)
		if err != nil {
			return prefixGroupError(g.Name, err)
		}
		report.PlatformUpdate = platformStatus
		reports[g.Name] = report
	}
	return printJSON(call.out, reports)
}

func readPlatformStatus(ctx context.Context, cfg config) (*platformStatusJSON, error) {
	store := mfiles.NewPlatformStateStore(cfg.platformStateDir())
	state, err := store.Load(ctx, beeper.PlatformName)
	if err != nil {
		return nil, err
	}
	if state.LastKnownVersion == "" &&
		state.LastKnownSpecVersion == "" &&
		state.LastManagedVersion == "" &&
		state.LastExternalDriftVersion == "" &&
		len(state.BlockedVersions) == 0 &&
		state.LastCheckedAt.IsZero() {
		return nil, nil
	}
	out := &platformStatusJSON{
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
	return out, nil
}

func readGroupStatus(ctx context.Context, g groupConfig, live bool) (statusJSON, error) {
	nominations, closeNominations, err := openNominationsReader(ctx, g, live)
	if err != nil {
		return statusJSON{}, err
	}
	defer closeNominations.close()
	replies, closeMessaging, err := openMessaging(ctx, g.databases(live).messaging, sqlitedb.ReadOnly)
	if err != nil {
		return statusJSON{}, err
	}
	defer closeMessaging.close()
	every, err := nominations.Nominations(ctx)
	if err != nil {
		return statusJSON{}, err
	}
	outbox, err := replies.UncertainReplies(ctx)
	if err != nil {
		return statusJSON{}, err
	}
	report := statusJSON{Nominations: make([]nominationJSON, len(every)), Counts: outbox.Counts, Attention: make([]attentionJSON, len(outbox.Attention))}
	for i, nomination := range every {
		report.Nominations[i] = nominationJSON(nomination)
	}
	for i, reply := range outbox.Attention {
		report.Attention[i] = attentionJSON{ID: reply.Key, Status: reply.State, Kind: reply.Kind, Tag: reply.Tag, Text: reply.Text}
	}
	return report, nil
}

func feedback(ctx context.Context, call invocation, args []string) error {
	_, groups, multi, live, err := readOnlyFlags(call, "feedback", args)
	if err != nil {
		return err
	}
	if !multi {
		shown, err := readGroupFeedback(ctx, groups[0], live)
		if err != nil {
			return err
		}
		return printJSON(call.out, shown)
	}
	reports := make(map[string][]feedbackJSON, len(groups))
	for _, g := range groups {
		shown, err := readGroupFeedback(ctx, g, live)
		if err != nil {
			return prefixGroupError(g.Name, err)
		}
		reports[g.Name] = shown
	}
	return printJSON(call.out, reports)
}

func readGroupFeedback(ctx context.Context, g groupConfig, live bool) ([]feedbackJSON, error) {
	nominations, closers, err := openNominationsReader(ctx, g, live)
	if err != nil {
		return nil, err
	}
	defer closers.close()
	entries, err := nominations.Feedback(ctx)
	if err != nil {
		return nil, err
	}
	shown := make([]feedbackJSON, len(entries))
	for i, entry := range entries {
		shown[i] = feedbackJSON{SenderID: string(entry.Sender), Kind: entry.Kind, Text: entry.Text}
		if !entry.SaidAt.IsZero() {
			shown[i].SaidAt = entry.SaidAt.UTC().Format(time.RFC3339Nano)
		}
	}
	return shown, nil
}

func busy(ctx context.Context, call invocation, args []string) error {
	_, groups, multi, live, err := readOnlyFlags(call, "busy", args)
	if err != nil {
		return err
	}
	var allRunning []string
	for _, g := range groups {
		running, err := groupBusy(ctx, g, live)
		if err != nil {
			return prefixGroupError(g.Name, err)
		}
		if len(running) == 0 {
			continue
		}
		if multi {
			allRunning = append(allRunning, fmt.Sprintf("%s (%s)", g.Name, strings.Join(running, " and ")))
		} else {
			allRunning = append(allRunning, running...)
		}
	}
	if len(allRunning) > 0 {
		separator := " and "
		if multi {
			separator = "; "
		}
		return fmt.Errorf("%s are running; a restart now would abandon them", strings.Join(allRunning, separator))
	}
	fmt.Fprintln(call.out, "No AI search is running.")
	return nil
}

func groupBusy(ctx context.Context, g groupConfig, live bool) ([]string, error) {
	nominations, closers, err := openNominationsReader(ctx, g, live)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closers.close()
	photos, err := nominations.SearchesRunning(ctx)
	if err != nil {
		return nil, err
	}
	who, err := nominations.WhoSearchesRunning(ctx)
	if err != nil {
		return nil, err
	}
	var running []string
	if len(photos) > 0 {
		running = append(running, "photo searches for nominations "+numbered(photos))
	}
	if len(who) > 0 {
		running = append(running, "who searches "+numbered(who))
	}
	return running, nil
}

func numbered(numbers []int) string {
	shown := make([]string, len(numbers))
	for i, number := range numbers {
		shown[i] = strconv.Itoa(number)
	}
	return strings.Join(shown, ", ")
}

func resolveSend(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "resolve-send")
	messageID := flags.String("message-id", "", "Beeper message ID that delivered this reply; verified against Beeper")
	notDelivered := flags.Bool("not-delivered", false, "record that the reply never arrived; it is not resent")
	live := flags.Bool("live", false, "use the real ballot")
	groupFlag := flags.String("group", "", "only this configured group")
	key, err := parseFlags(flags, args, "OUTBOX_ID")
	if err != nil {
		return err
	}
	if (*messageID == "") != *notDelivered {
		return usageError{errors.New("resolve-send needs exactly one of --message-id or --not-delivered")}
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	groups, multi, err := cfg.selectGroups(*groupFlag)
	if err != nil {
		return err
	}
	if multi {
		return usageError{errors.New("resolve-send requires --group when multiple groups are configured")}
	}
	g := groups[0]
	store, closers, err := openMessaging(ctx, g.databases(*live).messaging, sqlitedb.ReadWrite)
	if err != nil {
		return err
	}
	defer closers.close()
	client, err := beeperClient(cfg, g.group())
	if err != nil {
		return err
	}
	resolver := mcommand.Resolver{Store: store, Platform: client, Files: mfiles.AttachmentReader{}}
	if err := resolver.ResolveSend(ctx, key[0], *messageID); err != nil {
		return err
	}
	if *notDelivered {
		fmt.Fprintf(call.out, "Reply %s recorded as not delivered. It will not be resent.\n", key[0])
	} else {
		fmt.Fprintf(call.out, "Reply %s recorded as sent (message %s).\n", key[0], *messageID)
	}
	return nil
}
