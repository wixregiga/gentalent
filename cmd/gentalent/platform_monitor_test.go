package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	aicommand "gentalent/internal/ai/app/command"
	aidomain "gentalent/internal/ai/domain"
	"gentalent/internal/ai/infra/programs"
	aiport "gentalent/internal/ai/port"
	mcommand "gentalent/internal/messaging/app/command"
	mdomain "gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/infra/beeper"
	mfiles "gentalent/internal/messaging/infra/files"
	"gentalent/internal/messaging/port"
)

type monitorTestServer struct {
	mu          sync.Mutex
	appVersion  string
	specVersion string
	messages    []string
	sent        []string
}

func (s *monitorTestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	appVersion := s.appVersion
	specVersion := s.specVersion
	msgs := append([]string(nil), s.messages...)
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Beeper-Desktop-Version", appVersion)
	switch {
	case r.URL.EscapedPath() == "/v1/info":
		fmtVersion := fmtJSONString(appVersion)
		w.Write([]byte(`{"app":{"bundle_id":"com.automattic.beeper.desktop","name":"Beeper","version":` + fmtVersion + `},"server":{"status":"running"},"platform":{"os":"linux"},"endpoints":{"spec":"/v1/spec"}}`))
	case r.URL.EscapedPath() == "/v1/spec":
		fmtSpec := fmtJSONString(specVersion)
		w.Write([]byte(`{"info":{"version":` + fmtSpec + `},"components":{"schemas":{"SendStatus":{"properties":{"status":{"enum":["PENDING","SUCCESS","FAIL_RETRIABLE","FAIL_PERMANENT"]}}},"Message":{"required":["id"]}}},"paths":{"/v1/chats/{chatID}/messages/{messageID}":{"get":{"description":"Retrieve by pendingMessageID"}}}}`))
	case r.URL.EscapedPath() == "/v1/chats/group%2Fone":
		w.Write([]byte(`{"id":"group/one","accountID":"demo-account","type":"group","participants":{"hasMore":false,"items":[{"id":"blair"},{"id":"casey"}],"total":2}}`))
	case r.URL.EscapedPath() == "/v1/chats/group%2Fone/messages" && r.Method == http.MethodGet:
		if r.URL.Query().Get("cursor") == "" {
			w.Write([]byte(`{"items":[],"hasMore":false,"newestCursor":"0"}`))
			return
		}
		w.Write([]byte(`{"items":[` + strings.Join(msgs, ",") + `],"hasMore":false,"newestCursor":"1"}`))
	case r.URL.EscapedPath() == "/v1/chats/group%2Fone/messages" && r.Method == http.MethodPost:
		s.mu.Lock()
		s.sent = append(s.sent, r.URL.String())
		s.mu.Unlock()
		w.Write([]byte(`{"chatID":"group/one","pendingMessageID":"pending-1"}`))
	case strings.HasPrefix(r.URL.EscapedPath(), "/v1/chats/group%2Fone/messages/"):
		w.Write([]byte(`{"id":"final-1","chatID":"group/one","accountID":"demo-account","senderID":"alex","timestamp":"2026-11-21T15:00:00Z","sortKey":"10","type":"TEXT","text":"<p>@gentalent-bot says: ok</p>","isSender":true}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func fmtJSONString(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func TestPlatformMonitorLogsTransitionsDetectsExternalDriftAndKeepsRunning(t *testing.T) {
	srv := &monitorTestServer{appVersion: "4.3.200", specVersion: "5.0.0"}
	server := httptest.NewServer(srv)
	defer server.Close()

	dir := t.TempDir()
	cacheDir := t.TempDir()
	stateDir := filepath.Join(dir, ".state", "platforms")

	cfg := testConfig(t)
	cfg.ChatID = "group/one"
	cfg.AccountID = "demo-account"
	cfg.OwnerID = "alex"
	cfg.Members = []configMember{
		{ID: "alex", Name: "Alex", Platform: "android"},
		{ID: "blair", Name: "Blair", Platform: "iphone"},
		{ID: "casey", Name: "Casey", Platform: "unknown"},
	}
	databases, err := openWorker(t.Context(), cfg, true, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer databases.close()

	client, err := beeper.New(server.URL, "test-token", cfg.group())
	if err != nil {
		t.Fatal(err)
	}
	armer := mcommand.Armer{Platform: client, Store: databases.inbox, Now: func() time.Time { return armedAt }}
	if _, err := armer.Arm(t.Context(), true); err != nil {
		t.Fatal(err)
	}

	finder, err := aicommand.NewFinder([]aidomain.Provider{{CLI: "fake", Model: "test"}}, map[string]aiport.Agent{"fake": newBlockingAgent()}, copyingImages{}, programs.SearchFiles{})
	if err != nil {
		t.Fatal(err)
	}
	identifier, err := aicommand.NewIdentifier([]aidomain.Provider{{CLI: "fake", Model: "test"}}, map[string]aiport.WhoAgent{"fake": newBlockingAgent()}, copyingImages{})
	if err != nil {
		t.Fatal(err)
	}

	var logBuf lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	now := time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC)

	store := mfiles.NewPlatformStateStore(stateDir)
	monitorClient, err := beeper.New(server.URL, "test-token", mdomain.Group{})
	if err != nil {
		t.Fatal(err)
	}
	monitor := newPlatformMonitor(
		mcommand.PlatformUpdateCoordinator{
			Platform: beeper.NewUpdateObserver(monitorClient, cacheDir),
			State:    store,
			Now:      func() time.Time { return now },
		},
		[]port.HeaderVersionReader{client},
		nil,
		15*time.Minute,
		logger,
	)
	monitor.now = func() time.Time { return now }

	e := newEngine(cfg, databases, platform{reader: client, sender: client, delivery: client}, aiAgents{finder: finder, identifier: identifier}, logger)
	e.monitor = monitor

	if err := e.cycle(t.Context()); err != nil {
		t.Fatalf("startup cycle: %v", err)
	}
	if err := e.cycle(t.Context()); err != nil {
		t.Fatalf("second poll cycle: %v", err)
	}
	if err := e.cycle(t.Context()); err != nil {
		t.Fatalf("third poll cycle: %v", err)
	}
	if count := strings.Count(logBuf.String(), "platform compatibility verified"); count != 1 {
		t.Fatalf("expected exactly 1 startup log without poll spam, got %d:\n%s", count, logBuf.String())
	}

	writeCachedUpdate(t, cacheDir, "4.3.210", []byte("cached-update"))
	now = now.Add(16 * time.Minute)
	if err := e.cycle(t.Context()); err != nil {
		t.Fatalf("periodic cycle with pending update: %v", err)
	}
	if count := strings.Count(logBuf.String(), "pending platform update waiting for operator action"); count != 1 {
		t.Fatalf("expected pending update logged once, got %d:\n%s", count, logBuf.String())
	}

	_ = os.Remove(filepath.Join(cacheDir, "update-info.json"))
	srv.mu.Lock()
	srv.appVersion = "4.4.0"
	srv.specVersion = "6.0.0"
	srv.messages = []string{`{"id":"msg-cmd-1","chatID":"group/one","accountID":"demo-account","senderID":"blair","timestamp":"` + armedAt.Add(time.Minute).Format(time.RFC3339) + `","sortKey":"2","type":"TEXT","text":"!gtstatus","isSender":false}`}
	srv.mu.Unlock()

	if err := e.cycle(t.Context()); err != nil {
		t.Fatalf("cycle after external incompatible update must keep running: %v", err)
	}
	if count := strings.Count(logBuf.String(), "platform update requires code changes; GenTalent keeps running"); count != 1 {
		t.Fatalf("expected external incompatible warning logged once, got %d:\n%s", count, logBuf.String())
	}

	srv.mu.Lock()
	srv.messages = nil
	sentCount := len(srv.sent)
	srv.mu.Unlock()
	if sentCount != 1 {
		t.Fatalf("expected bot to continue answering chat commands normally after external drift, got sent=%d", sentCount)
	}

	if err := e.cycle(t.Context()); err != nil {
		t.Fatalf("cycle following external drift: %v", err)
	}
	if count := strings.Count(logBuf.String(), "platform update requires code changes; GenTalent keeps running"); count != 1 {
		t.Fatalf("expected no duplicate warning on repeat poll, got %d:\n%s", count, logBuf.String())
	}

	persisted, err := store.Load(t.Context(), beeper.PlatformName)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.LastExternalDriftVersion != "4.4.0" || len(persisted.BlockedVersions) != 1 || persisted.BlockedVersions[0].Version != "4.4.0" {
		t.Fatalf("unexpected persisted state after external drift: %+v", persisted)
	}
}
