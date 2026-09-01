package nswclient

import (
	"net/url"
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{
		BaseURL:      "http://localhost:8080",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		TokenURL:     "https://idp.example.com/oauth2/token",
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *Config)
		wantErr string
	}{
		{
			name:   "valid bare origin",
			mutate: func(c *Config) {},
		},
		{
			name:    "missing base URL",
			mutate:  func(c *Config) { c.BaseURL = "" },
			wantErr: "nsw.baseURL is required",
		},
		{
			name:    "base URL with api/v1 path is rejected",
			mutate:  func(c *Config) { c.BaseURL = "http://localhost:8080/api/v1" },
			wantErr: `nsw.baseURL must be the NSW service origin only, with no path (got "/api/v1")`,
		},
		{
			// A future API version must not require touching this validation:
			// it rejects any leftover path, not a specific version string.
			name:    "base URL with a different version path is also rejected",
			mutate:  func(c *Config) { c.BaseURL = "http://localhost:8080/api/v2" },
			wantErr: `nsw.baseURL must be the NSW service origin only, with no path (got "/api/v2")`,
		},
		{
			name:   "base URL with only a trailing slash is a bare origin",
			mutate: func(c *Config) { c.BaseURL = "http://localhost:8080/" },
		},
		{
			name:    "unparseable base URL",
			mutate:  func(c *Config) { c.BaseURL = ":not-a-url" },
			wantErr: "nsw.baseURL must be a valid URL",
		},
		{
			name:    "missing client id",
			mutate:  func(c *Config) { c.ClientID = "" },
			wantErr: "nsw.clientID is required",
		},
		{
			name:    "missing client secret",
			mutate:  func(c *Config) { c.ClientSecret = "" },
			wantErr: "nsw.clientSecret is required",
		},
		{
			name:    "missing token url",
			mutate:  func(c *Config) { c.TokenURL = "" },
			wantErr: "nsw.tokenURL is required",
		},
		{
			name: "reserved token param rejected",
			mutate: func(c *Config) {
				c.TokenParams = url.Values{"client_id": []string{"dup"}}
			},
			wantErr: `nsw.tokenParams must not set "client_id"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(&c)

			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
