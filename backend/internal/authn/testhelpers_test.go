package authn

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// These tests drive the middleware with real RS256 tokens against a local JWKS
// server, so the agency policy is exercised through the same path a request
// takes in production rather than against a stubbed token parser.

const (
	testKID      = "authn-test-key"
	testIssuer   = "https://idp.example.com"
	testAudience = "AGENCY_API"
	testClientID = "AGENCY_PORTAL"
	testOU       = "fcau"
)

func generateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

// jwksServer serves a JWKS document for key, and is torn down with the test.
func jwksServer(t *testing.T, key *rsa.PrivateKey) *httptest.Server {
	t.Helper()
	pub := &key.PublicKey
	body, err := json.Marshal(map[string]any{
		"keys": []map[string]any{{
			"kid": testKID,
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func validConfig(jwksURL string) Config {
	return Config{
		JWKSURL:    jwksURL,
		Issuer:     testIssuer,
		Audience:   testAudience,
		ClientIDs:  []string{testClientID},
		ExpectedOU: testOU,
	}
}

// keyring mints tokens signed with the key the manager's JWKS server publishes.
type keyring struct {
	t   *testing.T
	key *rsa.PrivateKey
}

func (k *keyring) sign(claims jwt.MapClaims) string {
	k.t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKID
	signed, err := tok.SignedString(k.key)
	if err != nil {
		k.t.Fatalf("sign token: %v", err)
	}
	return signed
}

// userClaims returns a complete, valid user token for the given OU. Individual
// tests delete or override entries to exercise a specific rule.
func userClaims(ouHandle string) jwt.MapClaims {
	return jwt.MapClaims{
		"iss":        testIssuer,
		"aud":        testAudience,
		"sub":        "sub-001",
		"exp":        time.Now().Add(time.Hour).Unix(),
		"client_id":  testClientID,
		"grant_type": "authorization_code",
		"email":      "officer@fcau.gov",
		"ouId":       "ou-123",
		"ouHandle":   ouHandle,
	}
}

func clientClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":        testIssuer,
		"aud":        testAudience,
		"sub":        testClientID,
		"exp":        time.Now().Add(time.Hour).Unix(),
		"client_id":  testClientID,
		"grant_type": "client_credentials",
	}
}

// stubProfiles is a UserProfileService whose outcome each test dictates.
type stubProfiles struct {
	returnID  string
	returnErr error
	calls     int
	lastSeen  *Principal
}

func (s *stubProfiles) GetOrCreateUser(_ context.Context, p *Principal) (string, error) {
	s.calls++
	s.lastSeen = p
	return s.returnID, s.returnErr
}

// serve runs a request bearing token through the manager's middleware and
// reports what the handler saw.
func serve(t *testing.T, m *Manager, token string) (status int, body string, reached bool, seen *Principal) {
	t.Helper()
	handler := m.RequireAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		seen, _ = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/applications", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w.Code, w.Body.String(), reached, seen
}
