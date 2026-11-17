package beeper

import (
	"context"
	"fmt"
	"mime"
	"net"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/beeper/desktop-api-go/v6"
	"github.com/beeper/desktop-api-go/v6/option"

	"gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

func (c *Client) Send(ctx context.Context, reply domain.Outgoing) (port.Receipt, error) {
	if reply.Text == "" && reply.Attachment == "" {
		return port.Receipt{}, fmt.Errorf("a message needs text, an attachment, or both")
	}
	if err := c.VerifyChat(ctx); err != nil {
		return port.Receipt{}, err
	}
	body := sdk.MessageSendParams{}
	if reply.Text != "" {
		body.Text = sdk.String(reply.Text)
	}
	if reply.Attachment != "" {
		attachment, err := c.upload(ctx, reply.Attachment)
		if err != nil {
			return port.Receipt{}, err
		}
		body.Attachment = attachment
	}
	result, err := c.api.Messages.Send(ctx, pathID(c.group.ChatID), body)
	if err != nil {
		return port.Receipt{}, apiError(err)
	}
	if result == nil || result.ChatID != c.group.ChatID || result.PendingMessageID == "" {
		return port.Receipt{}, fmt.Errorf("send response did not confirm the configured underlying chat and pending ID")
	}
	return port.Receipt{PendingID: result.PendingMessageID}, nil
}

func (c *Client) upload(ctx context.Context, path string) (sdk.MessageSendParamsAttachment, error) {
	var attachment sdk.MessageSendParamsAttachment
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return attachment, fmt.Errorf("attachment file does not exist: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return attachment, fmt.Errorf("open attachment: %w", err)
	}
	defer file.Close()
	name := filepath.Base(path)
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if contentType == "" {
		contentType = "image/jpeg"
	}
	safeName := strings.NewReplacer("\"", "", "\r", "", "\n", "").Replace(name)
	params := sdk.AssetUploadParams{File: sdk.File(file, safeName, contentType)}
	result, err := c.api.Assets.Upload(ctx, params,
		option.WithHTTPClient(httpClient(&net.Dialer{Timeout: uploadTimeout}, uploadTimeout)),
	)
	if err != nil {
		if response, ok := err.(responseError); ok {
			return attachment, fmt.Errorf("asset upload returned HTTP %d; no automatic write retry was made", response.status)
		}
		return attachment, fmt.Errorf("asset upload connection or JSON response failed")
	}
	if result == nil || result.UploadID == "" {
		if result != nil && result.Error != "" {
			return attachment, fmt.Errorf("asset upload failed: %s", result.Error)
		}
		return attachment, fmt.Errorf("asset upload failed: missing uploadID")
	}
	if result.Error != "" {
		return attachment, fmt.Errorf("asset upload failed: %s", result.Error)
	}
	kind := "image"
	if contentType == "image/gif" {
		kind = "gif"
	}
	return sdk.MessageSendParamsAttachment{
		UploadID: result.UploadID, Type: kind, FileName: sdk.String(name), MimeType: sdk.String(contentType),
	}, nil
}
