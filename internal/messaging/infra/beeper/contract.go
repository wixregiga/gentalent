package beeper

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/beeper/desktop-api-go/v6/shared"
)

func (c *Client) CheckConnection(ctx context.Context) error {
	info, err := c.api.Info.Get(ctx)
	if err != nil || info == nil {
		return apiError(err)
	}
	accounts, err := c.api.Accounts.List(ctx)
	if err != nil || accounts == nil {
		return apiError(err)
	}
	return nil
}

type specDocument struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
	Components struct {
		Schemas struct {
			SendStatus struct {
				Properties struct {
					Status struct {
						Enum []string `json:"enum"`
					} `json:"status"`
				} `json:"properties"`
			} `json:"SendStatus"`
			Message struct {
				Required []string `json:"required"`
			} `json:"Message"`
		} `json:"schemas"`
	} `json:"components"`
	Paths map[string]map[string]struct {
		Description string `json:"description"`
	} `json:"paths"`
}

func (c *Client) ContractIssues(ctx context.Context) ([]string, error) {
	_, issues, err := c.inspectSpec(ctx)
	return issues, err
}

func (c *Client) inspectSpec(ctx context.Context) (string, []string, error) {
	var spec specDocument
	if err := c.api.Get(ctx, "v1/spec", nil, &spec); err != nil {
		return "", nil, apiError(err)
	}
	var issues []string
	statuses := spec.Components.Schemas.SendStatus.Properties.Status.Enum
	expected := []string{
		string(shared.SendStatusStatusFailPermanent), string(shared.SendStatusStatusFailRetriable),
		string(shared.SendStatusStatusPending), string(shared.SendStatusStatusSuccess),
	}
	if slices.Sort(statuses); !slices.Equal(statuses, expected) {
		issues = append(issues, fmt.Sprintf("SendStatus values changed: %v", statuses))
	}
	if slices.Contains(spec.Components.Schemas.Message.Required, "sendStatus") {
		issues = append(issues, "Message.sendStatus became required; the absent-status rule needs review.")
	}
	message := spec.Paths["/v1/chats/{chatID}/messages/{messageID}"]["get"]
	if !strings.Contains(message.Description, "pendingMessageID") {
		issues = append(issues, "GET message no longer documents lookup by pendingMessageID.")
	}
	return strings.TrimSpace(spec.Info.Version), issues, nil
}
