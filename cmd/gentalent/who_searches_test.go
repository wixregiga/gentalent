package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAWhoSearchAnswersBesideTheLaneWithOnePhoto(t *testing.T) {
	b := newTestBot(t)
	b.chat.say(blairID, "!!gtwho the woman who wrote the first computer program")
	b.cycle()
	b.waitStarted(1)

	b.chat.say(alexID, "!gt Grace Hopper. Wrote the first compiler")
	b.cycle()
	if sent := b.sentTexts(); len(sent) != 2 || !strings.Contains(sent[1], "Grace Hopper") {
		t.Fatalf("a nomination waited for the who search: %q", sent)
	}

	close(b.agent.release)
	select {
	case result := <-b.engine.who.results:
		if result.Failure != "" || result.NoMatch || result.Name != "Ada Lovelace" || result.Provider != "fake:test" {
			t.Fatalf("who result %+v", result)
		}
		if err := b.engine.receiveWho(t.Context(), result); err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the who search never finished")
	}
	b.cycle()
	delivered := b.chat.delivered()
	answer := delivered[len(delivered)-1]
	if !strings.Contains(answer.Text, "Ada Lovelace") || !strings.Contains(answer.Text, "Not in GenTalent yet.") {
		t.Fatalf("the answer names the public figure and where they stand: %q", answer.Text)
	}
	if strings.Contains(answer.Text, "example.test") {
		t.Fatalf("the source link stays out of the reply: %q", answer.Text)
	}
	if !strings.Contains(answer.Attachment, filepath.Join("media", "who", "1")) {
		t.Fatalf("one photo from the search's own folder is attached: %q", answer.Attachment)
	}
	if photos := b.photoReplies(); photos != 0 {
		t.Fatalf("a who search sends no photo strip or extra photos, got %d", photos)
	}
	if identified := b.agent.identifications(); identified != 1 {
		t.Fatalf("a who search runs its AI once, ran %d", identified)
	}
}

func TestShutdownAbandonsARunningWhoSearchAndItIsNeverRerun(t *testing.T) {
	cfg := testConfig(t)
	chat := newFakeChat(cfg.group(), armedAt)
	agent := newBlockingAgent()
	b := startBot(t, cfg, chat, agent)
	chat.say(blairID, "!!gtwho the woman who wrote the first computer program")

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
		t.Fatal("shutdown left the who search running")
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
		t.Fatalf("the Group is told the who search was interrupted: %q", sent)
	}
	if identified := agent.identifications(); identified != 1 {
		t.Fatalf("an interrupted who search is never rerun: %d runs", identified)
	}
}
