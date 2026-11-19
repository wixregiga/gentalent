package domain

import "testing"

func TestDeliveryEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, id string
		status   SendStatus
		want     State
	}{
		{"success", "pending", Confirmed, Sent},
		{"retryable failure", "final", RetryableFailure, Failed},
		{"permanent failure", "final", PermanentFailure, Failed},
		{"assigned but unconfirmed", "final", AwaitingConfirmation, Sent},
		{"bridge omitted status", "final", "", Sent},
		{"pending id without a tilde", "pending", "", Pending},
		{"missing id", "", "", Pending},
		{"unknown status", "final", "unknown", Sent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Message{ID: tc.id, Status: tc.status}).Delivery("pending"); got != tc.want {
				t.Fatalf("delivery = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestTerminalReplyNeverReturnsToSending(t *testing.T) {
	for _, state := range []State{Sent, Failed, Uncertain, Previewed} {
		if state.Allows(Sending) || state.Allows(Queued) {
			t.Fatalf("%s can be sent again", state)
		}
	}
}
