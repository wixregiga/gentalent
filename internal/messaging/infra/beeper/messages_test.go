package beeper

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

const capturedText = `"id":"739","chatID":"group/one","accountID":"demo-account","senderID":"alex","timestamp":"2026-11-16T23:49:39.327Z","sortKey":"4164","type":"TEXT","text":"<p>@gentalent-bot says: A &amp; B<br>Next</p>","isSender":true`
const capturedImage = `{"id":"747","chatID":"group/one","accountID":"demo-account","senderID":"alex","timestamp":"2026-11-16T23:55:27.129Z","sortKey":"4179","type":"IMAGE","isSender":true,"attachments":[{"type":"img","fileName":"strip.jpg","fileSize":1,"mimeType":"image/jpeg","isGif":false}],"reactions":[{"id":"blair","participantID":"blair","reactionKey":"👍","emoji":true},{"id":"casey","participantID":"casey","reactionKey":"😂","emoji":true}]}`

func TestCapturedDeliveryEvidenceThroughSDK(t *testing.T) {
	for _, tc := range []struct {
		name, id, body string
		httpStatus     int
		want           domain.State
	}{
		{"numeric ID still pending", "~bridge_test_2", "{" + capturedText + `,"sendStatus":{"reason":"Awaiting send confirmation","status":"PENDING","timestamp":"2026-11-16T23:49:39.327Z","deliveredToUsers":[]}}`, 200, domain.Sent},
		{"status disappeared", "~bridge_test_2", "{" + capturedText + "}", 200, domain.Sent},
		{"opaque pending ID", "m1694783291234567", "{" + capturedText + "}", 200, domain.Sent},
		{"captioned photo resolves to image", "m1694783291234567", capturedImage, 200, domain.Sent},
		{"expired pending ID", "pending/id", "not found", 404, domain.Uncertain},
		{"temporary failure", "pending/id", "unavailable", 503, domain.Pending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.EscapedPath() != "/v1/chats/group%2Fone/messages/"+pathID(tc.id) {
					t.Errorf("retrieval request = %s %s", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.httpStatus)
				fmt.Fprint(w, tc.body)
			})
			message, err := client.Retrieve(t.Context(), tc.id)
			if tc.httpStatus != 200 {
				if err == nil || errors.Is(err, port.ErrMessageMissing) != (tc.httpStatus == 404) {
					t.Fatalf("lookup %d: %v", tc.httpStatus, err)
				}
				return
			}
			if err != nil || message.Delivery(tc.id) != tc.want || !message.FromAccount {
				t.Fatalf("message = %+v, error = %v", message, err)
			}
			if tc.body == capturedImage {
				if message.ID != "747" || message.Text != "" || len(message.Attachments) != 1 ||
					message.Attachments[0] != (domain.Attachment{Name: "strip.jpg", Bytes: 1, SizeKnown: true}) ||
					len(message.Reactions) != 2 || message.Reactions[0].Key != "👍" {
					t.Fatalf("image lookup lost evidence: %+v", message)
				}
			} else if !domain.SameText(message.Text, "@gentalent-bot says: A & B  \nNext") {
				t.Fatalf("stored text did not match send text: %s", message.Text)
			}
		})
	}
}

func TestCursorAndLinkedMessageShape(t *testing.T) {
	const cursor = "new/+opaque=?"
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") != cursor || r.URL.Query().Get("direction") != "after" {
			t.Errorf("pagination query = %v", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"newestCursor":"next","hasMore":true,"items":[
			{"id":"member-reply","chatID":"group/one","accountID":"demo-account","senderID":"blair","timestamp":"2026-11-16T23:55:27.129Z","sortKey":"4179","text":"<p>Text</p>","linkedMessageID":"747","mentions":["alex"]},
			{"id":"gif","chatID":"group/one","accountID":"demo-account","senderID":"casey","timestamp":"2026-11-16T23:55:28Z","sortKey":"4180","type":"IMAGE","attachments":[{"fileName":"reaction.gif","mimeType":"image/gif","isGif":true,"fileSize":2900000}],"mentions":null}
		]}`)
	})
	page, err := client.Messages(t.Context(), cursor)
	if err != nil || !page.HasMore || page.NewestCursor != "next" || len(page.Messages) != 2 {
		t.Fatalf("page = %+v, error = %v", page, err)
	}
	first, second := page.Messages[0], page.Messages[1]
	if first.LinkedID != "747" || first.FromAccount || first.Status != "" || len(first.Mentions) != 1 || first.Mentions[0] != "alex" {
		t.Fatalf("reply = %+v", first)
	}
	if len(second.Attachments) != 1 || second.Attachments[0].Name != "reaction.gif" || len(second.Mentions) != 0 {
		t.Fatalf("GIF message = %+v", second)
	}
}

func TestLookupHonorsDocumentedDeliveryStatuses(t *testing.T) {
	for status, want := range map[string]domain.State{
		"SUCCESS":        domain.Sent,
		"FAIL_RETRIABLE": domain.Failed,
		"FAIL_PERMANENT": domain.Failed,
		"NEW_STATUS":     domain.Sent,
	} {
		t.Run(status, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{%s,"sendStatus":{"status":%q,"timestamp":"2026-11-16T23:49:39.327Z"}}`, capturedText, status)
			})
			message, err := client.Retrieve(t.Context(), "pending")
			if err != nil || message.Delivery("pending") != want {
				t.Fatalf("lookup = %+v, %v, want %s", message, err, want)
			}
		})
	}
}
