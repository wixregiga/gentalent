package beeper

import (
	"context"
	"errors"
	"fmt"
	"math"

	sdk "github.com/beeper/desktop-api-go/v6"
	"github.com/beeper/desktop-api-go/v6/packages/respjson"
	"github.com/beeper/desktop-api-go/v6/shared"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

func (c *Client) Messages(ctx context.Context, cursor string) (port.Page, error) {
	params := sdk.MessageListParams{}
	if cursor != "" {
		params.Cursor = sdk.String(cursor)
		params.Direction = sdk.MessageListParamsDirectionAfter
	}
	result, err := c.api.Messages.List(ctx, pathID(c.group.ChatID), params)
	if err != nil || result == nil || !result.JSON.Items.Valid() || !result.JSON.HasMore.Valid() {
		return port.Page{}, apiError(err)
	}
	page := port.Page{NewestCursor: result.NewestCursor, HasMore: result.HasMore}
	for _, message := range result.Items {
		page.Messages = append(page.Messages, fromMessage(message))
	}
	return page, nil
}

func (c *Client) Retrieve(ctx context.Context, id string) (domain.Message, error) {
	message, err := c.api.Messages.Get(ctx, pathID(id), sdk.MessageGetParams{ChatID: pathID(c.group.ChatID)})
	var response responseError
	switch {
	case errors.As(err, &response) && response.status == 404:
		return domain.Message{}, fmt.Errorf("%w: %s", port.ErrMessageMissing, err)
	case err != nil || message == nil:
		return domain.Message{}, apiError(err)
	case message.ChatID != c.group.ChatID || message.AccountID != c.group.AccountID:
		return domain.Message{}, fmt.Errorf("retrieved message has the wrong chat/account")
	case message.ID == "":
		return domain.Message{}, fmt.Errorf("retrieved message has no ID")
	}
	return fromMessage(*message), nil
}

func fromMessage(m shared.Message) domain.Message {
	message := domain.Message{
		ID: m.ID, ChatID: m.ChatID, AccountID: m.AccountID, Sender: m.SenderID,
		Text: m.Text, SentAt: m.Timestamp, SortKey: m.SortKey, LinkedID: m.LinkedMessageID,
		Edited: edited(m), Deleted: m.IsDeleted, Hidden: m.IsHidden,
		FromAccount: m.IsSender, Status: sendStatus(m), Mentions: m.Mentions,
	}
	for _, a := range m.Attachments {
		known := a.JSON.FileSize.Valid() && a.FileSize >= 0 && a.FileSize < math.MaxInt64 && math.Trunc(a.FileSize) == a.FileSize
		attachment := domain.Attachment{Name: a.FileName, SizeKnown: known}
		if known {
			attachment.Bytes = int64(a.FileSize)
		}
		message.Attachments = append(message.Attachments, attachment)
	}
	for _, reaction := range m.Reactions {
		message.Reactions = append(message.Reactions, domain.Reaction{Sender: reaction.ParticipantID, Key: reaction.ReactionKey})
	}
	return message
}

func sendStatus(m shared.Message) domain.SendStatus {
	if m.JSON.SendStatus.Raw() == respjson.Omitted {
		return ""
	}
	switch m.SendStatus.Status {
	case shared.SendStatusStatusSuccess:
		return domain.Confirmed
	case shared.SendStatusStatusFailRetriable:
		return domain.RetryableFailure
	case shared.SendStatusStatusFailPermanent:
		return domain.PermanentFailure
	default:
		return domain.AwaitingConfirmation
	}
}

func edited(message shared.Message) bool {
	raw := message.JSON.EditedTimestamp.Raw()
	return raw != respjson.Omitted && raw != respjson.Null && raw != `""`
}
