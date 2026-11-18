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
	mfiles "gentalent/internal/messaging/infra/files"
	"gentalent/internal/shared/sqlitedb"
)

type statusJSON struct {
	Nominations string                `json:"nominations"`
	Counts      map[mdomain.State]int `json:"outbox_counts"`
	Attention   []attentionJSON       `json:"outbox_needing_attention"`
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

func readOnlyFlags(call invocation, name string, args []string) (config, bool, error) {
	flags := newFlags(call, name)
	live := flags.Bool("live", false, "use the real ballot")
	if _, err := parseFlags(flags, args); err != nil {
		return config{}, false, err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return config{}, false, err
	}
	return cfg, *live, cfg.validateGroup()
}

func status(ctx context.Context, call invocation, args []string) error {
	cfg, live, err := readOnlyFlags(call, "status", args)
	if err != nil {
		return err
	}
	bot, closeNominations, err := openNominationsReader(ctx, cfg, live)
	if err != nil {
		return err
	}
	defer closeNominations.close()
	replies, closeMessaging, err := openMessaging(ctx, cfg.databases(live).messaging, sqlitedb.ReadOnly)
	if err != nil {
		return err
	}
	defer closeMessaging.close()
	standing, err := bot.StatusReport(ctx)
	if err != nil {
		return err
	}
	outbox, err := replies.UncertainReplies(ctx)
	if err != nil {
		return err
	}
	report := statusJSON{Nominations: standing, Counts: outbox.Counts, Attention: make([]attentionJSON, len(outbox.Attention))}
	for i, reply := range outbox.Attention {
		report.Attention[i] = attentionJSON{ID: reply.Key, Status: reply.State, Kind: reply.Kind, Tag: reply.Tag, Text: reply.Text}
	}
	return printJSON(call.out, report)
}

func feedback(ctx context.Context, call invocation, args []string) error {
	cfg, live, err := readOnlyFlags(call, "feedback", args)
	if err != nil {
		return err
	}
	bot, closers, err := openNominationsReader(ctx, cfg, live)
	if err != nil {
		return err
	}
	defer closers.close()
	entries, err := bot.Feedback(ctx)
	if err != nil {
		return err
	}
	shown := make([]feedbackJSON, len(entries))
	for i, entry := range entries {
		shown[i] = feedbackJSON{SenderID: string(entry.Sender), Kind: entry.Kind, Text: entry.Text}
		if !entry.SaidAt.IsZero() {
			shown[i].SaidAt = entry.SaidAt.UTC().Format(time.RFC3339Nano)
		}
	}
	return printJSON(call.out, shown)
}

func busy(ctx context.Context, call invocation, args []string) error {
	cfg, live, err := readOnlyFlags(call, "busy", args)
	if err != nil {
		return err
	}
	bot, closers, err := openNominationsReader(ctx, cfg, live)
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintln(call.out, "No photo search is running.")
		return nil
	}
	if err != nil {
		return err
	}
	defer closers.close()
	running, err := bot.SearchesRunning(ctx)
	if err != nil {
		return err
	}
	if len(running) > 0 {
		numbers := make([]string, len(running))
		for i, number := range running {
			numbers[i] = strconv.Itoa(number)
		}
		return fmt.Errorf("photo searches are running for nominations %s; a restart now would abandon them", strings.Join(numbers, ", "))
	}
	fmt.Fprintln(call.out, "No photo search is running.")
	return nil
}

func resolveSend(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "resolve-send")
	messageID := flags.String("message-id", "", "Beeper message ID that delivered this reply; verified against Beeper")
	notDelivered := flags.Bool("not-delivered", false, "record that the reply never arrived; it is not resent")
	live := flags.Bool("live", false, "use the real ballot")
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
	if err := cfg.validateGroup(); err != nil {
		return err
	}
	store, closers, err := openMessaging(ctx, cfg.databases(*live).messaging, sqlitedb.ReadWrite)
	if err != nil {
		return err
	}
	defer closers.close()
	client, err := beeperClient(cfg, cfg.group())
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
