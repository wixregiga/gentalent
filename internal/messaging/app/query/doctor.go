package query

import (
	"context"
	"fmt"
	"strings"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type Doctor struct {
	Platform port.HealthReader
	Replies  port.SentReplyReader
}

func (d Doctor) Check(ctx context.Context, chat, contract bool) error {
	if err := d.Platform.CheckConnection(ctx); err != nil {
		return err
	}
	if chat || contract {
		if err := d.Platform.VerifyChat(ctx); err != nil {
			return err
		}
	}
	if !contract {
		return nil
	}
	issues, err := d.Platform.ContractIssues(ctx)
	if err != nil {
		return err
	}
	replies, err := d.Replies.RecentSent(ctx, 3)
	if err != nil {
		return err
	}
	if len(replies) == 0 {
		issues = append(issues, "No sent replies recorded yet; message checks skipped.")
	}
	for _, reply := range replies {
		message, err := d.Platform.Retrieve(ctx, reply.FinalID)
		if err != nil {
			return err
		}
		state := message.Delivery("")
		if state != domain.Sent || message.ID != reply.FinalID {
			issues = append(issues, fmt.Sprintf("Delivered reply %s (message %s) now reads as %s.", reply.Key, reply.FinalID, state))
		}
		if !message.FromAccount {
			issues = append(issues, fmt.Sprintf("Message %s no longer reads as sent by this account.", reply.FinalID))
		}
		if reply.Attachment == "" && !domain.SameText(message.Text, reply.Text) {
			issues = append(issues, fmt.Sprintf("Message %s text differs from what was sent.", reply.FinalID))
		}
	}
	if len(issues) > 0 {
		return fmt.Errorf("platform contract check failed: %s", strings.Join(issues, " | "))
	}
	return nil
}
