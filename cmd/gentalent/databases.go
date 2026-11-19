package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	msqlite "gentalent/internal/messaging/infra/sqlite"
	"gentalent/internal/messaging/infra/worker"
	"gentalent/internal/nominations/delivery/chat"
	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/infra/files"
	nsqlite "gentalent/internal/nominations/infra/sqlite"
	"gentalent/internal/shared/sqlitedb"
)

type databasePaths struct {
	messaging   string
	nominations string
}

func (c config) databases(live bool) databasePaths {
	paths := databasePaths{messaging: c.MessagingDatabase, nominations: c.NominationsDatabase}
	if live {
		return paths
	}
	paths.messaging = worker.DryRunDatabase(paths.messaging)
	paths.nominations = worker.DryRunDatabase(paths.nominations)
	return paths
}

type closers []func() error

func (c closers) close() error {
	var err error
	for _, close := range slices.Backward(c) {
		err = errors.Join(err, close())
	}
	return err
}

type workerDatabases struct {
	bot   *chat.Bot
	inbox *msqlite.Store
	now   func() time.Time
	closers
}

func openWorker(ctx context.Context, cfg config, live bool, now func() time.Time) (opened *workerDatabases, err error) {
	if err := errors.Join(cfg.validateGroup(), cfg.validateMedia()); err != nil {
		return nil, err
	}
	w := &workerDatabases{now: now}
	defer func() {
		if err != nil {
			err = errors.Join(err, w.close())
		}
	}()
	paths := cfg.databases(live)
	for _, path := range []string{paths.messaging, paths.nominations} {
		lock, err := worker.LockDatabase(path)
		if err != nil {
			return nil, err
		}
		w.closers = append(w.closers, lock.Close)
	}
	binding := worker.Binding(cfg.group())

	messaging, err := sqlitedb.Open(ctx, paths.messaging, sqlitedb.ReadWrite)
	if err != nil {
		return nil, err
	}
	w.closers = append(w.closers, messaging.Close)
	if err := sqlitedb.Migrate(ctx, messaging, msqlite.Migrations()); err != nil {
		return nil, fmt.Errorf("migrate %s: %w", paths.messaging, err)
	}
	w.inbox = msqlite.New(messaging)
	if err := w.inbox.StartWorker(ctx, binding); err != nil {
		return nil, err
	}

	nominations, err := sqlitedb.Open(ctx, paths.nominations, sqlitedb.ReadWrite)
	if err != nil {
		return nil, err
	}
	w.closers = append(w.closers, nominations.Close)
	if err := sqlitedb.Migrate(ctx, nominations, nsqlite.Migrations()); err != nil {
		return nil, fmt.Errorf("migrate %s: %w", paths.nominations, err)
	}
	store, err := nsqlite.NewStore(ctx, nominations, domain.GroupID(binding), cfg.memberIDs(), cfg.settings())
	if err != nil {
		return nil, settingsError(err)
	}
	readSide, err := sqlitedb.Open(ctx, paths.nominations, sqlitedb.ReadOnly)
	if err != nil {
		return nil, err
	}
	w.closers = append(w.closers, readSide.Close)
	w.bot = chat.NewBot(cfg.chat(now), chat.Ports{
		Groups:    store,
		Outbox:    store,
		Feedback:  store,
		Gifs:      files.NewGifBank(filepath.Join(cfg.MediaDir, "shame-gifs")),
		ReadPorts: readPorts(nsqlite.NewReader(readSide)),
	})
	return w, nil
}

func readPorts(reader nsqlite.Reader) chat.ReadPorts {
	return chat.ReadPorts{Standing: reader, Names: reader, Unrelayed: reader, Entries: reader, Nominations: reader}
}

func existingDatabase(path string) error {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no database at %s: %w", path, err)
	} else if err != nil {
		return err
	}
	return nil
}

func openNominationsReader(ctx context.Context, cfg config, live bool) (*chat.Reader, closers, error) {
	path := cfg.databases(live).nominations
	if err := existingDatabase(path); err != nil {
		return nil, nil, err
	}
	db, err := sqlitedb.Open(ctx, path, sqlitedb.ReadOnly)
	if err != nil {
		return nil, nil, err
	}
	return chat.NewReader(cfg.chat(time.Now), readPorts(nsqlite.NewReader(db))), closers{db.Close}, nil
}

func openMessaging(ctx context.Context, path string, access sqlitedb.Access) (*msqlite.Store, closers, error) {
	if err := existingDatabase(path); err != nil {
		return nil, nil, err
	}
	db, err := sqlitedb.Open(ctx, path, access)
	if err != nil {
		return nil, nil, err
	}
	return msqlite.New(db), closers{db.Close}, nil
}
