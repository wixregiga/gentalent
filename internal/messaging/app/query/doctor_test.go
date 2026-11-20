package query

import (
	"context"
	"strings"
	"testing"

	"gentalent/internal/messaging/domain"
)

type healthPlatform struct {
	issues   []string
	messages map[string]domain.Message
}

func (p healthPlatform) CheckConnection(context.Context) error            { return nil }
func (p healthPlatform) VerifyChat(context.Context) error                 { return nil }
func (p healthPlatform) ContractIssues(context.Context) ([]string, error) { return p.issues, nil }
func (p healthPlatform) Retrieve(_ context.Context, id string) (domain.Message, error) {
	return p.messages[id], nil
}

type sentReplies []domain.Reply

func (s sentReplies) RecentSent(_ context.Context, n int) ([]domain.Reply, error) {
	return s[:min(len(s), n)], nil
}

func TestDoctorChecksObservedEvidenceForRecentSentReplies(t *testing.T) {
	reader := sentReplies{
		{Outgoing: domain.Outgoing{Key: "a", Text: "plain *text*"}, FinalID: "first"},
		{Outgoing: domain.Outgoing{Key: "b", Text: "caption", Attachment: "image.jpg"}, FinalID: "image"},
	}
	platform := healthPlatform{messages: map[string]domain.Message{
		"first": {ID: "first", FromAccount: true, Text: "<p>plain <em>text</em></p>"},
		"image": {ID: "image", FromAccount: true},
	}}
	d := Doctor{Platform: platform, Replies: reader}
	if err := d.Check(t.Context(), false, true); err != nil {
		t.Fatal(err)
	}
	platform.messages["first"] = domain.Message{ID: "other", FromAccount: false, Text: "wrong", Status: domain.RetryableFailure}
	err := d.Check(t.Context(), false, true)
	for _, text := range []string{"now reads as failed", "no longer reads as sent by this account", "text differs"} {
		if err == nil || !strings.Contains(err.Error(), text) {
			t.Fatalf("expected diagnostic %q in %v", text, err)
		}
	}
	d.Replies = sentReplies{}
	if err := d.Check(t.Context(), false, true); err == nil || !strings.Contains(err.Error(), "No sent replies") {
		t.Fatalf("empty database passed the contract check: %v", err)
	}
}

func TestDoctorVerifyContractAndCheckSentReplies(t *testing.T) {
	clean := Doctor{Platform: healthPlatform{}, Replies: sentReplies{}}
	if err := clean.VerifyContract(t.Context()); err != nil {
		t.Fatalf("clean contract failed: %v", err)
	}
	if checked, err := clean.CheckSentReplies(t.Context()); err != nil || checked != 0 {
		t.Fatalf("zero sent replies: checked=%d err=%v", checked, err)
	}

	driftedContract := Doctor{Platform: healthPlatform{issues: []string{"SendStatus values changed"}}}
	if err := driftedContract.VerifyContract(t.Context()); err == nil || !strings.Contains(err.Error(), "SendStatus values changed") {
		t.Fatalf("expected contract drift error, got %v", err)
	}

	reader := sentReplies{{Outgoing: domain.Outgoing{Key: "a", Text: "plain"}, FinalID: "first"}}
	platform := healthPlatform{messages: map[string]domain.Message{"first": {ID: "first", FromAccount: true, Text: "<p>plain</p>"}}}
	ok := Doctor{Platform: platform, Replies: reader}
	if checked, err := ok.CheckSentReplies(t.Context()); err != nil || checked != 1 {
		t.Fatalf("valid sent reply: checked=%d err=%v", checked, err)
	}

	platform.messages["first"] = domain.Message{ID: "first", FromAccount: false, Text: "wrong"}
	if checked, err := ok.CheckSentReplies(t.Context()); err == nil || checked != 1 || !strings.Contains(err.Error(), "text differs") {
		t.Fatalf("drifted sent reply: checked=%d err=%v", checked, err)
	}
}
