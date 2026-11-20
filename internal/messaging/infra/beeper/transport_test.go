package beeper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gentalent/internal/messaging/domain"
)

const verifiedChat = `{"id":"group/one","accountID":"demo-account","type":"group","participants":{"hasMore":false,"items":[{"id":"blair"},{"id":"casey"}],"total":2}}`

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	return testClientForGroup(t, domain.Group{ChatID: "group/one", AccountID: "demo-account", Owner: "alex", Members: []string{"alex", "blair", "casey"}}, h)
}

func testClientForGroup(t *testing.T, group domain.Group, h http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	client, err := New(server.URL, "test-token", group)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestFailedPostIsNeverRetried(t *testing.T) {
	var posts atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if r.URL.EscapedPath() != "/v1/chats/group%2Fone" || r.URL.Query().Get("maxParticipantCount") != "-1" {
				t.Errorf("verification request: %s", r.URL)
			}
			fmt.Fprint(w, verifiedChat)
		case http.MethodPost:
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Errorf("authorization = %q", got)
			}
			if r.URL.EscapedPath() != "/v1/chats/group%2Fone/messages" {
				t.Errorf("send request: %s", r.URL)
			}
			if posts.Add(1) == 1 {
				w.WriteHeader(503)
				fmt.Fprint(w, `{"error":"unavailable"}`)
				return
			}
			fmt.Fprint(w, `{"chatID":"group/one","pendingMessageID":"pending"}`)
		}
	})
	if _, err := client.Send(t.Context(), domain.Outgoing{Text: "reply"}); err == nil || posts.Load() != 1 {
		t.Fatalf("failed POST retried or accepted: %v, requests %d", err, posts.Load())
	}
}

func TestPhotoOnlySendUsesOneMultipartFileAndOneAttachment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo\".gif")
	if err := os.WriteFile(path, []byte("GIF89a"), 0600); err != nil {
		t.Fatal(err)
	}
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.EscapedPath() {
		case "/v1/chats/group%2Fone":
			fmt.Fprint(w, verifiedChat)
		case "/v1/assets/upload":
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			defer file.Close()
			got, err := io.ReadAll(file)
			if header.Filename != "demo.gif" || header.Header.Get("Content-Type") != "image/gif" || string(got) != "GIF89a" || err != nil {
				t.Errorf("upload = %v, %q, %v", header, got, err)
			}
			fmt.Fprint(w, `{"uploadID":"asset"}`)
		case "/v1/chats/group%2Fone/messages":
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			var attachment map[string]string
			if err := json.Unmarshal(body["attachment"], &attachment); err != nil || len(body) != 1 ||
				attachment["uploadID"] != "asset" || attachment["type"] != "gif" || attachment["fileName"] != "demo\".gif" ||
				attachment["mimeType"] != "image/gif" {
				t.Errorf("send body = %v, %v", body, err)
			}
			fmt.Fprint(w, `{"chatID":"group/one","pendingMessageID":"pending"}`)
		default:
			t.Errorf("unexpected endpoint %s", r.URL)
			w.WriteHeader(404)
		}
	})
	if receipt, err := client.Send(t.Context(), domain.Outgoing{Attachment: path}); err != nil || receipt.PendingID != "pending" {
		t.Fatalf("send = %+v, %v", receipt, err)
	}
}

func TestRedirectDoesNotForwardRequest(t *testing.T) {
	var redirected atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Store(true)
	}))
	defer destination.Close()
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	})
	if err := client.VerifyChat(t.Context()); err == nil || redirected.Load() {
		t.Fatalf("redirect was followed or accepted: %v", err)
	}
}

func TestRosterMismatchStopsBeforeSend(t *testing.T) {
	var writes atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"group/one","accountID":"wrong","type":"group","participants":{"hasMore":false,"items":[{"id":"blair"},{"id":"casey"}]}}`)
	})
	if _, err := client.Send(t.Context(), domain.Outgoing{Text: "reply"}); err == nil || writes.Load() != 0 {
		t.Fatalf("wrong group allowed send: %v, writes %d", err, writes.Load())
	}
}

func TestRetrieveRejectsAnotherAccount(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"reply","chatID":"group/one","accountID":"wrong"}`)
	})
	if _, err := client.Retrieve(t.Context(), "reply"); err == nil {
		t.Fatal("accepted message from another account")
	}
}

func TestUploadErrorNeverSendsMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(path, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body, path string
		status           int
		uploads          int32
	}{
		{"missing file", "{}", filepath.Join(t.TempDir(), "missing.jpg"), 200, 0},
		{"server error", `{"error":"upload rejected","uploadID":"invalid"}`, path, 200, 1},
		{"missing upload ID", `{}`, path, 200, 1},
		{"failed request", "unavailable", path, 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var uploads, sends atomic.Int32
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.EscapedPath() {
				case "/v1/chats/group%2Fone":
					fmt.Fprint(w, verifiedChat)
				case "/v1/assets/upload":
					uploads.Add(1)
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
				case "/v1/chats/group%2Fone/messages":
					sends.Add(1)
				default:
					t.Errorf("unexpected request: %s", r.URL)
				}
			})
			receipt, err := client.Send(t.Context(), domain.Outgoing{Attachment: tc.path})
			if err == nil || receipt.PendingID != "" || uploads.Load() != tc.uploads || sends.Load() != 0 {
				t.Fatalf("invalid upload: receipt %+v, error %v, uploads %d, sends %d", receipt, err, uploads.Load(), sends.Load())
			}
		})
	}
}

func TestSendRequiresScopedPendingIDInResponse(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"different chat", `{"chatID":"other","pendingMessageID":"pending"}`},
		{"missing pending ID", `{"chatID":"group/one"}`},
		{"malformed JSON", `{"chatID":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					fmt.Fprint(w, verifiedChat)
				} else {
					fmt.Fprint(w, tc.body)
				}
			})
			if receipt, err := client.Send(t.Context(), domain.Outgoing{Text: "reply"}); err == nil || receipt.PendingID != "" {
				t.Fatalf("invalid confirmation = %+v, %v", receipt, err)
			}
		})
	}
}

func TestVerifyChatAllowsTwoOrMoreDistinctMembersAndRefusesInvalidRosters(t *testing.T) {
	for _, tc := range []struct {
		name         string
		members      []string
		participants string
		wantErr      string
	}{
		{"two members without owner in participants", []string{"alex", "blair"}, `[{"id":"blair"}]`, ""},
		{"two members with owner in participants", []string{"alex", "blair"}, `[{"id":"alex"},{"id":"blair"}]`, ""},
		{"four members without owner in participants", []string{"alex", "blair", "casey", "devon"}, `[{"id":"blair"},{"id":"casey"},{"id":"devon"}]`, ""},
		{"four members with owner in participants", []string{"alex", "blair", "casey", "devon"}, `[{"id":"alex"},{"id":"blair"},{"id":"casey"},{"id":"devon"}]`, ""},
		{"zero members", nil, `[{"id":"blair"}]`, "at least two distinct eligible sender IDs"},
		{"one member", []string{"alex"}, `[{"id":"alex"}]`, "at least two distinct eligible sender IDs"},
		{"duplicate member", []string{"alex", "blair", "alex"}, `[{"id":"alex"},{"id":"blair"}]`, "at least two distinct eligible sender IDs"},
		{"missing owner in roster", []string{"blair", "casey"}, `[{"id":"blair"},{"id":"casey"}]`, "at least two distinct eligible sender IDs"},
		{"mismatched participant set", []string{"alex", "blair", "casey"}, `[{"id":"blair"},{"id":"devon"}]`, "confirmed group roster"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := testClientForGroup(t, domain.Group{ChatID: "group/one", AccountID: "demo-account", Owner: "alex", Members: tc.members}, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"id":"group/one","accountID":"demo-account","type":"group","participants":{"hasMore":false,"items":%s}}`, tc.participants)
			})
			err := client.VerifyChat(t.Context())
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want valid chat, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
