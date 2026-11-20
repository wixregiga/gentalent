package main

import (
	"context"
	"path/filepath"
	"time"

	aidomain "gentalent/internal/ai/domain"
)

type mediaJSON struct {
	NomineeName  string   `json:"nominee_name"`
	Summary      string   `json:"summary"`
	ProviderUsed string   `json:"provider_used"`
	StripPath    string   `json:"strip_path"`
	PhotoPaths   []string `json:"photo_paths"`
	SourcesPath  string   `json:"sources_path"`
}

func generateMedia(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "generate-media")
	pitch := flags.String("pitch", "", "why they are a generational talent")
	cli := flags.String("cli", "", "use only this ai_providers entry: codex, claude or agy")
	name, err := parseFlags(flags, args, "NAME")
	if err != nil {
		return err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	ai, err := aiPrograms(cfg, *cli)
	if err != nil {
		return err
	}
	dir := filepath.Join(cfg.MediaDir, "generate-media", time.Now().UTC().Format("20060102-150405.000000000"))
	found, err := ai.finder.Find(ctx, aidomain.Subject{Name: name[0], Pitch: *pitch, Dir: dir})
	if err != nil {
		return err
	}
	return printJSON(call.out, mediaJSON{
		NomineeName: found.CanonicalName, Summary: found.Summary, ProviderUsed: found.Provider, StripPath: found.Strip, PhotoPaths: found.Photos, SourcesPath: found.Sources,
	})
}
