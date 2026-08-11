package nswclient

import (
	"fmt"
	"net/url"
	"strings"
)

// reservedTokenParams are set by the client-credentials flow itself. Letting configuration
// override them would either break the grant or send credentials by two routes at once.
var reservedTokenParams = []string{"grant_type", "scope", "client_id", "client_secret"}

// Config holds the connection and OAuth2 credentials for the NSW service.
type Config struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	TokenURL     string
	Scopes       []string
	// TokenParams carries extra token-request parameters, sent in the body (RFC 6749
	// §3.2). Which ones are needed is a property of the authorization server, not of
	// this client. Left empty, nothing extra is sent.
	TokenParams             url.Values
	TokenInsecureSkipVerify bool
}

// Validate ensures the required OAuth2 connection fields are present.
func (c Config) Validate() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("NSW_API_BASE_URL is required")
	}
	if strings.TrimSpace(c.ClientID) == "" {
		return fmt.Errorf("NSW_CLIENT_ID is required")
	}
	if strings.TrimSpace(c.ClientSecret) == "" {
		return fmt.Errorf("NSW_CLIENT_SECRET is required")
	}
	if strings.TrimSpace(c.TokenURL) == "" {
		return fmt.Errorf("NSW_TOKEN_URL is required")
	}
	for _, k := range reservedTokenParams {
		if _, ok := c.TokenParams[k]; ok {
			return fmt.Errorf("NSW_TOKEN_PARAMS must not set %q: it is set by the client-credentials flow", k)
		}
	}
	return nil
}
