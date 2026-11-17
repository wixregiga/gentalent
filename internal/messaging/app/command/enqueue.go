package command

import (
	"context"
	"fmt"
	"slices"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type Outgoing = domain.Outgoing

type Enqueuer struct {
	Store port.ReplyWriter
}

func (e Enqueuer) Enqueue(ctx context.Context, outgoing []Outgoing) error {
	replies := slices.Clone(outgoing)
	for i, reply := range replies {
		if reply.Key == "" || (reply.Text == "" && reply.Attachment == "") {
			return fmt.Errorf("a reply needs a key and text or an attachment")
		}
		if reply.Text != "" {
			replies[i].Text = domain.ReplyPrefix + " " + reply.Text
		}
	}
	return e.Store.Enqueue(ctx, replies)
}
