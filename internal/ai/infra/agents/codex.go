package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gentalent/internal/ai/domain"
	"gentalent/internal/ai/port"
)

const codexOutputFile = "agent_output.json"

type Codex struct{}

func (Codex) Run(ctx context.Context, task port.Task) (domain.Manifest, error) {
	if err := emptyRawFolder(task.Raw); err != nil {
		return domain.Manifest{}, err
	}
	schema, err := writeSchema(task.Folder)
	if err != nil {
		return domain.Manifest{}, err
	}
	outputPath := filepath.Join(task.Folder, codexOutputFile)
	if err := os.Remove(outputPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return domain.Manifest{}, fmt.Errorf("removing the previous codex output: %w", err)
	}
	args := []string{
		"exec",
		"-m", task.Model,
		"-c", `model_reasoning_effort="low"`,
		"--ephemeral",
		"--skip-git-repo-check",
		"--sandbox", "workspace-write",
		"-c", "sandbox_workspace_write.network_access=true",
		"-C", task.Raw,
		"--output-schema", schema,
		"-o", outputPath,
		"-",
	}
	stdout, err := runProvider(ctx, "codex", task.Raw, task.Prompt, args)
	if err != nil {
		return domain.Manifest{}, err
	}
	output, err := os.ReadFile(outputPath)
	if errors.Is(err, os.ErrNotExist) {
		return decodeManifest(string(stdout))
	}
	if err != nil {
		return domain.Manifest{}, fmt.Errorf("reading the codex output: %w", err)
	}
	return decodeManifest(string(output))
}
