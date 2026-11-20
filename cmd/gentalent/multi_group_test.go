package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	aicommand "gentalent/internal/ai/app/command"
	aidomain "gentalent/internal/ai/domain"
	"gentalent/internal/ai/infra/programs"
	aiport "gentalent/internal/ai/port"
	mdomain "gentalent/internal/messaging/domain"
	mcommand "gentalent/internal/messaging/app/command"
	"gentalent/internal/nominations/delivery/chat"
)

func writeMultiGroupConfig(t *testing.T, dir string, change func(map[string]any)) string {
	t.Helper()
	values := map[string]any{
		"beeper_url":                   "http://localhost:23373",
		"account_id":                   "acct-default",
		"owner_id":                     alexID,
		"token_file":                   "secret/beeper.token",
		"media_dir":                    ".state/media",
		"photo_layout":                 "stitched",
		"announce_reaction_votes":      true,
		"group_open_nomination_limit":  2,
		"allow_renomination":           true,
		"who_search_daily_limit":       5,
		"photo_search_workers":         2,
		"ai_providers":                 []map[string]string{{"cli": "codex", "model": "test"}},
		"groups": []map[string]any{
			{
				"name":                 "alpha",
				"chat_id":              "chat-alpha",
				"nominations_database": ".state/nominations.sqlite3",
				"messaging_database":   ".state/messaging.sqlite3",
				"members": []map[string]string{
					{"id": alexID, "name": "Alex", "platform": "android"},
					{"id": blairID, "name": "Blair", "platform": "iphone"},
				},
			},
			{
				"name":                    "beta",
				"chat_id":                 "chat-beta",
				"account_id":              "acct-beta",
				"owner_id":                blairID,
				"photo_layout":            "individual",
				"announce_reaction_votes": false,
				"allow_renomination":      false,
				"who_search_daily_limit":  0,
				"members": []map[string]string{
					{"id": alexID, "name": "Alex", "platform": "android"},
					{"id": blairID, "name": "Blair", "platform": "iphone"},
					{"id": caseyID, "name": "Casey", "platform": "unknown"},
					{"id": devonID, "name": "Devon", "platform": "android"},
				},
			},
		},
	}
	if change != nil {
		change(values)
	}
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMultiGroupConfigResolvesDefaultsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	cfg, err := loadConfig(writeMultiGroupConfig(t, dir, nil))
	if err != nil {
		t.Fatal(err)
	}
	groups, multi, err := cfg.selectGroups("")
	if err != nil || !multi || len(groups) != 2 {
		t.Fatalf("selectGroups: multi=%v len=%d err=%v", multi, len(groups), err)
	}

	alpha, beta := groups[0], groups[1]
	if alpha.NominationsDatabase != filepath.Join(dir, ".state", "nominations.sqlite3") ||
		alpha.MessagingDatabase != filepath.Join(dir, ".state", "messaging.sqlite3") {
		t.Fatalf("alpha explicit DB paths = %s, %s", alpha.NominationsDatabase, alpha.MessagingDatabase)
	}
	if beta.NominationsDatabase != filepath.Join(dir, ".state", "groups", "beta", "nominations.sqlite3") ||
		beta.MessagingDatabase != filepath.Join(dir, ".state", "groups", "beta", "messaging.sqlite3") {
		t.Fatalf("beta default DB paths = %s, %s", beta.NominationsDatabase, beta.MessagingDatabase)
	}
	if alpha.MediaDir != filepath.Join(dir, ".state", "media", "groups", "alpha") ||
		beta.MediaDir != filepath.Join(dir, ".state", "media", "groups", "beta") {
		t.Fatalf("per-group media dirs = %s, %s", alpha.MediaDir, beta.MediaDir)
	}
	wantGif := filepath.Join(dir, ".state", "media", "shame-gifs")
	if alpha.GifDir != wantGif || beta.GifDir != wantGif {
		t.Fatalf("shared gif dir = %s, %s, want %s", alpha.GifDir, beta.GifDir, wantGif)
	}
	if alpha.AccountID != "acct-default" || alpha.OwnerID != alexID || !alpha.AllowRenomination || alpha.PhotoLayout != chat.Stitched || !alpha.AnnounceReactionVotes || alpha.GroupOpenNominationLimit != 2 || alpha.WhoSearchDailyLimit != 5 {
		t.Fatalf("alpha inherited settings wrong: %+v", alpha)
	}
	if beta.AccountID != "acct-beta" || beta.OwnerID != blairID || beta.AllowRenomination || beta.PhotoLayout != chat.Individual || beta.AnnounceReactionVotes || beta.GroupOpenNominationLimit != 2 || beta.WhoSearchDailyLimit != 0 {
		t.Fatalf("beta overrides wrong: %+v", beta)
	}
	if err := cfg.validateAllMedia(groups); err != nil {
		t.Fatalf("validateAllMedia: %v", err)
	}

	onlyBeta, multiBeta, err := cfg.selectGroups("beta")
	if err != nil || multiBeta || len(onlyBeta) != 1 || onlyBeta[0].Name != "beta" {
		t.Fatalf("selectGroups(beta) = %+v, multi=%v, err=%v", onlyBeta, multiBeta, err)
	}
	if _, _, err := cfg.selectGroups("missing"); err == nil || !strings.Contains(err.Error(), `unknown group "missing"`) {
		t.Fatalf("unknown group error = %v", err)
	}
}

func TestMultiGroupConfigRefusesInvalidConfigurations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
		want   string
	}{
		{"empty groups", func(v map[string]any) { v["groups"] = []any{} }, "at least one group"},
		{"top-level chat_id mixed with groups", func(v map[string]any) { v["chat_id"] = "flat-chat" }, "move chat_id and members"},
		{"invalid slug name", func(v map[string]any) { v["groups"].([]map[string]any)[0]["name"] = "../escape" }, "groups[0].name"},
		{"case-insensitive duplicate name", func(v map[string]any) { v["groups"].([]map[string]any)[1]["name"] = "Alpha" }, "duplicate group name"},
		{"duplicate chat_id", func(v map[string]any) { v["groups"].([]map[string]any)[1]["chat_id"] = "chat-alpha" }, "duplicate chat_id"},
		{"shared database across groups", func(v map[string]any) {
			v["groups"].([]map[string]any)[1]["nominations_database"] = ".state/nominations.sqlite3"
		}, "used by both alpha and beta"},
		{"same nominations and messaging database", func(v map[string]any) {
			v["groups"].([]map[string]any)[1]["nominations_database"] = "same.sqlite3"
			v["groups"].([]map[string]any)[1]["messaging_database"] = "same.sqlite3"
		}, "group beta: nominations_database and messaging_database must be distinct"},
		{"invalid setting names group and key", func(v map[string]any) {
			v["groups"].([]map[string]any)[1]["single_message_nomination_limit"] = 11
		}, "group beta: single_message_nomination_limit is 11"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadConfig(writeMultiGroupConfig(t, t.TempDir(), tc.change))
			if err == nil {
				err = cfg.validateGroup()
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

type slotProbeAgent struct {
	mu            sync.Mutex
	active        int
	maxConcurrent int
	runs          int
	started       chan string
	release       chan struct{}
}

func newSlotProbeAgent() *slotProbeAgent {
	return &slotProbeAgent{started: make(chan string, 10), release: make(chan struct{})}
}

func (a *slotProbeAgent) Run(ctx context.Context, task aiport.Task) (aidomain.Manifest, error) {
	a.mu.Lock()
	a.runs++
	a.active++
	if a.active > a.maxConcurrent {
		a.maxConcurrent = a.active
	}
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.active--
		a.mu.Unlock()
	}()
	if err := os.MkdirAll(task.Raw, 0o700); err != nil {
		return aidomain.Manifest{}, err
	}
	a.started <- task.Folder
	select {
	case <-a.release:
	case <-ctx.Done():
		return aidomain.Manifest{}, ctx.Err()
	}
	photos := make([]aidomain.PhotoSource, aidomain.PhotoCount)
	for i := range photos {
		name := fmt.Sprintf("0%d.jpg", i+1)
		if err := os.WriteFile(filepath.Join(task.Raw, name), []byte("raw"), 0o600); err != nil {
			return aidomain.Manifest{}, err
		}
		photos[i] = aidomain.PhotoSource{File: name, PageURL: "https://example.test/page", ImageURL: "https://example.test/image", Caption: "Shot"}
	}
	return aidomain.NewManifest("Ada Lovelace", "Pioneer.", photos)
}

func (a *slotProbeAgent) Identify(context.Context, aiport.Task) (aidomain.WhoAnswer, error) {
	return aidomain.WhoAnswer{}, nil
}

type faultyChat struct {
	*fakeChat
	mu        sync.Mutex
	verifyErr error
	checks    atomic.Int32
}

func (f *faultyChat) VerifyChat(ctx context.Context) error {
	f.checks.Add(1)
	f.mu.Lock()
	err := f.verifyErr
	f.mu.Unlock()
	if err != nil {
		return err
	}
	return f.fakeChat.VerifyChat(ctx)
}

func TestConcurrentEnginesIsolateStateAndMediaWhileSharingWorkerCap(t *testing.T) {
	dir := t.TempDir()
	cfg, err := loadConfig(writeMultiGroupConfig(t, dir, func(v map[string]any) {
		v["poll_seconds"] = 1.0
		v["photo_search_workers"] = 1
		v["photo_layout"] = "individual"
	}))
	if err != nil {
		t.Fatal(err)
	}
	groups, _, err := cfg.selectGroups("")
	if err != nil {
		t.Fatal(err)
	}

	probe := newSlotProbeAgent()
	finder, err := aicommand.NewFinder([]aidomain.Provider{{CLI: "fake", Model: "test"}}, map[string]aiport.Agent{"fake": probe}, copyingImages{}, programs.SearchFiles{})
	if err != nil {
		t.Fatal(err)
	}
	identifier, err := aicommand.NewIdentifier([]aidomain.Provider{{CLI: "fake", Model: "test"}}, map[string]aiport.WhoAgent{"fake": probe}, copyingImages{})
	if err != nil {
		t.Fatal(err)
	}
	ai := aiAgents{finder: finder, identifier: identifier}

	photoSlots := make(chan struct{}, cfg.PhotoSearchWorkers)
	whoSlots := make(chan struct{}, cfg.PhotoSearchWorkers)
	chats := make([]*fakeChat, len(groups))
	dbs := make([]*workerDatabases, len(groups))
	engines := make([]*engine, len(groups))
	names := make([]string, len(groups))

	for i, g := range groups {
		db, err := openWorker(t.Context(), g, true, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		defer db.close()
		fc := newFakeChat(g.group(), armedAt)
		if _, err := (mcommand.Armer{Platform: fc, Store: db.inbox, Now: func() time.Time { return armedAt }}).Arm(t.Context(), true); err != nil {
			t.Fatal(err)
		}
		on := platform{reader: fc, sender: fc, delivery: fc}
		e := newEngineWithSlots(g, db, on, ai, photoSlots, whoSlots, slog.New(slog.NewTextHandler(io.Discard, nil)))
		e.pollEvery = 20 * time.Millisecond
		chats[i], dbs[i], engines[i], names[i] = fc, db, e, g.Name
	}

	chats[0].say(alexID, "!!gt Ada Lovelace. First compiler")
	chats[1].say(blairID, "!!gt Ada Lovelace. Second group")

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- runEngines(ctx, engines, names, false)
	}()

	select {
	case <-probe.started:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("first photo search did not start")
	}

	select {
	case second := <-probe.started:
		cancel()
		t.Fatalf("photo_search_workers=1 allowed concurrent search %q", second)
	case <-time.After(80 * time.Millisecond):
	}

	close(probe.release)
	select {
	case <-probe.started:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("second photo search did not start after release")
	}

	waitPhotos := func(fc *fakeChat, wantDir string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			photos := 0
			for _, msg := range fc.delivered() {
				if msg.Attachment != "" && msg.Text == "" {
					if !strings.Contains(msg.Attachment, wantDir) {
						t.Fatalf("photo %q escaped group media dir %q", msg.Attachment, wantDir)
					}
					photos++
				}
			}
			if photos == aidomain.PhotoCount {
				return
			}
			time.Sleep(15 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %d photos in %s; got %+v", aidomain.PhotoCount, wantDir, fc.delivered())
	}

	waitPhotos(chats[0], filepath.Join("groups", "alpha", "nominations", "1-ada-lovelace"))
	waitPhotos(chats[1], filepath.Join("groups", "beta", "nominations", "1-ada-lovelace"))

	chats[0].say(blairID, "!!Y 1")
	chats[0].say(alexID, "!final 1")

	chats[1].say(alexID, "!!Y 1")
	chats[1].say(caseyID, "!!N 1")
	chats[1].say(devonID, "!!Y 1")
	chats[1].say(blairID, "!final 1")

	waitVerdict := func(db *workerDatabases, wantStatus string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			list, err := db.bot.Nominations(t.Context())
			if err == nil && len(list) == 1 && list[0].Status == wantStatus {
				return
			}
			time.Sleep(15 * time.Millisecond)
		}
		list, _ := db.bot.Nominations(t.Context())
		t.Fatalf("timed out waiting for status %q, got %+v", wantStatus, list)
	}

	waitVerdict(dbs[0], "certified")
	waitVerdict(dbs[1], "rejected")

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("runEngines shutdown: %v", err)
	}
	probe.mu.Lock()
	maxC, runs := probe.maxConcurrent, probe.runs
	probe.mu.Unlock()
	if maxC != 1 || runs != 2 {
		t.Fatalf("shared slot cap violated: maxConcurrent=%d runs=%d", maxC, runs)
	}
}

func TestFaultInOneGroupDoesNotStopOtherGroups(t *testing.T) {
	dir := t.TempDir()
	cfg, err := loadConfig(writeMultiGroupConfig(t, dir, func(v map[string]any) {
		v["poll_seconds"] = 1.0
	}))
	if err != nil {
		t.Fatal(err)
	}
	groups, _, err := cfg.selectGroups("")
	if err != nil {
		t.Fatal(err)
	}
	agent := newBlockingAgent()
	finder, err := aicommand.NewFinder([]aidomain.Provider{{CLI: "fake", Model: "test"}}, map[string]aiport.Agent{"fake": agent}, copyingImages{}, programs.SearchFiles{})
	if err != nil {
		t.Fatal(err)
	}
	identifier, err := aicommand.NewIdentifier([]aidomain.Provider{{CLI: "fake", Model: "test"}}, map[string]aiport.WhoAgent{"fake": agent}, copyingImages{})
	if err != nil {
		t.Fatal(err)
	}
	ai := aiAgents{finder: finder, identifier: identifier}

	var logBuf lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	alphaChat := newFakeChat(groups[0].group(), armedAt)
	betaChat := &faultyChat{fakeChat: newFakeChat(groups[1].group(), armedAt), verifyErr: errors.New("beta chat unreachable")}

	var engines []*engine
	var names []string
	for i, g := range groups {
		db, err := openWorker(t.Context(), g, true, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		defer db.close()
		var p platform
		if i == 0 {
			if _, err := (mcommand.Armer{Platform: alphaChat, Store: db.inbox, Now: func() time.Time { return armedAt }}).Arm(t.Context(), true); err != nil {
				t.Fatal(err)
			}
			p = platform{reader: alphaChat, sender: alphaChat, delivery: alphaChat}
		} else {
			if _, err := (mcommand.Armer{Platform: betaChat.fakeChat, Store: db.inbox, Now: func() time.Time { return armedAt }}).Arm(t.Context(), true); err != nil {
				t.Fatal(err)
			}
			p = platform{reader: betaChat, sender: betaChat, delivery: betaChat}
		}
		e := newEngine(g, db, p, ai, logger.With("group", g.Name))
		e.pollEvery = 20 * time.Millisecond
		engines = append(engines, e)
		names = append(names, g.Name)
	}

	alphaChat.say(alexID, "!gt Grace Hopper. Compiler")

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runEngines(ctx, engines, names, false) }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(alphaChat.delivered()) > 0 && betaChat.checks.Load() >= 2 {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("runEngines error: %v", err)
	}
	if len(alphaChat.delivered()) != 1 || !strings.Contains(alphaChat.delivered()[0].Text, "Grace Hopper") {
		t.Fatalf("alpha did not keep running while beta failed: %+v", alphaChat.delivered())
	}
	if !strings.Contains(logBuf.String(), "beta chat unreachable") {
		t.Fatalf("expected beta error logged, got %q", logBuf.String())
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type multiBeeperServer struct {
	mu           sync.Mutex
	betaMismatch bool
}

func (s *multiBeeperServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.EscapedPath() {
	case "/v1/info":
		fmt.Fprint(w, `{"app":{"version":"test"},"server":{"status":"running"},"platform":{"os":"linux"},"endpoints":{"spec":"/v1/spec"}}`)
	case "/v1/accounts":
		fmt.Fprint(w, capturedAccounts)
	case "/v1/spec":
		fmt.Fprint(w, observedSpec)
	case "/v1/chats/chat-alpha":
		fmt.Fprint(w, `{"id":"chat-alpha","accountID":"acct-default","type":"group","participants":{"hasMore":false,"items":[{"id":"member-alex"},{"id":"member-blair"}],"total":2}}`)
	case "/v1/chats/chat-beta":
		s.mu.Lock()
		mismatch := s.betaMismatch
		s.mu.Unlock()
		if mismatch {
			fmt.Fprint(w, `{"id":"chat-beta","accountID":"acct-beta","type":"group","participants":{"hasMore":false,"items":[{"id":"member-alex"}],"total":1}}`)
			return
		}
		fmt.Fprint(w, `{"id":"chat-beta","accountID":"acct-beta","type":"group","participants":{"hasMore":false,"items":[{"id":"member-alex"},{"id":"member-blair"},{"id":"member-casey"},{"id":"member-devon"}],"total":4}}`)
	case "/v1/chats/chat-alpha/messages":
		fmt.Fprint(w, `{"items":[],"hasMore":false,"newestCursor":"cursor-alpha"}`)
	case "/v1/chats/chat-beta/messages":
		fmt.Fprint(w, `{"items":[],"hasMore":false,"newestCursor":"cursor-beta"}`)
	case "/v1/chats/chat-alpha/messages/msg-alpha-1":
		fmt.Fprint(w, `{"id":"msg-alpha-1","chatID":"chat-alpha","accountID":"acct-default","senderID":"member-alex","timestamp":"2026-11-16T23:49:39.327Z","sortKey":"1","type":"TEXT","text":"<p>@gentalent-bot says: Hello</p>","isSender":true}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestMultiGroupCLICommands(t *testing.T) {
	srv := &multiBeeperServer{}
	server := httptest.NewServer(srv)
	defer server.Close()

	dir := t.TempDir()
	token := filepath.Join(dir, "secret", "beeper.token")
	if err := os.MkdirAll(filepath.Dir(token), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(token, []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := writeMultiGroupConfig(t, dir, func(v map[string]any) {
		v["beeper_url"] = server.URL
	})

	code, out, errs := gentalent(t, "--config", configPath, "doctor", "--chat")
	if code != 0 || !strings.Contains(out, "Exact group, account, and roster match (2 groups).") {
		t.Fatalf("doctor --chat: code=%d out=%q errs=%q", code, out, errs)
	}

	srv.mu.Lock()
	srv.betaMismatch = true
	srv.mu.Unlock()
	code, out, errs = gentalent(t, "--config", configPath, "doctor", "--chat")
	if code != 1 || out != "" || !strings.Contains(errs, "group beta:") {
		t.Fatalf("doctor --chat mismatch: code=%d out=%q errs=%q", code, out, errs)
	}
	srv.mu.Lock()
	srv.betaMismatch = false
	srv.mu.Unlock()

	code, out, errs = gentalent(t, "--config", configPath, "arm", "--live")
	if code != 0 || !strings.Contains(out, "alpha: Armed") || !strings.Contains(out, "beta: Armed") {
		t.Fatalf("arm --live: code=%d out=%q errs=%q", code, out, errs)
	}
	code, out, errs = gentalent(t, "--config", configPath, "arm", "--live", "--group", "alpha", "--if-unarmed")
	if code != 0 || out != "Already armed.\n" {
		t.Fatalf("arm --group alpha --if-unarmed: code=%d out=%q errs=%q", code, out, errs)
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	groups, _, err := cfg.selectGroups("")
	if err != nil {
		t.Fatal(err)
	}

	alphaDB, err := openWorker(t.Context(), groups[0], true, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := alphaDB.inbox.Enqueue(t.Context(), []mdomain.Outgoing{{Key: "reply-1", Text: "@gentalent-bot says: Hello"}}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		from, to mdomain.State
		id       string
	}{{mdomain.Queued, mdomain.Sending, ""}, {mdomain.Sending, mdomain.Pending, "pend-1"}, {mdomain.Pending, mdomain.Sent, "msg-alpha-1"}} {
		if _, err := alphaDB.inbox.Transition(t.Context(), "reply-1", step.from, step.to, step.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := alphaDB.bot.Handle(t.Context(), chat.Message{ID: "fb-1", Sender: alexID, Text: "!gtfeedback nice bot", SentAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := alphaDB.close(); err != nil {
		t.Fatal(err)
	}

	code, out, errs = gentalent(t, "--config", configPath, "doctor", "--contract")
	if code != 0 || !strings.Contains(out, "Beeper behavior matches doc/beeper-api.md.") {
		t.Fatalf("doctor --contract with fresh beta DB: code=%d out=%q errs=%q", code, out, errs)
	}

	betaDB, err := openWorker(t.Context(), groups[1], true, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := betaDB.bot.Handle(t.Context(), chat.Message{ID: "nom-b-1", Sender: blairID, Text: "!!gt Grace Hopper. Compiler", SentAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := betaDB.bot.ClaimPhotoSearches(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := betaDB.close(); err != nil {
		t.Fatal(err)
	}

	code, out, errs = gentalent(t, "--config", configPath, "busy", "--live")
	if code != 1 || !strings.Contains(errs, "beta (photo searches for nominations 1)") {
		t.Fatalf("busy --live: code=%d out=%q errs=%q", code, out, errs)
	}
	code, out, errs = gentalent(t, "--config", configPath, "busy", "--live", "--group", "alpha")
	if code != 0 || !strings.Contains(out, "No AI search is running.") {
		t.Fatalf("busy --live --group alpha: code=%d out=%q errs=%q", code, out, errs)
	}

	code, out, errs = gentalent(t, "--config", configPath, "status", "--live")
	if code != 0 {
		t.Fatalf("status --live: code=%d errs=%q", code, errs)
	}
	var allStatus map[string]statusJSON
	if err := json.Unmarshal([]byte(out), &allStatus); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, out)
	}
	if len(allStatus) != 2 || len(allStatus["beta"].Nominations) != 1 || allStatus["beta"].Nominations[0].Person != "Grace Hopper" {
		t.Fatalf("multi status = %+v", allStatus)
	}

	code, out, errs = gentalent(t, "--config", configPath, "status", "--live", "--group", "beta")
	if code != 0 {
		t.Fatalf("status --live --group beta: code=%d errs=%q", code, errs)
	}
	var betaStatus statusJSON
	if err := json.Unmarshal([]byte(out), &betaStatus); err != nil || len(betaStatus.Nominations) != 1 {
		t.Fatalf("single group status: err=%v out=%s", err, out)
	}

	code, out, errs = gentalent(t, "--config", configPath, "feedback", "--live")
	if code != 0 {
		t.Fatalf("feedback --live: code=%d errs=%q", code, errs)
	}
	var allFeedback map[string][]feedbackJSON
	if err := json.Unmarshal([]byte(out), &allFeedback); err != nil {
		t.Fatalf("feedback JSON: %v\n%s", err, out)
	}
	if len(allFeedback) != 2 || len(allFeedback["alpha"]) != 1 || allFeedback["alpha"][0].Text != "nice bot" || len(allFeedback["beta"]) != 0 {
		t.Fatalf("multi feedback = %+v", allFeedback)
	}

	code, out, errs = gentalent(t, "--config", configPath, "backup", "--live", "--label", "multi-snap")
	if code != 0 {
		t.Fatalf("backup --live: code=%d errs=%q", code, errs)
	}
	for _, glob := range []string{
		filepath.Join(dir, ".state", "backups", "nominations-*-multi-snap.sqlite3"),
		filepath.Join(dir, ".state", "backups", "messaging-*-multi-snap.sqlite3"),
		filepath.Join(dir, ".state", "groups", "beta", "backups", "nominations-*-multi-snap.sqlite3"),
		filepath.Join(dir, ".state", "groups", "beta", "backups", "messaging-*-multi-snap.sqlite3"),
	} {
		matches, err := filepath.Glob(glob)
		if err != nil || len(matches) != 1 {
			t.Fatalf("missing backup %s: %v %v", glob, matches, err)
		}
	}

	code, _, errs = gentalent(t, "--config", configPath, "resolve-send", "--live", "--not-delivered", "missing-key")
	if code != 2 || !strings.Contains(errs, "requires --group") {
		t.Fatalf("resolve-send without --group: code=%d errs=%q", code, errs)
	}
	code, _, errs = gentalent(t, "--config", configPath, "resolve-send", "--live", "--group", "alpha", "--not-delivered", "missing-key")
	if code != 1 || strings.Contains(errs, "requires --group") {
		t.Fatalf("resolve-send --group alpha: code=%d errs=%q", code, errs)
	}
}
