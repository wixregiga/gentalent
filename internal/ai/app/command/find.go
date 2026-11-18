package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"gentalent/internal/ai/domain"
	"gentalent/internal/ai/port"
)

type Finder struct {
	providers []domain.Provider
	agents    map[string]port.Agent
	images    port.ImageTool
	record    port.SearchRecorder
}

func NewFinder(providers []domain.Provider, agents map[string]port.Agent, images port.ImageTool, record port.SearchRecorder) (*Finder, error) {
	if len(providers) == 0 {
		return nil, errors.New("no AI providers configured")
	}
	for _, provider := range providers {
		if agents[provider.CLI] == nil {
			return nil, fmt.Errorf("unsupported CLI provider: %s", provider.CLI)
		}
	}
	return &Finder{providers: providers, agents: agents, images: images, record: record}, nil
}

func (f *Finder) Find(ctx context.Context, subject domain.Subject) (domain.Result, error) {
	prompt, err := searchPrompt(subject)
	if err != nil {
		return domain.Result{}, err
	}
	var failures []string
	for _, provider := range f.providers {
		result, err := f.attempt(ctx, provider, subject, prompt)
		if err == nil {
			return result, nil
		}
		if ctx.Err() != nil {
			return domain.Result{}, fmt.Errorf("photo search interrupted: %w", ctx.Err())
		}
		failures = append(failures, fmt.Sprintf("%s (%s): %v", provider.CLI, provider.Model, err))
	}
	return domain.Result{}, errors.New("all configured subscription AI providers failed: " + strings.Join(failures, " | "))
}

func (f *Finder) attempt(ctx context.Context, provider domain.Provider, subject domain.Subject, prompt string) (domain.Result, error) {
	manifest, err := f.agents[provider.CLI].Run(ctx, port.Task{
		Model:  provider.Model,
		Prompt: prompt,
		Folder: subject.Dir,
		Raw:    subject.Raw(),
	})
	if err != nil {
		return domain.Result{}, err
	}
	for _, source := range manifest.Photos {
		if err := f.images.Check(ctx, filepath.Join(subject.Raw(), source.File)); err != nil {
			return domain.Result{}, fmt.Errorf("photo %s: %w", source.File, err)
		}
	}
	photos := make([]string, len(manifest.Photos))
	for i, source := range manifest.Photos {
		photos[i] = subject.PhotoPath(i + 1)
		if err := f.images.Optimize(ctx, filepath.Join(subject.Raw(), source.File), photos[i]); err != nil {
			return domain.Result{}, fmt.Errorf("optimizing photo %d: %w", i+1, err)
		}
	}
	if err := f.images.Stitch(ctx, photos, subject.StripPath()); err != nil {
		return domain.Result{}, fmt.Errorf("creating the strip: %w", err)
	}
	result := domain.Result{CanonicalName: manifest.CanonicalName, Photos: photos, Strip: subject.StripPath(), Sources: subject.SourcesPath(), Provider: provider.Label(), Summary: manifest.Summary}
	if err := f.record.Record(subject, manifest, result); err != nil {
		return domain.Result{}, fmt.Errorf("recording the search: %w", err)
	}
	return result, nil
}

type searchRequest struct {
	UserMessage   string        `json:"user_message"`
	Exhibit       searchExhibit `json:"exhibit"`
	TargetRawDir  string        `json:"target_raw_dir"`
	ExpectedFiles []string      `json:"expected_files"`
}

type searchExhibit struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

const searchInstruction = "Find 3 verified representative photos of the specified person, verify all 3 show the right " +
	"person (replace any wrong-person image), download them into target_raw_dir as 01.jpg, 02.jpg, " +
	"and 03.jpg (or .png/.webp if needed), and return ONLY JSON matching the output schema. " +
	"Do not inspect parent directories outside target_raw_dir."

func searchPrompt(subject domain.Subject) (string, error) {
	encoded, err := json.Marshal(searchRequest{
		UserMessage:   searchInstruction,
		Exhibit:       searchExhibit{Name: subject.Name, Message: subject.Pitch},
		TargetRawDir:  subject.Raw(),
		ExpectedFiles: []string{"01.jpg", "02.jpg", "03.jpg"},
	})
	if err != nil {
		return "", fmt.Errorf("encoding the search prompt: %w", err)
	}
	return string(encoded), nil
}
