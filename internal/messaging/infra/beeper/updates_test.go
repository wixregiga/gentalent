package beeper

import (
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientCapturesDesktopVersionHeaderAndObserverReadsCacheAndSpec(t *testing.T) {
	cacheDir := t.TempDir()
	appImageBytes := []byte("stub-appimage-payload")
	sum := sha512.Sum512(appImageBytes)
	b64Digest := base64.StdEncoding.EncodeToString(sum[:])

	appImageName := "Beeper-4.3.210-x86_64.AppImage"
	if err := os.WriteFile(filepath.Join(cacheDir, appImageName), appImageBytes, 0o700); err != nil {
		t.Fatal(err)
	}
	meta := fmt.Sprintf(`{"fileName":%q,"sha512":%q,"isAdminRightsRequired":false}`, appImageName, b64Digest)
	if err := os.WriteFile(filepath.Join(cacheDir, "update-info.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Beeper-Desktop-Version", "4.3.200")
		switch r.URL.EscapedPath() {
		case "/v1/chats/group%2Fone/messages":
			fmt.Fprint(w, `{"items":[],"hasMore":false,"newestCursor":"c1"}`)
		case "/v1/info":
			fmt.Fprint(w, `{"app":{"bundle_id":"com.automattic.beeper.desktop","name":"Beeper","version":"4.3.200"},"endpoints":{"spec":"/v1/spec"},"platform":{"os":"linux"},"server":{"status":"running"}}`)
		case "/v1/spec":
			fmt.Fprint(w, `{"info":{"version":"5.1.0"},"components":{"schemas":{"SendStatus":{"properties":{"status":{"enum":["PENDING","SUCCESS","FAIL_RETRIABLE","FAIL_PERMANENT"]}}},"Message":{"required":["id"]}}},"paths":{"/v1/chats/{chatID}/messages/{messageID}":{"get":{"description":"Retrieve by pendingMessageID"}}}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	if _, err := client.Messages(t.Context(), ""); err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if got := client.LastObservedVersion(); got != "4.3.200" {
		t.Fatalf("LastObservedVersion = %q, want 4.3.200", got)
	}

	observer := NewUpdateObserver(client, cacheDir)
	fixedNow := time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC)
	observer.now = func() time.Time { return fixedNow }

	obs, err := observer.ObservePlatform(t.Context())
	if err != nil {
		t.Fatalf("ObservePlatform: %v", err)
	}
	if obs.CurrentVersion != "4.3.200" || obs.HeaderVersion != "4.3.200" || obs.SpecVersion != "5.1.0" || len(obs.ContractIssues) != 0 {
		t.Fatalf("unexpected observation: %+v", obs)
	}
	if obs.Pending == nil || !obs.Pending.Verified || obs.Pending.Version != "4.3.210" || obs.Pending.SHA512 != b64Digest {
		t.Fatalf("unexpected pending update: %+v", obs.Pending)
	}
}

func TestUpdateObserverFlagsBadSHA512TraversalAndOfflineAPI(t *testing.T) {
	t.Run("sha512 mismatch", func(t *testing.T) {
		cacheDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(cacheDir, "Beeper-4.3.210-x86_64.AppImage"), []byte("payload"), 0o700); err != nil {
			t.Fatal(err)
		}
		wrong := base64.StdEncoding.EncodeToString(make([]byte, sha512.Size))
		if err := os.WriteFile(filepath.Join(cacheDir, "update-info.json"), []byte(fmt.Sprintf(`{"fileName":"Beeper-4.3.210-x86_64.AppImage","sha512":%q}`, wrong)), 0o600); err != nil {
			t.Fatal(err)
		}
		client := testClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
		obs, err := NewUpdateObserver(client, cacheDir).ObservePlatform(t.Context())
		if err != nil {
			t.Fatalf("ObservePlatform: %v", err)
		}
		if obs.ConnectionError == "" {
			t.Fatal("expected ConnectionError when API returns 503")
		}
		if obs.Pending == nil || obs.Pending.Verified || !strings.Contains(obs.Pending.VerificationError, "SHA-512") {
			t.Fatalf("expected SHA-512 error, got %+v", obs.Pending)
		}
	})

	t.Run("path traversal filename", func(t *testing.T) {
		cacheDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(cacheDir, "update-info.json"), []byte(`{"fileName":"../Beeper-4.3.210-x86_64.AppImage","sha512":"abcd"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		pending, err := readPendingUpdate(cacheDir)
		if err != nil {
			t.Fatalf("readPendingUpdate: %v", err)
		}
		if pending == nil || pending.Verified || !strings.Contains(pending.VerificationError, "invalid fileName") {
			t.Fatalf("expected traversal rejection, got %+v", pending)
		}
	})
}
