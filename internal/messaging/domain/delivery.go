package domain

type State string

const (
	Queued    State = "queued"
	Sending   State = "sending"
	Pending   State = "pending"
	Sent      State = "sent"
	Uncertain State = "uncertain"
	Failed    State = "failed"
	Previewed State = "preview"
)

type SendStatus string

const (
	Confirmed            SendStatus = "confirmed"
	AwaitingConfirmation SendStatus = "awaiting"
	RetryableFailure     SendStatus = "retryable_failure"
	PermanentFailure     SendStatus = "permanent_failure"
)

type Outgoing struct {
	Key        string
	Text       string
	Attachment string
	Tag        string
}

type Reply struct {
	Outgoing
	State                    State
	PendingID, FinalID, Kind string
}

func (m Message) Delivery(pendingID string) State {
	switch m.Status {
	case RetryableFailure, PermanentFailure:
		return Failed
	case Confirmed:
		return Sent
	}
	if m.ID != "" && m.ID != pendingID {
		return Sent
	}
	return Pending
}

func (s State) Allows(next State) bool {
	switch s {
	case Queued:
		return next == Sending
	case Sending:
		return next == Pending || next == Uncertain || next == Previewed
	case Pending:
		return next == Sent || next == Failed || next == Uncertain
	case Uncertain, Failed:
		return next == Sent || next == Failed
	}
	return false
}
