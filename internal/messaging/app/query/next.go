package query

import (
	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type Incoming = domain.Incoming
type Linked = domain.Linked

type Inbox struct {
	port.InboxReader
}
