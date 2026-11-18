package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	aicommand "gentalent/internal/ai/app/command"
	aidomain "gentalent/internal/ai/domain"
	"gentalent/internal/ai/infra/programs"
	aiport "gentalent/internal/ai/port"
	mcommand "gentalent/internal/messaging/app/command"
)

const (
	alexID  = "member-alex"
	blairID = "member-blair"
	caseyID = "member-casey"
)

var armedAt = time.Now().UTC()

func testConfig(t *testing.T) config {
	t.Helper()
	dir := t.TempDir()
	return config{
		ChatID: "group-chat", AccountID: "test-account", OwnerID: alexID,
		Members: []configMember{
			{ID: alexID, Name: "Alex", Platform: "android"},
			{ID: blairID, Name: "Blair", Platform: "iphone"},
			{ID: caseyID, Name: "Casey", Platform: "unknown"},
		},
		NominationsDatabase: filepath.Join(dir, "nominations.sqlite3"),
		MessagingDatabase:   filepath.Join(dir, "messaging.sqlite3"),
		MediaDir:            filepath.Join(dir, "media"),
		PollSeconds:         1, PhotoLayout: "individual", PhotoSearchWorkers: 2,
		GroupOpenNominationLimit: 2, MemberOpenNominationLimit: 1, SingleMessageNominationLimit: 3,
	}
}

type testBot struct {
	t         *testing.T
	cfg       config
	chat      *fakeChat
	agent     *blockingAgent
	databases *workerDatabases
	engine    *engine
}

func startBot(t *testing.T, cfg config, chat *fakeChat, agent *blockingAgent) *testBot {
	t.Helper()
	databases, err := openWorker(t.Context(), cfg, true, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { databases.close() })
	armer := mcommand.Armer{Platform: chat, Store: databases.inbox, Now: func() time.Time { return armedAt }}
	if _, err := armer.Arm(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	finder, err := aicommand.NewFinder([]aidomain.Provider{{CLI: "fake", Model: "test"}}, map[string]aiport.Agent{"fake": agent}, copyingImages{}, programs.SearchFiles{})
	if err != nil {
		t.Fatal(err)
	}
	on := platform{reader: chat, sender: chat, delivery: chat}
	e := newEngine(cfg, databases, on, finder, slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.pollEvery = 10 * time.Millisecond
	return &testBot{t: t, cfg: cfg, chat: chat, agent: agent, databases: databases, engine: e}
}

func newTestBot(t *testing.T) *testBot {
	t.Helper()
	cfg := testConfig(t)
	return startBot(t, cfg, newFakeChat(cfg.group(), armedAt), newBlockingAgent())
}

func (b *testBot) cycle() {
	b.t.Helper()
	if err := b.engine.cycle(b.t.Context()); err != nil {
		b.t.Fatalf("cycle: %v", err)
	}
}

func (b *testBot) sentTexts() []string {
	var texts []string
	for _, reply := range b.chat.delivered() {
		texts = append(texts, reply.Text)
	}
	return texts
}

func (b *testBot) waitStarted(n int) {
	b.t.Helper()
	for range n {
		select {
		case <-b.agent.started:
		case <-time.After(5 * time.Second):
			b.t.Fatal("photo search did not start")
		}
	}
}

func (b *testBot) votes(number int) int {
	b.t.Helper()
	every, err := b.databases.bot.Nominations(b.t.Context())
	if err != nil {
		b.t.Fatal(err)
	}
	for _, nomination := range every {
		if nomination.Number == number {
			return nomination.Votes
		}
	}
	b.t.Fatalf("nomination %d does not exist", number)
	return 0
}

func assertSentOnce(t *testing.T, replies []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, text := range replies {
		if seen[text] {
			t.Fatalf("reply sent twice: %q in %q", text, replies)
		}
		seen[text] = true
	}
}

func TestMessagesApplyInOrderSoStatusShowsTheVoteBeforeIt(t *testing.T) {
	b := newTestBot(t)
	b.chat.say(blairID, "!gt Grace Hopper. Wrote the first compiler")
	b.chat.say(caseyID, "!!Y")
	b.chat.say(alexID, "!gtstatus")
	b.cycle()

	sent := b.sentTexts()
	if len(sent) != 3 {
		t.Fatalf("want announcement, vote and status, got %q", sent)
	}
	for _, text := range sent {
		if !strings.HasPrefix(text, "@gentalent-bot says: ") {
			t.Fatalf("B140 every text reply carries the prefix once: %q", text)
		}
	}
	if !strings.Contains(sent[0], "Grace Hopper") || !strings.Contains(sent[1], "2/3") || !strings.Contains(sent[2], "2/3 votes in") {
		t.Fatalf("D6 replies out of message order: %q", sent)
	}
	b.cycle()
	if got := b.sentTexts(); len(got) != 3 {
		t.Fatalf("the bot answered its own replies or a handled message again: %q", got)
	}
}

func TestRelayAfterACrashBetweenStepsSendsEachReplyOnce(t *testing.T) {
	b := newTestBot(t)
	ctx := t.Context()
	e := b.engine

	b.chat.say(blairID, "!gt Grace Hopper. Wrote the first compiler")
	if err := e.poller.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := e.inbox.Next(ctx)
	if err != nil || first == nil {
		t.Fatalf("inbox: %v %v", first, err)
	}
	if err := e.bot.Handle(ctx, chatMessage(*first)); err != nil {
		t.Fatal(err)
	}
	b.cycle()

	b.chat.say(caseyID, "!!Y")
	if err := e.poller.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := e.inbox.Next(ctx)
	if err != nil || second == nil {
		t.Fatalf("inbox: %v %v", second, err)
	}
	if err := e.bot.Handle(ctx, chatMessage(*second)); err != nil {
		t.Fatal(err)
	}
	if err := e.relay(ctx); err != nil {
		t.Fatal(err)
	}
	b.cycle()

	b.chat.say(alexID, "!!N")
	if err := e.poller.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	third, err := e.inbox.Next(ctx)
	if err != nil || third == nil {
		t.Fatalf("inbox: %v %v", third, err)
	}
	if err := e.bot.Handle(ctx, chatMessage(*third)); err != nil {
		t.Fatal(err)
	}
	unrelayed, err := e.bot.Unrelayed(ctx)
	if err != nil || len(unrelayed) == 0 {
		t.Fatalf("unrelayed: %v %v", unrelayed, err)
	}
	if err := e.replies.Enqueue(ctx, []mcommand.Outgoing{{Key: unrelayed[0].Key, Text: unrelayed[0].Text}}); err != nil {
		t.Fatal(err)
	}
	b.cycle()
	b.cycle()

	sent := b.sentTexts()
	assertSentOnce(t, sent)
	if len(sent) != 3 || b.votes(1) != 3 {
		t.Fatalf("want one reply per message and every vote counted once, got %q and %d votes", sent, b.votes(1))
	}
}

func TestTwoPhotoSearchesRunWhileAVoteIsApplied(t *testing.T) {
	b := newTestBot(t)
	b.chat.say(blairID, "!!gt Grace Hopper. Wrote the first compiler")
	b.chat.say(caseyID, "!!gt Ada Lovelace. Wrote the first program")
	b.cycle()
	b.waitStarted(2)

	b.chat.say(alexID, "!!Y 1")
	b.cycle()
	sent := b.sentTexts()
	if len(sent) != 3 || !strings.Contains(sent[2], "2/3") {
		t.Fatalf("the vote waited for the photo searches: %q", sent)
	}

	close(b.agent.release)
	for range 2 {
		select {
		case result := <-b.engine.photos.results:
			if result.Failure != "" || len(result.Photos) != aidomain.PhotoCount || result.Provider != "fake:test" || result.Sources == "" {
				t.Fatalf("photo result %+v", result)
			}
			if err := b.engine.attach(t.Context(), result); err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("photo search never finished")
		}
	}
	b.cycle()
	photos := 0
	for _, reply := range b.chat.delivered() {
		if reply.Text == "" && reply.Attachment != "" {
			photos++
			if !strings.Contains(reply.Attachment, filepath.Join("media", "nominations")) {
				t.Fatalf("D26 photos belong in the nomination's folder: %s", reply.Attachment)
			}
		}
	}
	if photos != 2*aidomain.PhotoCount {
		t.Fatalf("want three photos per nomination, got %d in %+v", photos, b.chat.delivered())
	}
	if runs, _ := b.agent.counts(); runs != 2 {
		t.Fatalf("photos-searched-at-most-once-per-nomination: %d searches", runs)
	}
}

func TestShutdownCancelsSearchesAndTheNextStartAbandonsThem(t *testing.T) {
	cfg := testConfig(t)
	chat := newFakeChat(cfg.group(), armedAt)
	agent := newBlockingAgent()
	b := startBot(t, cfg, chat, agent)
	chat.say(blairID, "!!gt Grace Hopper. Wrote the first compiler")

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error)
	go func() { stopped <- b.engine.run(ctx, false) }()
	b.waitStarted(1)
	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the engine did not stop")
	}
	if _, canceled := agent.counts(); canceled != 1 {
		t.Fatal("shutdown left the photo search running")
	}
	if err := b.databases.close(); err != nil {
		t.Fatalf("close databases: %v", err)
	}

	restarted := startBot(t, cfg, chat, agent)
	if err := restarted.engine.run(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	sent := restarted.sentTexts()
	if !strings.Contains(sent[len(sent)-1], "interrupted") {
		t.Fatalf("D7 the Group is told the search was interrupted: %q", sent)
	}
	if runs, _ := agent.counts(); runs != 1 {
		t.Fatalf("D7 an interrupted search is never rerun: %d runs", runs)
	}
}

func TestAFailingMessageWaitsForTheNextPollWithoutStoppingTheBot(t *testing.T) {
	b := newTestBot(t)
	b.chat.say(blairID, "!gt Grace Hopper. Wrote the first compiler")
	b.cycle()

	gifs := b.breakGifBank()
	b.chat.say(alexID, "!gtshame @Blair for that pitch")
	b.chat.say(caseyID, "!gtstatus")
	if err := b.engine.cycle(t.Context()); err == nil {
		t.Fatal("D22 the failing message is reported")
	}
	if sent := b.sentTexts(); len(sent) != 1 {
		t.Fatalf("a later message jumped the failing one: %q", sent)
	}

	if err := os.Remove(gifs); err != nil {
		t.Fatal(err)
	}
	b.cycle()
	sent := b.sentTexts()
	if len(sent) != 3 || !strings.Contains(sent[1], "SHAME") || !strings.Contains(sent[2], "Grace Hopper") {
		t.Fatalf("D22 the failed message is retried first, then the rest: %q", sent)
	}
}

func (b *testBot) breakGifBank() string {
	b.t.Helper()
	gifs := filepath.Join(b.cfg.MediaDir, "shame-gifs")
	if err := os.MkdirAll(b.cfg.MediaDir, 0o700); err != nil {
		b.t.Fatal(err)
	}
	if err := os.WriteFile(gifs, []byte("not a folder"), 0o600); err != nil {
		b.t.Fatal(err)
	}
	return gifs
}

func TestRunOnceRaisesAnErrorAndStopsTheSearchItStarted(t *testing.T) {
	b := newTestBot(t)
	b.chat.say(blairID, "!gt Grace Hopper. Wrote the first compiler")
	b.cycle()
	b.breakGifBank()
	b.chat.say(caseyID, "!!gt Ada Lovelace. Wrote the first program")
	b.chat.say(alexID, "!gtshame @Blair for that pitch")

	stopped := make(chan error)
	go func() { stopped <- b.engine.run(t.Context(), true) }()
	select {
	case err := <-stopped:
		if err == nil {
			t.Fatal("B166 run --once lets the failure raise")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run --once hung on its photo search after an error")
	}
	if runs, canceled := b.agent.counts(); runs != canceled {
		t.Fatalf("the search started by the failed run was left running: %d runs, %d canceled", runs, canceled)
	}
}

func TestAnUnreadableAnnouncementLeavesReactionVotesAlone(t *testing.T) {
	b := newTestBot(t)
	b.chat.say(blairID, "!gt Grace Hopper. Wrote the first compiler")
	b.cycle()
	b.cycle()
	announcement := "sent-1"
	b.chat.react(announcement, caseyID, "👍🏽")
	b.cycle()
	if b.votes(1) != 2 {
		t.Fatalf("B66 a thumbs up on the announcement is a yes: %d votes", b.votes(1))
	}

	b.chat.unreadable(announcement)
	b.cycle()
	if b.votes(1) != 2 {
		t.Fatalf("B75 a failed read changed votes: %d votes", b.votes(1))
	}
}

func TestAVoteThatRepliesToAnAnnouncementCountsOnThatNomination(t *testing.T) {
	b := newTestBot(t)
	b.chat.say(blairID, "!gt Grace Hopper. Wrote the first compiler")
	b.chat.say(caseyID, "!gt Katherine Johnson. Orbital math")
	b.cycle()
	sent := b.chat.delivered()
	if len(sent) != 2 {
		t.Fatalf("want two announcements, got %q", b.sentTexts())
	}
	b.cycle()

	b.chat.say(alexID, "!!Y")
	b.chat.reply(alexID, "!!N", "sent-1")
	b.cycle()

	replies := b.sentTexts()[2:]
	if len(replies) != 2 {
		t.Fatalf("want a refusal and a vote reply, got %q", replies)
	}
	if !strings.Contains(replies[0], "Grace Hopper") || !strings.Contains(replies[0], "Katherine Johnson") {
		t.Fatalf("the unnamed vote must be refused listing both Nominations, got %q", replies[0])
	}
	if !strings.Contains(replies[1], "Grace Hopper") || strings.Contains(replies[1], "Katherine Johnson") {
		t.Fatalf("the reply to the first announcement must vote on that Nomination only, got %q", replies[1])
	}
}
