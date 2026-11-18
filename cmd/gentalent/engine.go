package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	aicommand "gentalent/internal/ai/app/command"
	mcommand "gentalent/internal/messaging/app/command"
	mquery "gentalent/internal/messaging/app/query"
	mdomain "gentalent/internal/messaging/domain"
	msqlite "gentalent/internal/messaging/infra/sqlite"
	"gentalent/internal/messaging/port"
	"gentalent/internal/nominations/delivery/chat"
)

type engine struct {
	bot       *chat.Bot
	inbox     *msqlite.Store
	poller    mcommand.Poller
	replies   mcommand.Enqueuer
	reactions mquery.ReactionReader
	flusher   mcommand.Flusher
	photos    photoSearches
	pollEvery time.Duration
	now       func() time.Time
	log       *slog.Logger
}

type platform struct {
	reader   port.PollReader
	sender   port.Sender
	delivery port.MessageReader
}

func newEngine(cfg config, w *workerDatabases, p platform, finder *aicommand.Finder, log *slog.Logger) *engine {
	group := cfg.group()
	return &engine{
		bot:       w.bot,
		inbox:     w.inbox,
		poller:    mcommand.Poller{Platform: p.reader, Store: w.inbox, Group: group},
		replies:   mcommand.Enqueuer{Store: w.inbox},
		reactions: mquery.ReactionReader{Replies: w.inbox, Platform: p.reader, Group: group},
		flusher:   mcommand.Flusher{Store: w.inbox, Sender: p.sender, Reader: p.delivery, Log: log},
		photos:    newPhotoSearches(finder, cfg.PhotoSearchWorkers, log),
		pollEvery: cfg.pollEvery(),
		now:       w.now,
		log:       log,
	}
}

func (e *engine) run(ctx context.Context, once bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer e.photos.stop(cancel)
	if err := e.bot.AbandonInterruptedSearches(ctx); err != nil {
		return fmt.Errorf("abandon interrupted photo searches: %w", err)
	}
	if once {
		return e.once(ctx)
	}
	next := time.NewTimer(0)
	defer next.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case result := <-e.photos.results:
			e.report(ctx, e.attach(ctx, result))
		case <-next.C:
			e.report(ctx, e.cycle(ctx))
			next.Reset(e.pollEvery)
		}
	}
}

func (e *engine) once(ctx context.Context) error {
	if err := e.cycle(ctx); err != nil {
		return err
	}
	for e.photos.running > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result := <-e.photos.results:
			if err := e.attach(ctx, result); err != nil {
				return err
			}
		}
	}
	return errors.Join(e.relay(ctx), e.flusher.Flush(ctx))
}

func (e *engine) report(ctx context.Context, err error) {
	if err != nil && ctx.Err() == nil {
		e.log.ErrorContext(ctx, "transient error; the next poll retries", "error", err)
	}
}

func (e *engine) cycle(ctx context.Context) error {
	if err := e.poller.Poll(ctx); err != nil {
		return fmt.Errorf("poll: %w", err)
	}
	inbox := e.applyInbox(ctx)
	reactions := e.countReactions(ctx)
	due := e.bot.FinalizeDue(ctx, e.now())
	relay := e.relay(ctx)
	return errors.Join(inbox, reactions, due, relay, e.flusher.Flush(ctx))
}

func (e *engine) applyInbox(ctx context.Context) error {
	for {
		incoming, err := e.inbox.Next(ctx)
		if err != nil || incoming == nil {
			return err
		}
		if err := e.apply(ctx, *incoming); err != nil {
			return fmt.Errorf("message %s waits for the next poll: %w", incoming.ID, err)
		}
		if err := e.claimPhotoSearches(ctx); err != nil {
			return err
		}
	}
}

func (e *engine) apply(ctx context.Context, incoming mdomain.Incoming) error {
	if err := e.bot.Handle(ctx, chatMessage(incoming)); err != nil {
		return err
	}
	if err := e.relay(ctx); err != nil {
		return err
	}
	return e.inbox.Handled(ctx, incoming.ID)
}

func chatMessage(incoming mdomain.Incoming) chat.Message {
	message := chat.Message{ID: incoming.ID, Sender: incoming.Sender, Text: incoming.Text, SentAt: incoming.SentAt, Mentions: incoming.Mentions}
	if linked := incoming.Linked; linked != nil {
		message.ReplyTo = &chat.Quoted{
			ID: linked.ID, Author: linked.Sender, Text: linked.Text, SentAt: linked.SentAt, HasAttachment: linked.HasAttachment,
			FromBot: linked.FromBot, Missing: linked.Missing, Announces: announcedNomination(linked.Tag),
		}
	}
	return message
}

func (e *engine) relay(ctx context.Context) error {
	replies, err := e.bot.Unrelayed(ctx)
	if err != nil || len(replies) == 0 {
		return err
	}
	outgoing := make([]mcommand.Outgoing, len(replies))
	keys := make([]string, len(replies))
	for i, reply := range replies {
		outgoing[i] = mcommand.Outgoing{Key: reply.Key, Text: reply.Text, Attachment: reply.Attachment, Tag: announcementTag(reply.Announces)}
		keys[i] = reply.Key
	}
	if err := e.replies.Enqueue(ctx, outgoing); err != nil {
		return fmt.Errorf("relay replies: %w", err)
	}
	return e.bot.MarkRelayed(ctx, keys)
}

const announcementTagPrefix = "announces/"

func announcementTag(nomination int) string {
	if nomination == 0 {
		return ""
	}
	return announcementTagPrefix + strconv.Itoa(nomination)
}

func announcedNomination(tag string) int {
	number, err := strconv.Atoi(strings.TrimPrefix(tag, announcementTagPrefix))
	if !strings.HasPrefix(tag, announcementTagPrefix) || err != nil || number < 1 {
		return 0
	}
	return number
}

func (e *engine) countReactions(ctx context.Context) error {
	open, err := e.bot.OpenAnnouncements(ctx)
	if err != nil {
		return err
	}
	for _, nomination := range open {
		byMember, err := e.reactions.Reactions(ctx, announcementTag(nomination))
		if err != nil {
			e.log.WarnContext(ctx, "reaction check skipped", "nomination", nomination, "error", err)
			continue
		}
		if byMember == nil {
			continue
		}
		if err := e.bot.CountReactions(ctx, nomination, byMember); err != nil {
			return fmt.Errorf("count reactions on nomination %d: %w", nomination, err)
		}
	}
	return nil
}

func (e *engine) claimPhotoSearches(ctx context.Context) error {
	searches, err := e.bot.ClaimPhotoSearches(ctx)
	if err != nil {
		return fmt.Errorf("claim photo searches: %w", err)
	}
	for _, search := range searches {
		e.photos.start(ctx, search)
	}
	return nil
}

func (e *engine) attach(ctx context.Context, result chat.PhotoResult) error {
	e.photos.running--
	if err := e.bot.AttachPhotos(ctx, result); err != nil {
		return fmt.Errorf("attach photos to nomination %d: %w", result.Nomination, err)
	}
	return e.relay(ctx)
}
