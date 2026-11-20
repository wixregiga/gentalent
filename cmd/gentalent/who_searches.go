package main

import (
	"context"
	"log/slog"

	aicommand "gentalent/internal/ai/app/command"
	aidomain "gentalent/internal/ai/domain"
	"gentalent/internal/nominations/delivery/chat"
)

type whoSearches = searches[chat.WhoSearch, chat.WhoResult]

func newWhoSearches(identifier *aicommand.Identifier, workers int, log *slog.Logger) whoSearches {
	run := func(ctx context.Context, search chat.WhoSearch) chat.WhoResult {
		found, err := identifier.Identify(ctx, aidomain.Description{Text: search.Description, Dir: search.Dir})
		if err != nil {
			log.WarnContext(ctx, "who search failed", "who_search", search.Number, "error", err)
			return chat.WhoResult{Number: search.Number, Failure: err.Error()}
		}
		return chat.WhoResult{
			Number: search.Number, NoMatch: !found.Found, Name: found.Name, Blurb: found.Blurb, Source: found.Source, Photo: found.Photo,
			Provider: found.Provider,
		}
	}
	interrupted := func(search chat.WhoSearch, err error) chat.WhoResult {
		return chat.WhoResult{Number: search.Number, Failure: err.Error()}
	}
	return newSearches(workers, run, interrupted)
}
