package preview

import (
	"context"
	"fmt"
	"io"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type Sender struct {
	Output io.Writer
}

func (s Sender) Send(ctx context.Context, reply domain.Outgoing) (port.Receipt, error) {
	if err := ctx.Err(); err != nil {
		return port.Receipt{}, err
	}
	suffix := ""
	if reply.Attachment != "" {
		suffix = " [ATTACHMENT: " + reply.Attachment + "]"
	}
	if _, err := fmt.Fprintln(s.Output, "PREVIEW:", reply.Text+suffix); err != nil {
		return port.Receipt{}, fmt.Errorf("write preview: %w", err)
	}
	return port.Receipt{Preview: true}, nil
}
