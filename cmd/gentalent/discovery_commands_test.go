package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	mdomain "gentalent/internal/messaging/domain"
)

func participantJSONOf(id string) string {
	return fmt.Sprintf(`{"id":%q,"fullName":"","username":"","isSelf":false,"phoneNumber":"","email":"","imgURL":"","cannotMessage":false,"isAdmin":false,"isNetworkBot":false,"isPending":false}`, id)
}

var shownGroup = `{"id":"group/one","accountID":"demo-account","network":"","title":"","type":"group","participants":{"items":[` +
	participantJSONOf("blair") + `,` + participantJSONOf("casey") + `],"hasMore":false,"total":2}}`

func TestDiscoveryPrintsTheChatMetadata(t *testing.T) {
	config := capturedGroupConfig(t, observedSpec)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"B195 accounts", []string{"accounts"}, capturedAccounts},
		{"B196 list-groups keeps only groups", []string{"list-groups", "--limit", "500", "--account", "demo-account"}, `{"items":[` + shownGroup + `],"has_more":true}`},
		{"B197 find-chat adds the note", []string{"find-chat", "exact words", "--account", "demo-account"}, `{"items":[` + shownGroup + `],"has_more":false,"note":"` + findChatNote + `"}`},
		{"B198 inspect-chat", []string{"inspect-chat", "group/one"}, shownGroup},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errs := gentalent(t, append([]string{"--config", config}, tc.args...)...)
			if code != 0 {
				t.Fatalf("exit %d: %s", code, errs)
			}
			assertJSON(t, out, tc.want)
		})
	}
}

func TestDoctorReportsEachCheckItPassed(t *testing.T) {
	config := capturedGroupConfig(t, observedSpec)
	reachable := "Local Beeper API is reachable and token is accepted.\n"
	roster := "Exact group, account, and three-person roster match.\n"
	if code, out, errs := gentalent(t, "--config", config, "doctor"); code != 0 || out != reachable {
		t.Fatalf("B193 doctor: %d %q %q", code, out, errs)
	}
	if code, out, errs := gentalent(t, "--config", config, "doctor", "--chat"); code != 0 || out != reachable+roster {
		t.Fatalf("B193 doctor --chat: %d %q %q", code, out, errs)
	}
	if code, _, errs := gentalent(t, "--config", config, "doctor", "--contract"); code != 1 || !strings.Contains(errs, "no database at") {
		t.Fatalf("B194 doctor --contract needs the live database: %d %q", code, errs)
	}

	recordSentReply(t, config)
	code, out, errs := gentalent(t, "--config", config, "doctor", "--contract")
	if code != 0 || out != reachable+roster+"Beeper behavior matches doc/beeper-api.md.\n" {
		t.Fatalf("B194 doctor --contract: %d %q %q", code, out, errs)
	}
}

func TestDoctorContractNamesEachDrift(t *testing.T) {
	config := capturedGroupConfig(t, `{"components":{"schemas":{"SendStatus":{"properties":{"status":{"enum":["SUCCESS"]}}},"Message":{"required":["sendStatus"]}}},"paths":{}}`)
	recordSentReply(t, config)
	code, out, errs := gentalent(t, "--config", config, "doctor", "--contract")
	for _, drift := range []string{"SendStatus values changed", "Message.sendStatus became required", "no longer documents lookup by pendingMessageID"} {
		if code != 1 || out != "" || !strings.Contains(errs, drift) {
			t.Fatalf("B159 want %q: %d %q %q", drift, code, out, errs)
		}
	}
}

func recordSentReply(t *testing.T, config string) {
	t.Helper()
	cfg, err := loadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	databases, err := openWorker(t.Context(), cfg, true, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer databases.close()
	ctx := t.Context()
	if err := databases.inbox.Enqueue(ctx, []mdomain.Outgoing{{Key: "reply-1", Text: "@gentalent-bot says: A & B  \nNext"}}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		from, to mdomain.State
		id       string
	}{{mdomain.Queued, mdomain.Sending, ""}, {mdomain.Sending, mdomain.Pending, "pending-1"}, {mdomain.Pending, mdomain.Sent, "739"}} {
		if recorded, err := databases.inbox.Transition(ctx, "reply-1", step.from, step.to, step.id); err != nil || recorded != step.to {
			t.Fatalf("record %s: %q %v", step.to, recorded, err)
		}
	}
}
