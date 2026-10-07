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

// callbackPath is the NSW API's callback endpoint. The callback token from the
// inject is appended as a single path segment.
const callbackPath = "api/v1/callbacks"

// taskResponse is the callback envelope sent to the NSW service: a command and
// its nested payload.
type taskResponse struct {
	Command string `json:"command"`
	Payload any    `json:"payload"`
}

// SendOutcome sends a review outcome (command + payload) back to the NSW service
// for the step the inject was for. callbackToken is the opaque token the inject
// carried: it names that one step, so NSW rejects (409) an outcome sent after the
// task has moved on rather than applying it to a later step.
func (c *Client) SendOutcome(ctx context.Context, callbackToken, command string, payload any) error {
	if callbackToken == "" {
		return fmt.Errorf("send outcome to NSW service: no callback token")
	}
	// JoinPath treats its arguments as already-escaped path elements, so escape
	// the token first to keep it within one segment whatever it contains.
	path, err := url.JoinPath(callbackPath, url.PathEscape(callbackToken))
	if err != nil {
		return fmt.Errorf("build callback path: %w", err)
	}
	if err := c.postEnvelope(ctx, path, callbackToken, taskResponse{Command: command, Payload: payload}); err != nil {
		return fmt.Errorf("send outcome to NSW service: %w", err)
	}
	return nil
}

// RequestAmendment asks the trader (via the NSW service) to amend a submission.
func (c *Client) RequestAmendment(ctx context.Context, callbackToken string, payload any) error {
	if err := c.SendOutcome(ctx, callbackToken, CommandRequestAmendment, payload); err != nil {
		return fmt.Errorf("request amendment via NSW service: %w", err)
	}
	return nil
}
