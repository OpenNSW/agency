package httpclient

import (
	"net/http"
	"net/url"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// OAuth2Authenticator implements the Client Credentials flow.
type OAuth2Authenticator struct {
	config *clientcredentials.Config

	// mu guards the cached access token, which is reused across requests and
	// refreshed (via a fresh client-credentials fetch) once it nears expiry.
	mu    sync.Mutex
	token *oauth2.Token
}

// OAuth2Option configures an OAuth2Authenticator.
type OAuth2Option func(*clientcredentials.Config)

// WithEndpointParams adds extra parameters to the token request, sent in the body
// alongside grant_type and scope (RFC 6749 §3.2).
func WithEndpointParams(params url.Values) OAuth2Option {
	return func(c *clientcredentials.Config) {
		c.EndpointParams = params
	}
}

// NewOAuth2Authenticator creates a new OAuth2Authenticator.
func NewOAuth2Authenticator(
	clientID, clientSecret, tokenURL string,
	scopes []string,
	opts ...OAuth2Option,
) *OAuth2Authenticator {
	cfg := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		Scopes:       scopes,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &OAuth2Authenticator{config: cfg}
}

// Authenticate fetches a token if necessary and injects it into the request header.
func (o *OAuth2Authenticator) Authenticate(req *http.Request) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Token.Valid() is nil-safe and treats a token within ~10s of expiry as
	// invalid, so we refresh proactively. Fetch using the request's context so
	// the token call uses the HTTP client Client.Do injects and honours the
	// request's timeout/cancellation, rather than a long-lived background context.
	if !o.token.Valid() {
		token, err := o.config.TokenSource(req.Context()).Token()
		if err != nil {
			return err
		}
		o.token = token
	}

	o.token.SetAuthHeader(req)
	return nil
}
