package programs

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
	output, err := runCodex(ctx, task, photoSearch)
	if err != nil {
		return domain.Manifest{}, err
	}
	return decodeManifest(output)
}

func (Codex) Identify(ctx context.Context, task port.Task) (domain.WhoAnswer, error) {
	output, err := runCodex(ctx, task, whoSearch)
	if err != nil {
		return domain.WhoAnswer{}, err
	}
	return decodeWhoAnswer(output)
}

func runCodex(ctx context.Context, task port.Task, j job) (string, error) {
	if err := emptyRawFolder(task.Raw); err != nil {
		return "", err
	}
	schema, err := writeSchema(task.Folder, j)
	if err != nil {
		return "", err
	}
	outputPath := filepath.Join(task.Folder, codexOutputFile)
	if err := os.Remove(outputPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("removing the previous codex output: %w", err)
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
	stdout, err := runProvider(ctx, "codex", task.Raw, task.Prompt, args, j.timeout)
	if err != nil {
		return "", err
	}
	output, err := os.ReadFile(outputPath)
	if errors.Is(err, os.ErrNotExist) {
		return string(stdout), nil
	}
	if err != nil {
		return "", fmt.Errorf("reading the codex output: %w", err)
	}
	return string(output), nil
}
