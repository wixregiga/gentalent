package beeper

import (
	"context"
	"fmt"
	"maps"

	sdk "github.com/beeper/desktop-api-go/v6"
)

func (c *Client) VerifyChat(ctx context.Context) error {
	group := c.group
	chat, err := c.chat(ctx, group.ChatID)
	if err != nil {
		return err
	}
	if chat.ID != group.ChatID || chat.AccountID != group.AccountID || chat.Type != sdk.ChatTypeGroup {
		return fmt.Errorf("chat/account/type differs from the configured group; stopped")
	}
	if chat.Participants.HasMore || !chat.Participants.JSON.Items.Valid() || !chat.Participants.JSON.HasMore.Valid() {
		return fmt.Errorf("the complete group roster could not be verified")
	}
	expected := make(map[string]bool, len(group.Members))
	for _, member := range group.Members {
		expected[member] = true
	}
	if len(group.Members) < 2 || len(expected) != len(group.Members) || expected[""] || !expected[group.Owner] {
		return fmt.Errorf("at least two distinct eligible sender IDs, including the owner, are required")
	}
	found := make(map[string]bool, len(chat.Participants.Items))
	for _, participant := range chat.Participants.Items {
		found[participant.ID] = true
	}
	withOwner := maps.Equal(found, expected)
	delete(expected, group.Owner)
	if !withOwner && !maps.Equal(found, expected) {
		return fmt.Errorf("the current group roster differs from the confirmed group roster")
	}
	return nil
}

func (c *Client) chat(ctx context.Context, id string) (*sdk.Chat, error) {
	params := sdk.ChatGetParams{MaxParticipantCount: sdk.Int(-1)}
	chat, err := c.api.Chats.Get(ctx, pathID(id), params)
	if err != nil || chat == nil {
		return nil, apiError(err)
	}
	return chat, nil
}

func accountFilter(id string) []string {
	if id == "" {
		return nil
	}
	return []string{id}
}
