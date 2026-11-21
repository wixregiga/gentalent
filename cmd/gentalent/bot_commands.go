package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	aicommand "gentalent/internal/ai/app/command"
	"gentalent/internal/ai/infra/programs"
	mcommand "gentalent/internal/messaging/app/command"
	mdomain "gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/infra/beeper"
	mfiles "gentalent/internal/messaging/infra/files"
	"gentalent/internal/messaging/infra/preview"
	"gentalent/internal/messaging/infra/worker"
	"gentalent/internal/messaging/port"
)

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func arm(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "arm")
	ifUnarmed := flags.Bool("if-unarmed", false, "do nothing if this database is already armed")
	live := flags.Bool("live", false, "use the real ballot")
	groupFlag := flags.String("group", "", "only this configured group")
	if _, err := parseFlags(flags, args); err != nil {
		return err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	groups, multi, err := cfg.selectGroups(*groupFlag)
	if err != nil {
		return err
	}
	if err := cfg.validateAllMedia(groups); err != nil {
		return err
	}
	var errs []error
	for _, g := range groups {
		armed, err := armGroup(ctx, cfg, g, *live, *ifUnarmed)
		if err != nil {
			errs = append(errs, prefixGroupError(g.Name, err))
			continue
		}
		line := "Armed for new messages only. Historical commands will not be processed."
		if !armed {
			line = "Already armed."
		}
		if multi {
			fmt.Fprintf(call.out, "%s: %s\n", g.Name, line)
		} else {
			fmt.Fprintln(call.out, line)
		}
	}
	return errors.Join(errs...)
}

func armGroup(ctx context.Context, cfg config, g groupConfig, live, ifUnarmed bool) (bool, error) {
	databases, err := openWorker(ctx, g, live, time.Now)
	if err != nil {
		return false, err
	}
	defer databases.close()
	client, err := beeperClient(cfg, g.group())
	if err != nil {
		return false, err
	}
	return mcommand.Armer{Platform: client, Store: databases.inbox}.Arm(ctx, ifUnarmed)
}

func runBot(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "run")
	once := flags.Bool("once", false, "run one poll, wait for its photo searches, and stop")
	live := flags.Bool("live", false, "use the real ballot and send replies")
	groupFlag := flags.String("group", "", "only this configured group")
	if _, err := parseFlags(flags, args); err != nil {
		return err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	groups, multi, err := cfg.selectGroups(*groupFlag)
	if err != nil {
		return err
	}
	if err := cfg.validateAllMedia(groups); err != nil {
		return err
	}
	ai, err := aiPrograms(cfg, "")
	if err != nil {
		return err
	}
	var resources closers
	defer resources.close()
	if *live {
		for _, g := range groups {
			lock, err := worker.LockLiveGroup(worker.Binding(g.group()))
			if err != nil {
				return prefixGroupError(g.Name, err)
			}
			resources = append(resources, lock.Close)
		}
	}
	photoSlots := make(chan struct{}, cfg.PhotoSearchWorkers)
	whoSlots := make(chan struct{}, cfg.PhotoSearchWorkers)
	previewOut := call.out
	if !*live && multi {
		previewOut = &lockedWriter{w: call.out}
	}
	baseLog := slog.New(slog.NewTextHandler(call.errs, nil))
	monitorClient, err := beeperClient(cfg, mdomain.Group{})
	if err != nil {
		return err
	}
	engines := make([]*engine, 0, len(groups))
	names := make([]string, 0, len(groups))
	headerReaders := make([]port.HeaderVersionReader, 0, len(groups))
	for _, g := range groups {
		databases, err := openWorker(ctx, g, *live, time.Now)
		if err != nil {
			return prefixGroupError(g.Name, err)
		}
		resources = append(resources, databases.close)
		client, err := beeperClient(cfg, g.group())
		if err != nil {
			return prefixGroupError(g.Name, err)
		}
		headerReaders = append(headerReaders, client)
		on := platform{reader: client, sender: client, delivery: client}
		if !*live {
			on = platform{reader: client, sender: preview.Sender{Output: previewOut}}
		}
		groupLog := baseLog
		if multi {
			groupLog = baseLog.With("group", g.Name)
		}
		engines = append(engines, newEngineWithSlots(g, databases, on, ai, photoSlots, whoSlots, groupLog))
		names = append(names, g.Name)
	}
	var checkReplies func(context.Context) (int, []string, error)
	if *live {
		checkReplies = func(checkCtx context.Context) (int, []string, error) {
			return collectLiveReplyIssues(checkCtx, cfg, monitorClient)
		}
	}
	monitor := newPlatformMonitor(
		mcommand.PlatformUpdateCoordinator{
			Platform: beeper.NewUpdateObserver(monitorClient, beeperUpdaterCacheDir("")),
			State:    mfiles.NewPlatformStateStore(cfg.platformStateDir()),
			Now:      time.Now,
		},
		headerReaders,
		checkReplies,
		defaultPlatformCheckInterval,
		baseLog,
	)
	for _, e := range engines {
		e.monitor = monitor
	}
	mode := "LIVE replies enabled."
	if !*live {
		mode = "DRY RUN: isolated ballot, previews only."
	}
	fmt.Fprintln(call.out, mode)
	stopping, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = runEngines(stopping, engines, names, *once)
	if stopping.Err() != nil && ctx.Err() == nil {
		fmt.Fprintln(call.out, "Stopped. Ballot and cursor are saved; start the same command to resume.")
	}
	return err
}

func aiPrograms(cfg config, only string) (aiAgents, error) {
	if len(cfg.Groups) == 0 {
		if err := cfg.validateMedia(); err != nil {
			return aiAgents{}, err
		}
	} else {
		groups, err := cfg.resolveGroups()
		if err != nil {
			return aiAgents{}, err
		}
		if err := cfg.validateAllMedia(groups); err != nil {
			return aiAgents{}, err
		}
	}
	providers, err := cfg.providers(only)
	if err != nil {
		return aiAgents{}, err
	}
	images, err := programs.NewImageMagick()
	if err != nil {
		return aiAgents{}, err
	}
	finder, err := aicommand.NewFinder(providers, programs.Programs(), images, programs.SearchFiles{})
	if err != nil {
		return aiAgents{}, fmt.Errorf("ai_providers: %w", err)
	}
	identifier, err := aicommand.NewIdentifier(providers, programs.WhoAgents(), images)
	if err != nil {
		return aiAgents{}, fmt.Errorf("ai_providers: %w", err)
	}
	return aiAgents{finder: finder, identifier: identifier}, nil
}
