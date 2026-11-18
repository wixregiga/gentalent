package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	aicommand "gentalent/internal/ai/app/command"
	"gentalent/internal/ai/infra/programs"
	mcommand "gentalent/internal/messaging/app/command"
	"gentalent/internal/messaging/infra/preview"
	"gentalent/internal/messaging/infra/worker"
)

func arm(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "arm")
	ifUnarmed := flags.Bool("if-unarmed", false, "do nothing if this database is already armed")
	live := flags.Bool("live", false, "use the real ballot")
	if _, err := parseFlags(flags, args); err != nil {
		return err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	databases, err := openWorker(ctx, cfg, *live, time.Now)
	if err != nil {
		return err
	}
	defer databases.close()
	client, err := beeperClient(cfg, cfg.group())
	if err != nil {
		return err
	}
	armed, err := mcommand.Armer{Platform: client, Store: databases.inbox}.Arm(ctx, *ifUnarmed)
	if err != nil {
		return err
	}
	if !armed {
		fmt.Fprintln(call.out, "Already armed.")
		return nil
	}
	fmt.Fprintln(call.out, "Armed for new messages only. Historical commands will not be processed.")
	return nil
}

func runBot(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "run")
	once := flags.Bool("once", false, "run one poll, wait for its photo searches, and stop")
	live := flags.Bool("live", false, "use the real ballot and send replies")
	if _, err := parseFlags(flags, args); err != nil {
		return err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	if err := cfg.validateGroup(); err != nil {
		return err
	}
	finder, err := photoFinder(cfg, "")
	if err != nil {
		return err
	}
	if *live {
		lock, err := worker.LockLiveGroup(worker.Binding(cfg.group()))
		if err != nil {
			return err
		}
		defer lock.Close()
	}
	databases, err := openWorker(ctx, cfg, *live, time.Now)
	if err != nil {
		return err
	}
	defer databases.close()
	client, err := beeperClient(cfg, cfg.group())
	if err != nil {
		return err
	}
	on := platform{reader: client, sender: client, delivery: client}
	mode := "LIVE replies enabled."
	if !*live {
		on = platform{reader: client, sender: preview.Sender{Output: call.out}}
		mode = "DRY RUN: isolated ballot, previews only."
	}
	fmt.Fprintln(call.out, mode)
	stopping, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewTextHandler(call.errs, nil))
	err = newEngine(cfg, databases, on, finder, log).run(stopping, *once)
	if stopping.Err() != nil && ctx.Err() == nil {
		fmt.Fprintln(call.out, "Stopped. Ballot and cursor are saved; start the same command to resume.")
	}
	return err
}

func photoFinder(cfg config, only string) (*aicommand.Finder, error) {
	if err := cfg.validateMedia(); err != nil {
		return nil, err
	}
	providers, err := cfg.providers(only)
	if err != nil {
		return nil, err
	}
	images, err := programs.NewImageMagick()
	if err != nil {
		return nil, err
	}
	finder, err := aicommand.NewFinder(providers, programs.Programs(), images, programs.SourcesFile{})
	if err != nil {
		return nil, fmt.Errorf("ai_providers: %w", err)
	}
	return finder, nil
}
