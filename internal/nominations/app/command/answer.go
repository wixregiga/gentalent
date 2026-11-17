package command

import (
	"context"

	"gentalent/internal/nominations/port"
)

type AnswerHandler struct {
	Outbox port.Outbox
}

func (h AnswerHandler) Handle(ctx context.Context, messageID string, replies []port.Reply) error {
	return h.Outbox.Answer(ctx, messageID, replies)
}
