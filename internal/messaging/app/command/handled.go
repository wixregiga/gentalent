package command

import "gentalent/internal/messaging/port"

type Acknowledger struct {
	port.InboxWriter
}
