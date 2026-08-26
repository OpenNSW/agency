package nswclient

import (
	"context"
	"fmt"
	"net/url"
)

// Command values understood by the NSW service callback envelope.
const (
	// CommandApprove is the default outcome command for a reviewed task.
	CommandApprove = "approve"
	// CommandRequestAmendment asks the trader to amend a submission.
	CommandRequestAmendment = "request-amendment"
)

// taskCallbackPath is the NSW API's task callback endpoint. The task ID is
// appended as a single path segment.
const taskCallbackPath = "api/v1/tasks"

// taskResponse is the callback envelope sent to the NSW service: a command and
// its nested payload.
type taskResponse struct {
	Command string `json:"command"`
	Payload any    `json:"payload"`
}

// SendOutcome sends a review outcome (command + payload) for a task back to the
// NSW service.
func (c *Client) SendOutcome(ctx context.Context, taskID, command string, payload any) error {
	// JoinPath treats its arguments as already-escaped path elements, so escape
	// taskID first to keep a slash-containing ID within one segment.
	path, err := url.JoinPath(taskCallbackPath, url.PathEscape(taskID))
	if err != nil {
		return fmt.Errorf("build task callback path: %w", err)
	}
	if err := c.postEnvelope(ctx, path, taskID, taskResponse{Command: command, Payload: payload}); err != nil {
		return fmt.Errorf("send outcome to NSW service: %w", err)
	}
	return nil
}

// RequestAmendment asks the trader (via the NSW service) to amend a submission.
func (c *Client) RequestAmendment(ctx context.Context, taskID string, payload any) error {
	if err := c.SendOutcome(ctx, taskID, CommandRequestAmendment, payload); err != nil {
		return fmt.Errorf("request amendment via NSW service: %w", err)
	}
	return nil
}
