package nswclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/OpenNSW/nsw-agency/backend/pkg/httpclient"
)

// defaultTimeout is the HTTP timeout applied to NSW service calls.
const defaultTimeout = 10 * time.Second

// Client speaks the NSW service wire protocol over an authenticated HTTP
// transport. It is safe for concurrent use if the underlying transport is.
type Client struct {
	http *httpclient.Client
}

// New creates a Client that talks to the NSW service using OAuth2 credentials
// from cfg. It builds the authenticated HTTP transport internally.
func New(cfg Config) *Client {
	var opts []httpclient.OAuth2Option
	if len(cfg.TokenParams) > 0 {
		opts = append(opts, httpclient.WithEndpointParams(cfg.TokenParams))
	}

	authenticator := httpclient.NewOAuth2Authenticator(
		cfg.ClientID,
		cfg.ClientSecret,
		cfg.TokenURL,
		cfg.Scopes,
		opts...,
	)

	hc := httpclient.NewClientBuilder().
		WithBaseURL(cfg.BaseURL).
		WithTimeout(defaultTimeout).
		WithAuthenticator(authenticator).
		WithTLS(&httpclient.TLSConfig{InsecureSkipVerify: cfg.TokenInsecureSkipVerify}).
		Build()

	return NewWithClient(hc)
}

// NewWithClient creates a Client backed by a pre-built HTTP transport. It is the
// injection seam used by tests; production code should prefer New.
func NewWithClient(hc *httpclient.Client) *Client {
	if hc == nil {
		panic("nswclient.NewWithClient: http client must be non-nil")
	}
	return &Client{http: hc}
}

// postEnvelope marshals body to JSON, POSTs it to url, and returns an error
// unless the response status is 2xx. The response body is drained and closed.
func (c *Client) postEnvelope(ctx context.Context, url, taskID string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// The URL may carry callback credentials in its query string and the payload
	// may carry reviewer responses, so neither is logged.
	slog.DebugContext(ctx, "nswclient: sending callback request", "taskID", taskID)

	resp, err := c.http.Post(url, "application/json", data)
	if err != nil {
		return err
	}
	defer closeBody(ctx, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("nsw service returned status %d", resp.StatusCode)
	}
	return nil
}

// closeBody closes an HTTP response body, logging any error.
func closeBody(ctx context.Context, body io.ReadCloser) {
	if err := body.Close(); err != nil {
		slog.ErrorContext(ctx, "nswclient: failed to close response body", "error", err)
	}
}
