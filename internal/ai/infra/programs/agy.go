package programs

import (
	"context"

	"gentalent/internal/ai/domain"
	"gentalent/internal/ai/port"
)

type Agy struct{}

func (Agy) Run(ctx context.Context, task port.Task) (domain.Manifest, error) {
	output, err := runAgy(ctx, task, photoSearch)
	if err != nil {
		return domain.Manifest{}, err
	}
	return decodeManifest(output)
}

func (Agy) Identify(ctx context.Context, task port.Task) (domain.WhoAnswer, error) {
	output, err := runAgy(ctx, task, whoSearch)
	if err != nil {
		return domain.WhoAnswer{}, err
	}
	return decodeWhoAnswer(output)
}

func runAgy(ctx context.Context, task port.Task, j job) (string, error) {
	if err := emptyRawFolder(task.Raw); err != nil {
		return "", err
	}
	schema, err := writeSchema(task.Folder, j)
	if err != nil {
		return "", err
	}
	args := []string{
		"-p",
		"--model", task.Model,
		"--effort", "low",
		"--dangerously-skip-permissions",
		"--disable-slash-commands",
		"--add-dir", task.Raw,
		"--output-format", "json",
		"--json-schema", schema,
		task.Prompt,
	}
	stdout, err := runProvider(ctx, "agy", task.Raw, "", args, j.timeout)
	if err != nil {
		return "", err
	}
	return string(stdout), nil
}
