package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const (
	capturedAccounts = `[{"accountID":"demo-account","network":"SMS","status":"connected","bridge":{"id":"bridge","type":"sms","provider":"local"},"user":{"id":"alex","fullName":"Alex","username":"alex-demo","isSelf":true}}]`
	verifiedChat     = `{"id":"group/one","accountID":"demo-account","type":"group","participants":{"hasMore":false,"items":[{"id":"blair"},{"id":"casey"}],"total":2}}`
	capturedText     = `{"id":"739","chatID":"group/one","accountID":"demo-account","senderID":"alex","timestamp":"2026-11-16T23:49:39.327Z","sortKey":"4164","type":"TEXT","text":"<p>@gentalent-bot says: A &amp; B<br>Next</p>","isSender":true}`
	observedSpec     = `{"components":{"schemas":{"SendStatus":{"properties":{"status":{"enum":["PENDING","SUCCESS","FAIL_RETRIABLE","FAIL_PERMANENT"]}}},"Message":{"required":["id"]}}},"paths":{"/v1/chats/{chatID}/messages/{messageID}":{"get":{"description":"Retrieve by pendingMessageID"}}}}`
)

type beeperServer struct {
	t    *testing.T
	spec string
}

func (s *beeperServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.t.Errorf("the CLI wrote to Beeper: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.EscapedPath() {
	case "/v1/info":
		fmt.Fprint(w, `{"app":{"version":"test"},"server":{"status":"running"},"platform":{"os":"linux"},"endpoints":{"spec":"/v1/spec"}}`)
	case "/v1/accounts":
		fmt.Fprint(w, capturedAccounts)
	case "/v1/chats":
		if r.URL.Query().Get("limit") != "200" || r.URL.Query().Get("accountIDs") != "demo-account" {
			s.t.Errorf("B196 list-groups clamps the limit and scopes the account: %v", r.URL.Query())
		}
		fmt.Fprint(w, `{"items":[`+verifiedChat+`,{"id":"direct","type":"single","preview":{"text":"private body"}}],"hasMore":true}`)
	case "/v1/chats/search":
		fmt.Fprint(w, `{"items":[`+verifiedChat+`],"hasMore":false}`)
	case "/v1/chats/group%2Fone":
		fmt.Fprint(w, verifiedChat)
	case "/v1/chats/group%2Fone/messages":
		fmt.Fprint(w, `{"items":[],"hasMore":false,"newestCursor":"cursor-1"}`)
	case "/v1/chats/group%2Fone/messages/739":
		fmt.Fprint(w, capturedText)
	case "/v1/spec":
		fmt.Fprint(w, s.spec)
	default:
		s.t.Errorf("unexpected request %s", r.URL)
		w.WriteHeader(http.StatusNotFound)
	}
}

func capturedGroupConfig(t *testing.T, spec string) string {
	t.Helper()
	server := httptest.NewServer(&beeperServer{t: t, spec: spec})
	t.Cleanup(server.Close)
	dir := t.TempDir()
	token := filepath.Join(dir, "secret", "beeper.token")
	if err := os.MkdirAll(filepath.Dir(token), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(token, []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return writeConfig(t, dir, func(values map[string]any) {
		values["beeper_url"] = server.URL
		values["chat_id"], values["account_id"], values["owner_id"] = "group/one", "demo-account", "alex"
		values["members"] = []map[string]string{
			{"id": "alex", "name": "Alex", "platform": "android"},
			{"id": "blair", "name": "Blair", "platform": "iphone"},
			{"id": "casey", "name": "Casey", "platform": "unknown"},
		}
	})
}

func assertJSON(t *testing.T, got, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
}
