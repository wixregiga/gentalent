package beeper

import (
	"fmt"
	"net/http"
	"testing"
)

func TestDiscoveryReturnsMetadataAndUsesRequestedScope(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			t.Errorf("discovery used %s", r.Method)
		}
		switch r.URL.EscapedPath() {
		case "/v1/accounts":
			fmt.Fprint(w, `[{"accountID":"demo-account","network":"SMS","status":"connected","bridge":{"id":"bridge","type":"sms","provider":"local"},"user":{"id":"alex","fullName":"Alex","username":"alex-demo","isSelf":true}}]`)
		case "/v1/chats":
			if r.URL.Query().Get("limit") != "200" || r.URL.Query().Get("accountIDs") != "demo-account" {
				t.Errorf("list query = %v", r.URL.Query())
			}
			fmt.Fprint(w, `{"items":[`+verifiedChat+`,{"id":"direct","type":"single","preview":{"text":"private body"}}],"hasMore":true}`)
		case "/v1/chats/search":
			if r.URL.Query().Get("limit") != "20" || r.URL.Query().Get("type") != "group" || r.URL.Query().Get("query") != "exact & words" || r.URL.Query().Get("accountIDs") != "demo-account" {
				t.Errorf("search query = %v", r.URL.Query())
			}
			fmt.Fprint(w, `{"items":[`+verifiedChat+`],"hasMore":false}`)
		case "/v1/chats/group%2Fone":
			if r.URL.Query().Get("maxParticipantCount") != "-1" {
				t.Errorf("inspect query = %v", r.URL.Query())
			}
			fmt.Fprint(w, verifiedChat)
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(404)
		}
	})
	ctx := t.Context()
	accounts, err := client.Accounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].Status != "connected" || accounts[0].User.FullName != "Alex" ||
		accounts[0].Bridge.Provider != "local" {
		t.Fatalf("accounts = %+v, %v", accounts, err)
	}
	groups, err := client.ListGroups(ctx, "demo-account", 200)
	if err != nil || len(groups.Items) != 1 || !groups.HasMore || groups.Items[0].ID != "group/one" {
		t.Fatalf("groups = %+v, %v", groups, err)
	}
	groups, err = client.FindChat(ctx, "exact & words", "demo-account")
	if err != nil || len(groups.Items) != 1 || groups.HasMore {
		t.Fatalf("matching groups = %+v, %v", groups, err)
	}
	chat, err := client.InspectChat(ctx, "group/one")
	if err != nil || len(chat.Participants.Items) != 2 || chat.Participants.Items[0].ID != "blair" {
		t.Fatalf("chat = %+v, %v", chat, err)
	}
}

func TestContractCheckFlagsEachSpecDrift(t *testing.T) {
	wantSpec := `{"components":{"schemas":{"SendStatus":{"properties":{"status":{"enum":["PENDING","SUCCESS","FAIL_RETRIABLE","FAIL_PERMANENT"]}}},"Message":{"required":["id"]}, "WSReadyMessage":{"properties":{"version":{"enum":[1]}}}}},"paths":{"/v1/chats/{chatID}/messages/{messageID}":{"get":{"description":"Retrieve by pendingMessageID"}}}}`
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"observed spec", wantSpec, 0},
		{"all assumptions drifted", `{"components":{"schemas":{"SendStatus":{"properties":{"status":{"enum":["SUCCESS"]}}},"Message":{"required":["sendStatus"]}}},"paths":{}}`, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/spec" || r.Method != http.MethodGet {
					t.Errorf("spec request = %s %s", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.body)
			})
			issues, err := client.ContractIssues(t.Context())
			if err != nil || len(issues) != tc.want {
				t.Fatalf("contract issues = %v, %v", issues, err)
			}
		})
	}
}

func TestDoctorRequiresAuthenticatedAccountsEndpoint(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/info":
			fmt.Fprint(w, `{"app":{"version":"test"},"server":{"status":"running"},"platform":{"os":"linux"},"endpoints":{"spec":"/v1/spec"}}`)
		case "/v1/accounts":
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"unauthorized"}`)
		default:
			t.Errorf("unexpected check request %s", r.URL)
		}
	})
	if err := client.CheckConnection(t.Context()); err == nil {
		t.Fatal("doctor passed without a valid token")
	}
}
