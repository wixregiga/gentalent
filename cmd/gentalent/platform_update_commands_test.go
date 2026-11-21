package main

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type dynamicBeeperServer struct {
	mu          sync.Mutex
	appVersion  string
	specVersion string
	specBody    string
	chatID      string
	accountID   string
}

func (s *dynamicBeeperServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	appVersion := s.appVersion
	specVersion := s.specVersion
	specBody := s.specBody
	chatID := s.chatID
	accountID := s.accountID
	s.mu.Unlock()

	if chatID == "" {
		chatID = "group/one"
	}
	if accountID == "" {
		accountID = "demo-account"
	}
	if specBody == "" {
		specBody = fmt.Sprintf(`{"info":{"version":%q},"components":{"schemas":{"SendStatus":{"properties":{"status":{"enum":["PENDING","SUCCESS","FAIL_RETRIABLE","FAIL_PERMANENT"]}}},"Message":{"required":["id"]}}},"paths":{"/v1/chats/{chatID}/messages/{messageID}":{"get":{"description":"Retrieve by pendingMessageID"}}}}`, specVersion)
	}

	w.Header().Set("Content-Type", "application/json")
	if appVersion != "" {
		w.Header().Set("X-Beeper-Desktop-Version", appVersion)
	}
	switch r.URL.EscapedPath() {
	case "/v1/info":
		fmt.Fprintf(w, `{"app":{"bundle_id":"com.automattic.beeper.desktop","name":"Beeper","version":%q},"server":{"status":"running"},"platform":{"os":"linux"},"endpoints":{"spec":"/v1/spec"}}`, appVersion)
	case "/v1/accounts":
		fmt.Fprint(w, capturedAccounts)
	case "/v1/spec":
		fmt.Fprint(w, specBody)
	case "/v1/chats/group%2Fone":
		fmt.Fprintf(w, `{"id":%q,"accountID":%q,"type":"group","participants":{"hasMore":false,"items":[{"id":"blair"},{"id":"casey"}],"total":2}}`, chatID, accountID)
	case "/v1/chats/group%2Fone/messages":
		fmt.Fprint(w, `{"items":[],"hasMore":false,"newestCursor":"cursor-1"}`)
	case "/v1/chats/group%2Fone/messages/739":
		fmt.Fprint(w, capturedText)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func setupPlatformCommandFixture(t *testing.T, srv *dynamicBeeperServer) (configPath, cacheDir string) {
	t.Helper()
	server := httptest.NewServer(srv)
	t.Cleanup(server.Close)
	dir := t.TempDir()
	cacheDir = t.TempDir()
	t.Setenv("BEEPER_UPDATER_CACHE_DIR", cacheDir)
	t.Setenv("GENTALENT_PLATFORM_STATE_DIR", filepath.Join(dir, ".state", "platforms"))

	token := filepath.Join(dir, "secret", "beeper.token")
	if err := os.MkdirAll(filepath.Dir(token), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(token, []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath = writeConfig(t, dir, func(values map[string]any) {
		values["beeper_url"] = server.URL
		values["chat_id"], values["account_id"], values["owner_id"] = "group/one", "demo-account", "alex"
		values["members"] = []map[string]string{
			{"id": "alex", "name": "Alex", "platform": "android"},
			{"id": "blair", "name": "Blair", "platform": "iphone"},
			{"id": "casey", "name": "Casey", "platform": "unknown"},
		}
	})
	return configPath, cacheDir
}

func writeCachedUpdate(t *testing.T, cacheDir, version string, payload []byte) {
	t.Helper()
	name := fmt.Sprintf("Beeper-%s-x86_64.AppImage", version)
	if err := os.WriteFile(filepath.Join(cacheDir, name), payload, 0o700); err != nil {
		t.Fatal(err)
	}
	sum := sha512.Sum512(payload)
	meta := fmt.Sprintf(`{"fileName":%q,"sha512":%q,"isAdminRightsRequired":false}`, name, base64.StdEncoding.EncodeToString(sum[:]))
	if err := os.WriteFile(filepath.Join(cacheDir, "update-info.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCheckUpdatesAndManagedWorkflowCLI(t *testing.T) {
	srv := &dynamicBeeperServer{appVersion: "4.3.200", specVersion: "5.1.0"}
	configPath, cacheDir := setupPlatformCommandFixture(t, srv)
	writeCachedUpdate(t, cacheDir, "4.3.210", []byte("pending-image"))

	code, out, errs := gentalent(t, "--config", configPath, "check-updates", "--json", "--cache-dir", cacheDir)
	if code != 0 {
		t.Fatalf("check-updates --json failed: code=%d errs=%s", code, errs)
	}
	var got assessmentJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode check-updates JSON: %v\n%s", err, out)
	}
	if got.CurrentVersion != "4.3.200" || got.SpecVersion != "5.1.0" || got.Compatibility != "compatible_review_recommended" {
		t.Fatalf("unexpected check-updates assessment: %+v", got)
	}
	if got.AutoInstallAllowed || got.Pending == nil || !got.Pending.Verified || got.Pending.Version != "4.3.210" {
		t.Fatalf("unexpected pending update output: %+v", got.Pending)
	}

	code, out, errs = gentalent(t, "--config", configPath, "platform-update", "assess", "--candidate", "4.3.210", "--json", "--cache-dir", cacheDir)
	if code != 0 {
		t.Fatalf("assess failed: code=%d errs=%s", code, errs)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || !got.ManagedUpdateAllowed {
		t.Fatalf("expected candidate 4.3.210 allowed: err=%v got=%+v", err, got)
	}

	srv.mu.Lock()
	srv.appVersion = "4.3.220"
	srv.specVersion = "6.0.0"
	srv.mu.Unlock()

	code, out, errs = gentalent(t, "--config", configPath, "platform-update", "verify", "--previous", "4.3.200", "--target", "4.3.220", "--json", "--cache-dir", cacheDir)
	if code != 1 {
		t.Fatalf("expected incompatible verify to exit 1, got %d out=%s errs=%s", code, out, errs)
	}
	var ver verificationJSON
	if err := json.Unmarshal([]byte(out), &ver); err != nil {
		t.Fatalf("decode verify JSON: %v\n%s", err, out)
	}
	if ver.Compatible || !ver.RequiresRollback || ver.RollbackToVersion != "4.3.200" || !ver.MarkRequiresCodeChanges {
		t.Fatalf("unexpected verification result: %+v", ver)
	}

	srv.mu.Lock()
	srv.appVersion = "4.3.200"
	srv.specVersion = "5.0.0"
	srv.mu.Unlock()

	code, out, errs = gentalent(t, "--config", configPath, "platform-update", "record-rollback", "--previous", "4.3.200", "--failed", "4.3.220", "--json")
	if code != 0 {
		t.Fatalf("record-rollback failed: code=%d errs=%s", code, errs)
	}

	code, out, errs = gentalent(t, "--config", configPath, "platform-update", "assess", "--candidate", "4.3.220", "--json", "--cache-dir", cacheDir)
	if code != 1 {
		t.Fatalf("blocked candidate should exit 1, got %d out=%s errs=%s", code, out, errs)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.ManagedUpdateAllowed || !got.CandidateBlocked {
		t.Fatalf("expected blocked candidate JSON, err=%v got=%+v", err, got)
	}

	code, out, errs = gentalent(t, "--config", configPath, "platform-update", "assess", "--candidate", "4.3.220", "--force", "--json", "--cache-dir", cacheDir)
	if code != 0 {
		t.Fatalf("forced assess should exit 0, got %d errs=%s", code, errs)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || !got.ManagedUpdateAllowed || !got.ForceBypassedBlock {
		t.Fatalf("expected force bypass JSON, err=%v got=%+v", err, got)
	}

	recordSentReply(t, configPath)
	code, out, errs = gentalent(t, "--config", configPath, "status", "--live")
	if code != 0 {
		t.Fatalf("status --live failed: code=%d errs=%s", code, errs)
	}
	var st statusJSON
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("decode status JSON: %v\n%s", err, out)
	}
	if st.PlatformUpdate == nil || !st.PlatformUpdate.RequiresCodeChanges || len(st.PlatformUpdate.BlockedVersions) != 1 || st.PlatformUpdate.BlockedVersions[0].Version != "4.3.220" {
		t.Fatalf("expected status to surface blocked platform update, got %+v", st.PlatformUpdate)
	}

	code, out, errs = gentalent(t, "--config", configPath, "check-updates", "--live", "--cache-dir", cacheDir)
	if code != 0 || !strings.Contains(out, "Blocked versions requiring code changes: 4.3.220") {
		t.Fatalf("human check-updates output: code=%d out=%q errs=%q", code, out, errs)
	}
}
