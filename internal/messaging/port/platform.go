package port

import (
	"context"
	"errors"

	"gentalent/internal/messaging/domain"
)

var ErrMessageMissing = errors.New("platform message not found")

type MessageReader interface {
	Retrieve(context.Context, string) (domain.Message, error)
}

type GroupReader interface {
	VerifyChat(context.Context) error
}

type PollReader interface {
	GroupReader
	MessageReader
	Messages(context.Context, string) (Page, error)
}

type Page struct {
	Messages     []domain.Message
	NewestCursor string
	HasMore      bool
}

type Sender interface {
	Send(context.Context, domain.Outgoing) (Receipt, error)
}

type Receipt struct {
	PendingID string
	Preview   bool
}

type AttachmentReader interface {
	Attachment(context.Context, string) (domain.Attachment, error)
}
