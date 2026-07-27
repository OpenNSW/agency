package nswclient

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Command values understood by the NSW service callback envelope.
const (
	// CommandApprove is the default outcome command for a reviewed task.
	CommandApprove = "approve"
	// CommandRequestAmendment asks the trader to amend a submission.
	CommandRequestAmendment = "request-amendment"
)

// taskResponse is the "Style B" callback envelope sent back to the NSW service:
// an envelope carrying a command and its nested payload.
type taskResponse struct {
	Command string `json:"command"`
	Payload any    `json:"payload"`
}

// SendOutcome sends a review outcome (command + payload) for a task back to the
// originating NSW service.
func (c *Client) SendOutcome(ctx context.Context, serviceURL, taskID, command string, payload any) error {
	callbackURL := buildCallbackURL(serviceURL, taskID)
	if err := c.postEnvelope(ctx, callbackURL, taskResponse{Command: command, Payload: payload}); err != nil {
		return fmt.Errorf("send outcome to NSW service: %w", err)
	}
	return nil
}

// RequestAmendment asks the trader (via the NSW service) to amend a submission.
func (c *Client) RequestAmendment(ctx context.Context, serviceURL, taskID string, payload any) error {
	if err := c.SendOutcome(ctx, serviceURL, taskID, CommandRequestAmendment, payload); err != nil {
		return fmt.Errorf("request amendment via NSW service: %w", err)
	}
	return nil
}

// buildCallbackURL constructs the callback URL target. If serviceURL contains a
// "{id}" placeholder it is substituted; otherwise the taskID is appended as a
// single path segment, preserving any query string.
func buildCallbackURL(serviceURL, taskID string) string {
	if strings.Contains(serviceURL, "{id}") {
		return strings.ReplaceAll(serviceURL, "{id}", url.PathEscape(taskID))
	}
	u, err := url.Parse(serviceURL)
	if err != nil {
		return fmt.Sprintf("%s/%s", strings.TrimSuffix(serviceURL, "/"), url.PathEscape(taskID))
	}
	// JoinPath treats its arguments as already-escaped path elements, so escape
	// taskID first to keep a slash-containing ID within one segment.
	return u.JoinPath(url.PathEscape(taskID)).String()
}
