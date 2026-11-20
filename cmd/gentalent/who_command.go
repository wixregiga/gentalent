package main

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	aidomain "gentalent/internal/ai/domain"
)

type whoJSON struct {
	Found        bool   `json:"found"`
	Name         string `json:"name"`
	Blurb        string `json:"blurb"`
	Source       string `json:"source"`
	PhotoPath    string `json:"photo_path"`
	ProviderUsed string `json:"provider_used"`
}

func whoSearch(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "who")
	cli := flags.String("cli", "", "use only this ai_providers entry: codex, claude or agy")
	description, err := parseFlags(flags, args, "DESCRIPTION")
	if err != nil {
		return err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	if cfg.MediaDir == "" {
		return errors.New("media_dir is required")
	}
	ai, err := aiPrograms(cfg, *cli)
	if err != nil {
		return err
	}
	dir := filepath.Join(cfg.MediaDir, "who-search", time.Now().UTC().Format("20060102-150405.000000000"))
	found, err := ai.identifier.Identify(ctx, aidomain.Description{Text: description[0], Dir: dir})
	if err != nil {
		return err
	}
	return printJSON(call.out, whoJSON{
		Found: found.Found, Name: found.Name, Blurb: found.Blurb, Source: found.Source, PhotoPath: found.Photo, ProviderUsed: found.Provider,
	})
}
