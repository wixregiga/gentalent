package preview

import (
	"bytes"
	"testing"

	"gentalent/internal/messaging/domain"
)

func TestPreviewPrintsAttachmentAndReturnsNoPlatformID(t *testing.T) {
	var output bytes.Buffer
	sender := Sender{Output: &output}
	result, err := sender.Send(t.Context(), domain.Outgoing{Key: "photo", Attachment: "strip.jpg"})
	if err != nil || !result.Preview || result.PendingID != "" || output.String() != "PREVIEW:  [ATTACHMENT: strip.jpg]\n" {
		t.Fatalf("receipt = %+v, output = %q, err = %v", result, output.String(), err)
	}
}
