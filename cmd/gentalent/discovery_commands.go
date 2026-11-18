package main

import (
	"context"
	"fmt"

	mquery "gentalent/internal/messaging/app/query"
	mdomain "gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/infra/beeper"
	"gentalent/internal/messaging/port"
	"gentalent/internal/shared/sqlitedb"
)

const findChatNote = "Confirm one underlying Google Messages chat; do not select by title alone."

func beeperClient(cfg config, group mdomain.Group) (*beeper.Client, error) {
	path, err := cfg.tokenFile()
	if err != nil {
		return nil, err
	}
	token, err := beeper.ReadToken(path)
	if err != nil {
		return nil, err
	}
	client, err := beeper.New(cfg.BeeperURL, token, group)
	if err != nil {
		return nil, fmt.Errorf("beeper_url: %w", err)
	}
	return client, nil
}

func discovery(call invocation) (mquery.Discovery, error) {
	cfg, err := loadConfig(call.config)
	if err != nil {
		return mquery.Discovery{}, err
	}
	client, err := beeperClient(cfg, mdomain.Group{})
	if err != nil {
		return mquery.Discovery{}, err
	}
	return mquery.Discovery{DiscoveryReader: client}, nil
}

func accounts(ctx context.Context, call invocation, args []string) error {
	if _, err := parseFlags(newFlags(call, "accounts"), args); err != nil {
		return err
	}
	platform, err := discovery(call)
	if err != nil {
		return err
	}
	found, err := platform.Accounts(ctx)
	if err != nil {
		return err
	}
	shown := make([]accountJSON, len(found))
	for i, account := range found {
		shown[i] = accountJSON{
			AccountID: account.ID, Network: account.Network, Status: account.Status,
			Bridge: bridgeJSON{ID: account.Bridge.ID, Type: account.Bridge.Type, Provider: account.Bridge.Provider},
			User:   userOf(account.User),
		}
	}
	return printJSON(call.out, shown)
}

func listGroups(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "list-groups")
	account := flags.String("account", "", "only chats of this account ID")
	limit := flags.Int("limit", 50, "how many chats, 1 to 200")
	if _, err := parseFlags(flags, args); err != nil {
		return err
	}
	platform, err := discovery(call)
	if err != nil {
		return err
	}
	page, err := platform.ListGroups(ctx, *account, *limit)
	if err != nil {
		return err
	}
	return printJSON(call.out, chatPageOf(page, ""))
}

func findChat(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "find-chat")
	account := flags.String("account", "", "only chats of this account ID")
	words, err := parseFlags(flags, args, "QUERY")
	if err != nil {
		return err
	}
	platform, err := discovery(call)
	if err != nil {
		return err
	}
	page, err := platform.FindChat(ctx, words[0], *account)
	if err != nil {
		return err
	}
	return printJSON(call.out, chatPageOf(page, findChatNote))
}

func inspectChat(ctx context.Context, call invocation, args []string) error {
	id, err := parseFlags(newFlags(call, "inspect-chat"), args, "CHAT_ID")
	if err != nil {
		return err
	}
	platform, err := discovery(call)
	if err != nil {
		return err
	}
	chat, err := platform.InspectChat(ctx, id[0])
	if err != nil {
		return err
	}
	return printJSON(call.out, chatOf(chat))
}

func doctor(ctx context.Context, call invocation, args []string) error {
	flags := newFlags(call, "doctor")
	chat := flags.Bool("chat", false, "also verify the exact group, account and roster")
	contract := flags.Bool("contract", false, "also check the Beeper behavior GenTalent relies on, using recent live replies (read-only)")
	if _, err := parseFlags(flags, args); err != nil {
		return err
	}
	cfg, err := loadConfig(call.config)
	if err != nil {
		return err
	}
	group := mdomain.Group{}
	if *chat || *contract {
		if err := cfg.validateGroup(); err != nil {
			return err
		}
		group = cfg.group()
	}
	client, err := beeperClient(cfg, group)
	if err != nil {
		return err
	}
	checks := mquery.Doctor{Platform: client}
	if *contract {
		replies, closers, err := openMessaging(ctx, cfg.databases(true).messaging, sqlitedb.ReadOnly)
		if err != nil {
			return err
		}
		defer closers.close()
		checks.Replies = replies
	}
	if err := checks.Check(ctx, *chat, *contract); err != nil {
		return err
	}
	fmt.Fprintln(call.out, "Local Beeper API is reachable and token is accepted.")
	if *chat || *contract {
		fmt.Fprintln(call.out, "Exact group, account, and three-person roster match.")
	}
	if *contract {
		fmt.Fprintln(call.out, "Beeper behavior matches doc/beeper-api.md.")
	}
	return nil
}

type accountJSON struct {
	AccountID string     `json:"accountID"`
	Network   string     `json:"network"`
	Status    string     `json:"status"`
	Bridge    bridgeJSON `json:"bridge"`
	User      userJSON   `json:"user"`
}

type bridgeJSON struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Provider string `json:"provider"`
}

type userJSON struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
	Username string `json:"username"`
	IsSelf   bool   `json:"isSelf"`
}

type chatPageJSON struct {
	Items   []chatJSON `json:"items"`
	HasMore bool       `json:"has_more"`
	Note    string     `json:"note,omitempty"`
}

type chatJSON struct {
	ID           string           `json:"id"`
	AccountID    string           `json:"accountID"`
	Network      string           `json:"network"`
	Title        string           `json:"title"`
	Type         string           `json:"type"`
	Participants participantsJSON `json:"participants"`
}

type participantsJSON struct {
	Items   []participantJSON `json:"items"`
	HasMore bool              `json:"hasMore"`
	Total   int64             `json:"total"`
}

type participantJSON struct {
	userJSON
	PhoneNumber   string `json:"phoneNumber"`
	Email         string `json:"email"`
	ImgURL        string `json:"imgURL"`
	CannotMessage bool   `json:"cannotMessage"`
	IsAdmin       bool   `json:"isAdmin"`
	IsNetworkBot  bool   `json:"isNetworkBot"`
	IsPending     bool   `json:"isPending"`
}

func userOf(user port.User) userJSON {
	return userJSON{ID: user.ID, FullName: user.FullName, Username: user.Username, IsSelf: user.IsSelf}
}

func chatPageOf(page port.ChatPage, note string) chatPageJSON {
	shown := chatPageJSON{Items: make([]chatJSON, len(page.Items)), HasMore: page.HasMore, Note: note}
	for i, chat := range page.Items {
		shown.Items[i] = chatOf(chat)
	}
	return shown
}

func chatOf(chat port.Chat) chatJSON {
	participants := make([]participantJSON, len(chat.Participants.Items))
	for i, p := range chat.Participants.Items {
		participants[i] = participantJSON{
			userJSON: userOf(p.User), PhoneNumber: p.PhoneNumber, Email: p.Email, ImgURL: p.ImageURL,
			CannotMessage: p.CannotMessage, IsAdmin: p.IsAdmin, IsNetworkBot: p.IsNetworkBot, IsPending: p.IsPending,
		}
	}
	return chatJSON{
		ID: chat.ID, AccountID: chat.AccountID, Network: chat.Network, Title: chat.Title, Type: chat.Type,
		Participants: participantsJSON{Items: participants, HasMore: chat.Participants.HasMore, Total: chat.Participants.Total},
	}
}
