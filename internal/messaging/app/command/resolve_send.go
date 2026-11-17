package command

import (
	"context"
	"fmt"
	"path/filepath"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type Resolver struct {
	Store    port.ResolutionStore
	Platform port.MessageReader
	Files    port.AttachmentReader
}

func (r Resolver) ResolveSend(ctx context.Context, key, messageID string) error {
	reply, err := r.Store.Reply(ctx, key)
	if err != nil {
		return err
	}
	if reply.State != domain.Uncertain && reply.State != domain.Failed {
		return fmt.Errorf("only an uncertain or failed reply can be resolved")
	}
	next := domain.Failed
	if messageID != "" {
		if err := r.verifyDelivery(ctx, reply, messageID); err != nil {
			return err
		}
		next = domain.Sent
	}
	changed, err := r.Store.Transition(ctx, key, reply.State, next, messageID)
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("reply %s changed while resolving or its message ID was claimed by another reply", key)
	}
	return nil
}

func (r Resolver) verifyDelivery(ctx context.Context, reply domain.Reply, id string) error {
	used, err := r.Store.FinalIDUsed(ctx, reply.Key, id)
	if err != nil {
		return err
	}
	if used {
		return fmt.Errorf("message %s is already recorded for another reply", id)
	}
	message, err := r.Platform.Retrieve(ctx, id)
	if err != nil {
		return err
	}
	if !message.FromAccount {
		return fmt.Errorf("message %s was not sent by the connected account", id)
	}
	if message.ID != id {
		return fmt.Errorf("message %s resolved to a different ID, use the final message ID", id)
	}
	if reply.Attachment != "" {
		file, err := r.Files.Attachment(ctx, reply.Attachment)
		if err != nil || !file.SizeKnown || len(message.Attachments) != 1 || file != message.Attachments[0] {
			return fmt.Errorf("message %s does not carry this reply's %s", id, filepath.Base(reply.Attachment))
		}
	} else if !domain.SameText(message.Text, reply.Text) {
		return fmt.Errorf("message %s text differs from reply %s", id, reply.Key)
	}
	return nil
}
