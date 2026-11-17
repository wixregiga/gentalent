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
	ID      string `json:"accountID"`
	Network string `json:"network"`
	Status  string `json:"status"`
	Bridge  Bridge `json:"bridge"`
	User    User   `json:"user"`
}

type Bridge struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Provider string `json:"provider"`
}

type User struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
	Username string `json:"username"`
	IsSelf   bool   `json:"isSelf"`
}

type Chat struct {
	ID           string       `json:"id"`
	AccountID    string       `json:"accountID"`
	Network      string       `json:"network"`
	Title        string       `json:"title"`
	Type         string       `json:"type"`
	Participants Participants `json:"participants"`
}

type Participants struct {
	Items   []Participant `json:"items"`
	HasMore bool          `json:"hasMore"`
	Total   int64         `json:"total"`
}

type Participant struct {
	User
	PhoneNumber   string `json:"phoneNumber,omitempty"`
	Email         string `json:"email,omitempty"`
	ImageURL      string `json:"imgURL,omitempty"`
	CannotMessage bool   `json:"cannotMessage,omitempty"`
	IsAdmin       bool   `json:"isAdmin,omitempty"`
	IsNetworkBot  bool   `json:"isNetworkBot,omitempty"`
	IsPending     bool   `json:"isPending,omitempty"`
}

type ChatPage struct {
	Items   []Chat `json:"items"`
	HasMore bool   `json:"has_more"`
}
