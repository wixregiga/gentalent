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

type Identifier struct {
	providers []domain.Provider
	agents    map[string]port.WhoAgent
	images    port.ImageTool
}

func NewIdentifier(providers []domain.Provider, agents map[string]port.WhoAgent, images port.ImageTool) (*Identifier, error) {
	if len(providers) == 0 {
		return nil, errors.New("no AI providers configured")
	}
	for _, provider := range providers {
		if agents[provider.CLI] == nil {
			return nil, fmt.Errorf("unsupported CLI provider: %s", provider.CLI)
		}
	}
	if images == nil {
		return nil, errors.New("no image tool configured")
	}
	return &Identifier{providers: providers, agents: agents, images: images}, nil
}

func (i *Identifier) Identify(ctx context.Context, description domain.Description) (domain.Identification, error) {
	if strings.TrimSpace(description.Text) == "" {
		return domain.Identification{}, errors.New("a who search needs a description")
	}
	if description.Dir == "" {
		return domain.Identification{}, errors.New("a who search needs a folder for its photo")
	}
	prompt, err := whoPrompt(description)
	if err != nil {
		return domain.Identification{}, err
	}
	var failures []string
	for _, provider := range i.providers {
		answer, err := i.agents[provider.CLI].Identify(ctx, port.Task{
			Model:  provider.Model,
			Prompt: prompt,
			Folder: description.Dir,
			Raw:    description.Raw(),
		})
		if err == nil {
			return i.identification(ctx, provider, description, answer)
		}
		if ctx.Err() != nil {
			return domain.Identification{}, fmt.Errorf("who search interrupted: %w", ctx.Err())
		}
		failures = append(failures, fmt.Sprintf("%s (%s): %v", provider.CLI, provider.Model, err))
	}
	return domain.Identification{}, errors.New("all configured subscription AI providers failed: " + strings.Join(failures, " | "))
}

func (i *Identifier) identification(ctx context.Context, provider domain.Provider, description domain.Description, answer domain.WhoAnswer) (domain.Identification, error) {
	if !answer.Found {
		return domain.Identification{Provider: provider.Label()}, nil
	}
	photo, err := i.photo(ctx, description, answer)
	if err != nil {
		return domain.Identification{}, err
	}
	return domain.Identification{
		Found:    true,
		Name:     answer.Name,
		Blurb:    answer.Blurb,
		Source:   answer.Source,
		Photo:    photo,
		Provider: provider.Label(),
	}, nil
}

func (i *Identifier) photo(ctx context.Context, description domain.Description, answer domain.WhoAnswer) (string, error) {
	if answer.PhotoFile == "" {
		return "", nil
	}
	raw := filepath.Join(description.Raw(), answer.PhotoFile)
	if err := i.images.Check(ctx, raw); err != nil {
		return "", interruption(ctx)
	}
	destination := description.PhotoPath(answer.Name)
	if err := i.images.Optimize(ctx, raw, destination); err != nil {
		return "", interruption(ctx)
	}
	return destination, nil
}

func interruption(ctx context.Context) error {
	if ctx.Err() != nil {
		return fmt.Errorf("who search interrupted: %w", ctx.Err())
	}
	return nil
}

type whoRequest struct {
	Instruction string `json:"instruction"`
	Description string `json:"description"`
	RawDir      string `json:"raw_dir"`
}

const whoInstruction = "Work in three quick steps. 1. Name the one clear, widely known public figure this description " +
	"fits. Never name a private individual. If you are unsure or no public figure clearly fits, set found to false, leave " +
	"the other fields empty and stop. 2. Download one clear photo of that person into raw_dir as photo.jpg (or .png/.webp); " +
	"their Wikipedia or Wikimedia Commons portrait is fine. Verify the file is saved before you answer. 3. Answer: name is " +
	"their usual full name; blurb is one or two sentences about their life, under 300 characters; source_url is the page " +
	"you relied on, or empty; photo_file is the saved filename, empty only if no photo of them could be downloaded at " +
	"all. No second opinions. Return ONLY JSON matching the output schema. Do not inspect folders outside raw_dir."

func whoPrompt(description domain.Description) (string, error) {
	encoded, err := json.Marshal(whoRequest{
		Instruction: whoInstruction,
		Description: strings.TrimSpace(description.Text),
		RawDir:      description.Raw(),
	})
	if err != nil {
		return "", fmt.Errorf("encoding the who prompt: %w", err)
	}
	return string(encoded), nil
}
