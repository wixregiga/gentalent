package chat

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gentalent/internal/nominations/app/command"
	"gentalent/internal/nominations/app/query"
	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type PhotoLayout string

const (
	Individual PhotoLayout = "individual"
	Stitched   PhotoLayout = "stitched"
)

type Member struct {
	ID   domain.MemberID
	Name string
}

type Config struct {
	Members               []Member
	Settings              domain.Settings
	PhotoLayout           PhotoLayout
	MediaDir              string
	AnnounceReactionVotes bool
	Now                   func() time.Time
}

type Ports struct {
	Groups    port.GroupStore
	Outbox    port.Outbox
	Feedback  port.FeedbackStore
	Gifs      port.GifBank
	Standing  port.StandingReader
	Names     port.NameReader
	Unrelayed port.OutboxReader
	Entries   port.FeedbackReader
}

type Bot struct {
	config     Config
	nominate   command.NominateHandler
	renominate command.RenominateHandler
	castVote   command.CastVoteHandler
	finalize   command.FinalizeHandler
	due        command.FinalizeDueHandler
	reactions  command.CountReactionsHandler
	shame      command.ShameHandler
	claim      command.ClaimPhotoSearchesHandler
	finish     command.FinishPhotoSearchHandler
	abandon    command.AbandonInterruptedSearchesHandler
	feedback   command.RecordFeedbackHandler
	answer     command.AnswerHandler
	relayed    command.MarkRelayedHandler
	standing   query.StandingHandler
	names      query.NamesHandler
	unrelayed  query.UnrelayedHandler
	entries    query.FeedbackHandler
}

func NewBot(config Config, ports Ports) *Bot {
	return &Bot{
		config:     config,
		nominate:   command.NominateHandler{Groups: ports.Groups},
		renominate: command.RenominateHandler{Groups: ports.Groups},
		castVote:   command.CastVoteHandler{Groups: ports.Groups},
		finalize:   command.FinalizeHandler{Groups: ports.Groups},
		due:        command.FinalizeDueHandler{Groups: ports.Groups},
		reactions:  command.CountReactionsHandler{Groups: ports.Groups},
		shame:      command.ShameHandler{Groups: ports.Groups, Gifs: ports.Gifs},
		claim:      command.ClaimPhotoSearchesHandler{Groups: ports.Groups},
		finish:     command.FinishPhotoSearchHandler{Groups: ports.Groups},
		abandon:    command.AbandonInterruptedSearchesHandler{Groups: ports.Groups},
		feedback:   command.RecordFeedbackHandler{Feedback: ports.Feedback},
		answer:     command.AnswerHandler{Outbox: ports.Outbox},
		relayed:    command.MarkRelayedHandler{Outbox: ports.Outbox},
		standing:   query.StandingHandler{Reader: ports.Standing},
		names:      query.NamesHandler{Reader: ports.Names},
		unrelayed:  query.UnrelayedHandler{Reader: ports.Unrelayed},
		entries:    query.FeedbackHandler{Reader: ports.Entries},
	}
}

type route struct {
	pattern *regexp.Regexp
	run     func(b *Bot, ctx context.Context, from Member, msg Message, match []string) error
}

var routes = []route{
	{regexp.MustCompile(`(?i)^!{1,2}gthelp\s*$`), (*Bot).help},
	{regexp.MustCompile(`(?i)^!{1,2}gtstatus\s*$`), (*Bot).status},
	{regexp.MustCompile(`(?is)^!{1,2}gtlist(?:\s+(.*))?$`), (*Bot).list},
	{regexp.MustCompile(`(?is)^!gtshame(?:\s+(.*))?$`), (*Bot).shameOver},
	{regexp.MustCompile(`(?is)^!gt(feedback|suggest)(?:\s+(.*))?$`), (*Bot).recordFeedback},
	{regexp.MustCompile(`(?is)^!{1,2}gtrenominate(?:\s.*)?$`), (*Bot).renominateAll},
	{regexp.MustCompile(`(?is)^!{1,2}gt(?:\s+(.*))?$`), (*Bot).nominateAll},
	{regexp.MustCompile(`(?is)^!!(?:yes|y|no|n|clear|c)(?:$|[\s!.,;:\-\x{2014}\x{2013}].*)$`), (*Bot).vote},
	{regexp.MustCompile(`(?is)^!final(?:\s+(.*))?$`), (*Bot).final},
}

func (b *Bot) Handle(ctx context.Context, msg Message) error {
	if err := b.FinalizeDue(ctx, b.config.Now()); err != nil {
		return err
	}
	from, ok := b.member(msg.Sender)
	text := strings.TrimSpace(msg.Text)
	if !ok || text == "" {
		return nil
	}
	for _, r := range routes {
		if match := r.pattern.FindStringSubmatch(text); match != nil {
			if err := r.run(b, ctx, from, msg, match); err != nil {
				return fmt.Errorf("handle message %s: %w", msg.ID, err)
			}
			return nil
		}
	}
	return nil
}

func (b *Bot) FinalizeDue(ctx context.Context, now time.Time) error {
	return b.due.Handle(ctx, command.FinalizeDue{Now: now}, present(b, eventKey("verdict", now), nil, wording.verdicts))
}

func (b *Bot) Unrelayed(ctx context.Context) ([]Reply, error) {
	stored, err := b.unrelayed.Handle(ctx)
	if err != nil {
		return nil, err
	}
	replies := make([]Reply, len(stored))
	for i, reply := range stored {
		replies[i] = Reply{Key: reply.Key, Text: reply.Text, Attachment: reply.Attachment, Announces: int(reply.Announces)}
	}
	return replies, nil
}

func (b *Bot) MarkRelayed(ctx context.Context, keys []string) error {
	return b.relayed.Handle(ctx, keys)
}

func (b *Bot) Feedback(ctx context.Context) ([]port.Feedback, error) {
	return b.entries.Handle(ctx)
}

func (b *Bot) StatusReport(ctx context.Context) (string, error) {
	standing, err := b.standing.Handle(ctx)
	if err != nil {
		return "", err
	}
	return b.statusText(standing), nil
}

func (b *Bot) SearchesRunning(ctx context.Context) ([]int, error) {
	standing, err := b.standing.Handle(ctx)
	if err != nil {
		return nil, err
	}
	running := make([]int, len(standing.Searching))
	for i, number := range standing.Searching {
		running[i] = int(number)
	}
	return running, nil
}

func (b *Bot) member(id string) (Member, bool) {
	for _, member := range b.config.Members {
		if string(member.ID) == id {
			return member, true
		}
	}
	return Member{}, false
}

func (b *Bot) say(ctx context.Context, msg Message, texts ...string) error {
	replies := make([]port.Reply, 0, len(texts))
	for _, text := range texts {
		replies = append(replies, port.Reply{Text: text})
	}
	return b.answer.Handle(ctx, msg.ID, keyed(msg.ID, replies))
}
