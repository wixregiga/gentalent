package worker

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func LockDatabase(path string) (*os.File, error) {
	return lock(path+".lock", "another GenTalent operation is using this database")
}

func LockLiveGroup(binding string) (*os.File, error) {
	sum, err := hex.DecodeString(binding)
	if err != nil || len(sum) != 32 {
		return nil, fmt.Errorf("invalid group binding")
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		state = filepath.Join(home, ".local", "state")
	}
	path := filepath.Join(state, "gentalent", "live-"+binding[:16]+".lock")
	return lock(path, "a live GenTalent bot is already running for this group on this machine")
}

func DryRunDatabase(path string) string {
	return filepath.Join(filepath.Dir(path), "dryrun-"+filepath.Base(path))
}

func lock(path, busy string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("open worker lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.New(busy)
		}
		return nil, fmt.Errorf("take worker lock: %w", err)
	}
	return file, nil
}
