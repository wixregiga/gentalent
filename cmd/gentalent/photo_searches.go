package main

import (
	"context"
	"log/slog"
	"sync"

	aicommand "gentalent/internal/ai/app/command"
	aidomain "gentalent/internal/ai/domain"
	"gentalent/internal/nominations/delivery/chat"
)

type photoSearches struct {
	finder  *aicommand.Finder
	slots   chan struct{}
	results chan chat.PhotoResult
	group   *sync.WaitGroup
	running int
	log     *slog.Logger
}

func newPhotoSearches(finder *aicommand.Finder, workers int, log *slog.Logger) photoSearches {
	return photoSearches{finder: finder, slots: make(chan struct{}, workers), results: make(chan chat.PhotoResult), group: &sync.WaitGroup{}, log: log}
}

func (p *photoSearches) start(ctx context.Context, search chat.PhotoSearch) {
	p.running++
	p.group.Go(func() {
		result := p.find(ctx, search)
		select {
		case p.results <- result:
		case <-ctx.Done():
		}
	})
}

func (p *photoSearches) find(ctx context.Context, search chat.PhotoSearch) chat.PhotoResult {
	select {
	case p.slots <- struct{}{}:
	case <-ctx.Done():
		return chat.PhotoResult{Nomination: search.Nomination, Failure: ctx.Err().Error()}
	}
	defer func() { <-p.slots }()
	found, err := p.finder.Find(ctx, aidomain.Subject{Name: search.Person, Pitch: search.Pitch, Dir: search.Dir})
	if err != nil {
		p.log.WarnContext(ctx, "photo search failed", "nomination", search.Nomination, "error", err)
		return chat.PhotoResult{Nomination: search.Nomination, Failure: err.Error()}
	}
	return chat.PhotoResult{
		Nomination: search.Nomination, Photos: found.Photos, Strip: found.Strip, Provider: found.Provider, Sources: found.Sources,
		Summary: found.Summary,
	}
}

func (p *photoSearches) stop(cancel context.CancelFunc) {
	cancel()
	p.group.Wait()
}
