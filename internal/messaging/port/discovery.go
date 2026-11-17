package port

import "context"

type DiscoveryReader interface {
	Accounts(context.Context) ([]Account, error)
	ListGroups(context.Context, string, int) (ChatPage, error)
	FindChat(context.Context, string, string) (ChatPage, error)
	InspectChat(context.Context, string) (Chat, error)
}

type HealthReader interface {
	GroupReader
	MessageReader
	CheckConnection(context.Context) error
	ContractIssues(context.Context) ([]string, error)
}

type Account struct {
	ID      string
	Network string
	Status  string
	Bridge  Bridge
	User    User
}

type Bridge struct {
	ID       string
	Type     string
	Provider string
}

type User struct {
	ID       string
	FullName string
	Username string
	IsSelf   bool
}

type Chat struct {
	ID           string
	AccountID    string
	Network      string
	Title        string
	Type         string
	Participants Participants
}

type Participants struct {
	Items   []Participant
	HasMore bool
	Total   int64
}

type Participant struct {
	User
	PhoneNumber   string
	Email         string
	ImageURL      string
	CannotMessage bool
	IsAdmin       bool
	IsNetworkBot  bool
	IsPending     bool
}

type ChatPage struct {
	Items   []Chat
	HasMore bool
}
