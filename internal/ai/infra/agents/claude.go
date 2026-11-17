package agents

import (
	"context"

	"gentalent/internal/ai/domain"
	"gentalent/internal/ai/port"
)

type Claude struct{}

func (Claude) Run(ctx context.Context, task port.Task) (domain.Manifest, error) {
	if err := emptyRawFolder(task.Raw); err != nil {
		return domain.Manifest{}, err
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
		"--json-schema", manifestSchema,
		task.Prompt,
	}
	stdout, err := runProvider(ctx, "claude", task.Raw, "", args)
	if err != nil {
		return domain.Manifest{}, err
	}
	return decodeManifest(string(stdout))
}
