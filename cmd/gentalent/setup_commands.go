package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"gentalent/internal/messaging/infra/beeper"
)

const exampleConfig = "config.example.json"

func initConfig(ctx context.Context, call invocation, args []string) error {
	if _, err := parseFlags(newFlags(call, "init"), args); err != nil {
		return err
	}
	path, err := filepath.Abs(expandHome(call.config))
	if err != nil {
		return err
	}
	example, err := os.ReadFile(exampleConfig)
	if err != nil {
		return fmt.Errorf("read %s from the repository root: %w", exampleConfig, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return errors.New("config already exists; no overwrite")
	}
	if err != nil {
		return err
	}
	if _, err := file.Write(example); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	fmt.Fprintln(call.out, "Created", path, "- fill in the confirmed group and roster IDs.")
	return nil
}

func setSecret(ctx context.Context, call invocation, args []string) error {
	if _, err := parseFlags(newFlags(call, "set-secret"), args); err != nil {
		return err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	path, err := cfg.tokenFile()
	if err != nil {
		return err
	}
	fmt.Fprint(call.errs, "Beeper access token (hidden; do not paste into chat): ")
	token, err := readHidden(call.in)
	fmt.Fprintln(call.errs)
	if err != nil {
		return err
	}
	if err := beeper.WriteToken(path, token); err != nil {
		return err
	}
	fmt.Fprintln(call.out, "Token saved privately.")
	return nil
}

func readHidden(in *os.File) (string, error) {
	fd := int(in.Fd())
	if shown, err := unix.IoctlGetTermios(fd, unix.TCGETS); err == nil {
		hidden := *shown
		hidden.Lflag &^= unix.ECHO
		if err := unix.IoctlSetTermios(fd, unix.TCSETS, &hidden); err != nil {
			return "", fmt.Errorf("hide the token as it is typed: %w", err)
		}
		defer unix.IoctlSetTermios(fd, unix.TCSETS, shown)
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return "", fmt.Errorf("read the token: %w", err)
	}
	return strings.TrimSpace(line), nil
}
