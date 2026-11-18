package command

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type Flusher struct {
	Store  port.DeliveryStore
	Sender port.Sender
	Reader port.MessageReader
	Log    *slog.Logger
}

func (f Flusher) Flush(ctx context.Context) error {
	replies, err := f.Store.ActiveReplies(ctx)
	if err != nil {
		return err
	}
	for _, reply := range replies {
		if err := f.deliver(ctx, reply); err != nil {
			return err
		}
	}
	return nil
}

func (f Flusher) deliver(ctx context.Context, reply domain.Reply) error {
	if reply.State == domain.Queued {
		return f.send(ctx, reply)
	}
	if f.Reader == nil {
		return nil
	}
	message, err := f.Reader.Retrieve(ctx, reply.PendingID)
	switch {
	case errors.Is(err, port.ErrMessageMissing):
		if f.Log != nil {
			f.Log.WarnContext(ctx, "pending reply no longer resolves; check the thread, then use resolve-send", "key", reply.Key, "error", err)
		}
		return f.record(ctx, reply, domain.Uncertain, "")
	case err != nil:
		return err
	}
	if state := message.Delivery(reply.PendingID); state != domain.Pending {
		return f.record(ctx, reply, state, message.ID)
	}
	return nil
}

func (f Flusher) send(ctx context.Context, reply domain.Reply) error {
	claimed, err := f.Store.Transition(ctx, reply.Key, domain.Queued, domain.Sending, "")
	if err != nil || claimed != domain.Sending {
		return err
	}
	reply.State = domain.Sending
	receipt, err := f.Sender.Send(ctx, reply.Outgoing)
	record := context.WithoutCancel(ctx)
	switch {
	case err != nil || (!receipt.Preview && receipt.PendingID == ""):
		return errors.Join(
			fmt.Errorf("send %s is uncertain. Check the thread, then use resolve-send", reply.Key),
			err, f.record(record, reply, domain.Uncertain, ""),
		)
	case receipt.Preview:
		return f.record(record, reply, domain.Previewed, "")
	default:
		return f.record(record, reply, domain.Pending, receipt.PendingID)
	}
}

func (f Flusher) record(ctx context.Context, reply domain.Reply, state domain.State, id string) error {
	recorded, err := f.Store.Transition(ctx, reply.Key, reply.State, state, id)
	if err != nil {
		return fmt.Errorf("record %s for reply %s: %w", state, reply.Key, err)
	}
	switch {
	case recorded == "" && reply.State == domain.Sending:
		return fmt.Errorf("reply %s changed while recording %s", reply.Key, state)
	case recorded == "" || f.Log == nil:
		return nil
	case recorded != state:
		f.Log.WarnContext(ctx, "message is already recorded for another reply; check the thread, then use resolve-send", "key", reply.Key, "message", id)
	}
	f.Log.InfoContext(ctx, "reply delivery", "key", reply.Key, "status", recorded, "message", id)
	return nil
}
