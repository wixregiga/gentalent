package main

import (
	"context"
	"log/slog"
	"sync"

	aicommand "gentalent/internal/ai/app/command"
	aidomain "gentalent/internal/ai/domain"
	"gentalent/internal/nominations/delivery/chat"
)

type searches[S, R any] struct {
	run         func(context.Context, S) R
	interrupted func(S, error) R
	slots       chan struct{}
	results     chan R
	group       *sync.WaitGroup
	running     int
	kept        []R
}

func newSearches[S, R any](workers int, run func(context.Context, S) R, interrupted func(S, error) R) searches[S, R] {
	return newSearchesWithSlots(workers, nil, run, interrupted)
}

func newSearchesWithSlots[S, R any](workers int, slots chan struct{}, run func(context.Context, S) R, interrupted func(S, error) R) searches[S, R] {
	if slots == nil {
		slots = make(chan struct{}, workers)
	}
	return searches[S, R]{run: run, interrupted: interrupted, slots: slots, results: make(chan R), group: &sync.WaitGroup{}}
}

func (s *searches[S, R]) start(ctx context.Context, search S) {
	s.running++
	s.group.Go(func() {
		result := s.find(ctx, search)
		select {
		case s.results <- result:
		case <-ctx.Done():
		}
	})
}

func (s *searches[S, R]) find(ctx context.Context, search S) R {
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return s.interrupted(search, ctx.Err())
	}
	defer func() { <-s.slots }()
	return s.run(ctx, search)
}

func (s *searches[S, R]) stop(cancel context.CancelFunc) {
	cancel()
	s.group.Wait()
}

type photoSearches = searches[chat.PhotoSearch, chat.PhotoResult]

func newPhotoSearches(finder *aicommand.Finder, workers int, log *slog.Logger) photoSearches {
	return newPhotoSearchesWithSlots(finder, workers, nil, log)
}

func newPhotoSearchesWithSlots(finder *aicommand.Finder, workers int, slots chan struct{}, log *slog.Logger) photoSearches {
	run := func(ctx context.Context, search chat.PhotoSearch) chat.PhotoResult {
		found, err := finder.Find(ctx, aidomain.Subject{Name: search.Person, Pitch: search.Pitch, Dir: search.Dir})
		if err != nil {
			log.WarnContext(ctx, "photo search failed", "nomination", search.Nomination, "error", err)
			return chat.PhotoResult{Nomination: search.Nomination, Failure: err.Error()}
		}
		return chat.PhotoResult{
			Nomination: search.Nomination, Photos: found.Photos, Strip: found.Strip, Provider: found.Provider, Sources: found.Sources,
			Summary: found.Summary,
		}
	}
	interrupted := func(search chat.PhotoSearch, err error) chat.PhotoResult {
		return chat.PhotoResult{Nomination: search.Nomination, Failure: err.Error()}
	}
	return newSearchesWithSlots(workers, slots, run, interrupted)
}
