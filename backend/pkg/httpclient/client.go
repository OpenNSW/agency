package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// TLSConfig holds TLS-specific configuration.
type TLSConfig struct {
	InsecureSkipVerify bool // If true, TLS certificate verification is disabled (development only)
}

// Config holds generic HTTP client configuration.
type Config struct {
	Timeout time.Duration
	TLS     *TLSConfig
}

// Client is a wrapper around *http.Client with built-in authentication and a base URL.
type Client struct {
	httpClient *http.Client
	auth       Authenticator
	BaseURL    string
}

// NewHTTPClient creates an HTTP client from a Config.
func NewHTTPClient(config Config) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	if config.TLS != nil {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: config.TLS.InsecureSkipVerify,
		}
		if config.TLS.InsecureSkipVerify {
			slog.Warn("TLS certificate verification disabled - use only in development")
		}
	}

	return &http.Client{
		Timeout:   config.Timeout,
		Transport: transport,
		// The NSW client posts callbacks to a caller-supplied URL that is
		// validated against a trusted origin (see
		// application.validateServiceURLOrigin). Following redirects would let
		// a redirect on that trusted origin retarget the request to an
		// arbitrary host, bypassing the origin check (SSRF).
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// ClientBuilder provides a fluent API for building HTTP clients.
type ClientBuilder struct {
	baseURL    string
	config     Config
	auth       Authenticator
	transport  *http.Transport
	httpClient *http.Client
}

// NewClientBuilder creates a new ClientBuilder with sensible defaults.
func NewClientBuilder() *ClientBuilder {
	return &ClientBuilder{
		config: Config{
			Timeout: 0,
		},
	}
}

// WithBaseURL sets the base URL.
func (cb *ClientBuilder) WithBaseURL(baseURL string) *ClientBuilder {
	cb.baseURL = baseURL
	return cb
}

// WithTimeout sets the HTTP timeout.
func (cb *ClientBuilder) WithTimeout(timeout time.Duration) *ClientBuilder {
	cb.config.Timeout = timeout
	return cb
}

// WithTLS sets the TLS configuration.
func (cb *ClientBuilder) WithTLS(tlsConfig *TLSConfig) *ClientBuilder {
	cb.config.TLS = tlsConfig
	return cb
}

// WithAuthenticator sets the authenticator.
func (cb *ClientBuilder) WithAuthenticator(auth Authenticator) *ClientBuilder {
	cb.auth = auth
	return cb
}

// WithTransport sets a shared transport for the HTTP client.
// This allows multiple clients to reuse the same connection pool.
func (cb *ClientBuilder) WithTransport(transport *http.Transport) *ClientBuilder {
	cb.transport = transport
	cb.httpClient = nil
	return cb
}

// WithHTTPClient sets a pre-configured HTTP client.
// When set, builder timeout and TLS settings are ignored in favor of the provided client.
func (cb *ClientBuilder) WithHTTPClient(httpClient *http.Client) *ClientBuilder {
	cb.httpClient = httpClient
	cb.transport = nil
	return cb
}

// Build creates the Client.
func (cb *ClientBuilder) Build() *Client {
	baseURL := cb.baseURL
	if baseURL != "" && !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	httpClient := cb.httpClient
	if httpClient == nil {
		if cb.transport != nil {
			httpClient = &http.Client{
				Timeout:   cb.config.Timeout,
				Transport: cb.transport,
			}
		} else {
			httpClient = NewHTTPClient(cb.config)
		}
	}

	return &Client{
		httpClient: httpClient,
		auth:       cb.auth,
		BaseURL:    baseURL,
	}
}

// Do performs an HTTP request and applies authentication.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("http request is nil")
	}
	if c.auth != nil && c.shouldAuthenticate(req) {
		// Inject HTTP client into the context so OAuth2 token fetches
		// use the same transport (e.g. InsecureSkipVerify TLS config).
		ctx := context.WithValue(req.Context(), oauth2.HTTPClient, c.httpClient)
		req = req.WithContext(ctx)

		if err := c.auth.Authenticate(req); err != nil {
			return nil, err
		}
	}
	return c.httpClient.Do(req)
}

// shouldAuthenticate checks if the request URL aligns with the BaseURL.
func (c *Client) shouldAuthenticate(req *http.Request) bool {
	if c.BaseURL == "" {
		return true
	}
	baseURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return false
	}
	return req.URL.Host == baseURL.Host && req.URL.Scheme == baseURL.Scheme
}

// resolveURL joins the base URL with the provided path.
func (c *Client) resolveURL(path string) (string, error) {
	if c.BaseURL == "" {
		return path, nil
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", err
	}
	// If path is already an absolute URL, use it directly
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path, nil
	}
	rel, err := url.Parse(strings.TrimPrefix(path, "/"))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(rel).String(), nil
}

// Get performs a GET request relative to the BaseURL.
func (c *Client) Get(path string) (*http.Response, error) {
	fullURL, err := c.resolveURL(path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Post performs a POST request relative to the BaseURL.
func (c *Client) Post(path string, contentType string, body []byte) (*http.Response, error) {
	fullURL, err := c.resolveURL(path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}
