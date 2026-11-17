package beeper

import (
	"context"

	sdk "github.com/beeper/desktop-api-go/v6"
	"github.com/beeper/desktop-api-go/v6/shared"

	"gentalent/internal/messaging/port"
)

func (c *Client) Accounts(ctx context.Context) ([]port.Account, error) {
	accounts, err := c.api.Accounts.List(ctx)
	if err != nil || accounts == nil {
		return nil, apiError(err)
	}
	out := make([]port.Account, 0, len(*accounts))
	for _, account := range *accounts {
		out = append(out, port.Account{
			ID: account.AccountID, Network: account.Network, Status: string(account.Status), User: fromUser(account.User),
			Bridge: port.Bridge{ID: account.Bridge.ID, Type: account.Bridge.Type, Provider: string(account.Bridge.Provider)},
		})
	}
	return out, nil
}

func (c *Client) ListGroups(ctx context.Context, account string, limit int) (port.ChatPage, error) {
	params := sdk.ChatListParams{AccountIDs: accountFilter(account), Limit: sdk.Int(int64(limit))}
	result, err := c.api.Chats.List(ctx, params)
	if err != nil || result == nil || !result.JSON.Items.Valid() || !result.JSON.HasMore.Valid() {
		return port.ChatPage{}, apiError(err)
	}
	out := port.ChatPage{Items: []port.Chat{}, HasMore: result.HasMore}
	for _, item := range result.Items {
		if item.Type == sdk.ChatTypeGroup {
			out.Items = append(out.Items, fromChat(item.Chat))
		}
	}
	return out, nil
}

func (c *Client) FindChat(ctx context.Context, words, account string) (port.ChatPage, error) {
	params := sdk.ChatSearchParams{
		AccountIDs: accountFilter(account), Query: sdk.String(words), Limit: sdk.Int(20), Type: sdk.ChatSearchParamsTypeGroup,
	}
	result, err := c.api.Chats.Search(ctx, params)
	if err != nil || result == nil || !result.JSON.Items.Valid() || !result.JSON.HasMore.Valid() {
		return port.ChatPage{}, apiError(err)
	}
	out := port.ChatPage{Items: []port.Chat{}, HasMore: result.HasMore}
	for _, item := range result.Items {
		if item.Type == sdk.ChatTypeGroup {
			out.Items = append(out.Items, fromChat(item))
		}
	}
	return out, nil
}

func (c *Client) InspectChat(ctx context.Context, id string) (port.Chat, error) {
	result, err := c.chat(ctx, id)
	if err != nil {
		return port.Chat{}, err
	}
	return fromChat(*result), nil
}

func fromUser(u shared.User) port.User {
	return port.User{ID: u.ID, FullName: u.FullName, Username: u.Username, IsSelf: u.IsSelf}
}

func fromChat(c sdk.Chat) port.Chat {
	participants := port.Participants{Items: []port.Participant{}, HasMore: c.Participants.HasMore, Total: c.Participants.Total}
	for _, item := range c.Participants.Items {
		participants.Items = append(participants.Items, port.Participant{
			User: fromUser(item.User), PhoneNumber: item.PhoneNumber, Email: item.Email, ImageURL: item.ImgURL,
			CannotMessage: item.CannotMessage, IsAdmin: item.IsAdmin, IsNetworkBot: item.IsNetworkBot, IsPending: item.IsPending,
		})
	}
	return port.Chat{ID: c.ID, AccountID: c.AccountID, Title: c.Title, Network: c.Network, Type: string(c.Type), Participants: participants}
}
