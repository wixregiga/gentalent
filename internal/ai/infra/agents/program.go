package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	providerTimeout = 180 * time.Second
	exitTextLimit   = 400
	waitDelay       = 2 * time.Second
)

var strippedEnvironment = []string{
	"OPENAI_API_KEY",
	"ANTHROPIC_API_KEY",
	"CLAUDE_API_KEY",
	"GEMINI_API_KEY",
	"GOOGLE_API_KEY",
	"ELECTRON_RUN_AS_NODE",
}

type call struct {
	exe     string
	args    []string
	dir     string
	stdin   string
	timeout time.Duration
}

func subscriptionEnvironment(base []string) []string {
	kept := make([]string, 0, len(base))
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if !contains(strippedEnvironment, name) {
			kept = append(kept, entry)
		}
	}
	return kept
}

func runProvider(ctx context.Context, cli, dir, stdin string, args []string) ([]byte, error) {
	exe, err := exec.LookPath(cli)
	if err != nil {
		return nil, fmt.Errorf("%s CLI is not installed or on PATH", cli)
	}
	return runProgram(ctx, call{exe: exe, args: args, dir: dir, stdin: stdin, timeout: providerTimeout})
}

func runProgram(ctx context.Context, c call) ([]byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, c.exe, c.args...)
	cmd.Dir = c.dir
	cmd.Env = subscriptionEnvironment(os.Environ())
	if c.stdin != "" {
		cmd.Stdin = strings.NewReader(c.stdin)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = waitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	name := filepath.Base(c.exe)
	var exitFailure *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case runCtx.Err() != nil:
		return nil, fmt.Errorf("%s timed out after %s", name, c.timeout)
	case errors.As(err, &exitFailure):
		return nil, fmt.Errorf("%s exited %d: %s", name, exitFailure.ExitCode(), firstText(stderr.String(), stdout.String()))
	case err != nil:
		return nil, fmt.Errorf("running %s: %w", name, err)
	}
	return stdout.Bytes(), nil
}

func firstText(preferred, fallback string) string {
	text := strings.TrimSpace(preferred)
	if text == "" {
		text = strings.TrimSpace(fallback)
	}
	if len(text) > exitTextLimit {
		text = text[:exitTextLimit]
	}
	return text
}

func contains(list []string, item string) bool {
	for _, candidate := range list {
		if candidate == item {
			return true
		}
	}
	return false
}
