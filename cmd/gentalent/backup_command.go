package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gentalent/internal/shared/sqlitedb"
)

var backupLabel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func backup(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "backup")
	live := flags.Bool("live", false, "back up the real ballot")
	label := flags.String("label", "", "end of each backup's file name, such as before-abc1234")
	if _, err := parseFlags(flags, args); err != nil {
		return err
	}
	if !backupLabel.MatchString(*label) {
		return usageError{fmt.Errorf("--label must be letters, digits, '.', '_' or '-', starting with a letter or digit; got %q", *label)}
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	stamp := time.Now().Format("20060102-150405")
	paths := cfg.databases(*live)
	for _, path := range []string{paths.nominations, paths.messaging} {
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		destination := filepath.Join(filepath.Dir(path), "backups", fmt.Sprintf("%s-%s-%s.sqlite3", strings.TrimSuffix(filepath.Base(path), ".sqlite3"), stamp, *label))
		if err := backupDatabase(ctx, path, destination); err != nil {
			return err
		}
		fmt.Fprintf(call.out, "Backed up %s to %s\n", filepath.Base(path), destination)
	}
	return nil
}

func backupDatabase(ctx context.Context, path, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	db, err := sqlitedb.Open(ctx, path, sqlitedb.ReadOnly)
	if err != nil {
		return err
	}
	defer db.Close()
	return sqlitedb.Backup(ctx, db, destination)
}
