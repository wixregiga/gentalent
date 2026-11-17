package programs

import (
	"context"

	"gentalent/internal/ai/domain"
	"gentalent/internal/ai/port"
)

type Agy struct{}

func (Agy) Run(ctx context.Context, task port.Task) (domain.Manifest, error) {
	if err := emptyRawFolder(task.Raw); err != nil {
		return domain.Manifest{}, err
	}
	schema, err := writeSchema(task.Folder)
	if err != nil {
		return domain.Manifest{}, err
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
	stdout, err := runProvider(ctx, "agy", task.Raw, "", args)
	if err != nil {
		return domain.Manifest{}, err
	}
	return decodeManifest(string(stdout))
}
