package programs

import (
	"context"

	"gentalent/internal/ai/domain"
	"gentalent/internal/ai/port"
)

type Claude struct{}

func (Claude) Run(ctx context.Context, task port.Task) (domain.Manifest, error) {
	output, err := runClaude(ctx, task, photoSearch)
	if err != nil {
		return domain.Manifest{}, err
	}
	return decodeManifest(output)
}

func (Claude) Identify(ctx context.Context, task port.Task) (domain.WhoAnswer, error) {
	output, err := runClaude(ctx, task, whoSearch)
	if err != nil {
		return domain.WhoAnswer{}, err
	}
	return decodeWhoAnswer(output)
}

func runClaude(ctx context.Context, task port.Task, j job) (string, error) {
	if err := emptyRawFolder(task.Raw); err != nil {
		return "", err
	}
	args := []string{
		"-p",
		"--model", task.Model,
		"--effort", "low",
		"--no-session-persistence",
		"--disable-slash-commands",
		"--permission-mode", "bypassPermissions",
		"--add-dir", task.Raw,
		"--output-format", "json",
		"--json-schema", j.schema,
		task.Prompt,
	}
	stdout, err := runProvider(ctx, "claude", task.Raw, "", args, j.timeout)
	if err != nil {
		return "", err
	}
	return string(stdout), nil
}
