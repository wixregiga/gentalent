package port

import (
	"context"

	"gentalent/internal/ai/domain"
)

type Task struct {
	Model  string
	Prompt string
	Folder string
	Raw    string
}

type Agent interface {
	Run(ctx context.Context, task Task) (domain.Manifest, error)
}
