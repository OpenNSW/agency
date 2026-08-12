package authn

import (
	"fmt"
	"log/slog"
	"net/http"

	coreauthn "github.com/OpenNSW/core/authn"
)

// Response bodies for the two rejections this package owns. They are written
// directly rather than through httputil so the wire format stays exactly what
// the SPA has always received.
const (
	forbiddenBody    = `{"error":"forbidden","message":"access denied"}`
	unauthorizedBody = `{"error":"unauthorized","message":"invalid authentication token"}`
)

// Manager owns the underlying core/authn manager and this agency's access
// policy. It exposes only what the server actually uses: one middleware and a
// shutdown hook.
type Manager struct {
	core       *coreauthn.Manager
	profiles   UserProfileService
	expectedOU string
}

// NewManager builds the authentication manager. userProfileService is optional;
// pass nil to skip resolving callers to persisted user records.
//
// The config is validated here so a bad configuration fails at construction
// rather than on the first request; core/authn's own constructor does not
// validate.
func NewManager(userProfileService UserProfileService, cfg Config) (*Manager, error) {
	slog.Info("initializing auth manager", "user_profile_service_enabled", userProfileService != nil)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid auth config: %w", err)
	}

	// core/authn is given no profile service on purpose. It would call one from
	// inside its own middleware, i.e. before this package gets to check the OU,
	// and a cross-agency token must never reach the database. Resolution
	// therefore happens in enforce, after the OU gate.
	core, err := coreauthn.NewManager(nil, cfg.coreConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth manager: %w", err)
	}

	return &Manager{
		core:       core,
		profiles:   userProfileService,
		expectedOU: cfg.ExpectedOU,
	}, nil
}

// RequireAuthMiddleware rejects any request that is not an authenticated caller
// of this agency, and attaches the caller's Principal for handlers to read with
// FromContext.
//
// The chain is: core/authn validates the token and rejects an absent or invalid
// one with 401, then enforce applies this agency's policy.
func (m *Manager) RequireAuthMiddleware() func(http.Handler) http.Handler {
	requireAuth := m.core.RequireAuthMiddleware()
	return func(next http.Handler) http.Handler {
		return requireAuth(m.enforce(next))
	}
}

// enforce runs the agency policy on a token core/authn has already validated:
// the caller's OU must match, and a user must resolve to a persisted record.
// Both rejections return 403 with an identical body, deliberately saying no
// more than "access denied".
func (m *Manager) enforce(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		principal := principalFromAuthContext(coreauthn.GetAuthContext(ctx))
		if principal == nil {
			// core/authn already 401s a request with no auth context; this
			// covers a context carrying neither a user nor a client, so an
			// unidentifiable caller fails closed rather than reaching a handler.
			slog.ErrorContext(ctx, "auth: authenticated request carried no principal")
			writeJSON(w, http.StatusUnauthorized, unauthorizedBody)
			return
		}

		// Reject cross-agency tokens before resolving the user, so a token from
		// another agency never reaches the database.
		if principal.Kind == KindUser && principal.OUHandle != m.expectedOU {
			slog.WarnContext(ctx, "auth: OU handle mismatch", "expected", m.expectedOU, "got", principal.OUHandle)
			writeJSON(w, http.StatusForbidden, forbiddenBody)
			return
		}

		if principal.Kind == KindUser && m.profiles != nil {
			userID, err := m.profiles.GetOrCreateUser(ctx, principal)
			if err != nil {
				slog.ErrorContext(ctx, "failed to resolve user profile", "idp_user_id", principal.IDPUserID, "error", err)
			} else {
				principal.UserID = userID
			}

			// A caller we could not resolve is not a user of this agency.
			// Resolution failures are logged rather than propagated, so an
			// empty ID is the only signal available here.
			if principal.UserID == "" {
				writeJSON(w, http.StatusForbidden, forbiddenBody)
				return
			}
			slog.DebugContext(ctx, "resolved user profile", "idp_user_id", principal.IDPUserID, "user_id", principal.UserID)
		}

		next.ServeHTTP(w, r.WithContext(ContextWithPrincipal(ctx, principal)))
	})
}

// Close releases the manager's resources.
func (m *Manager) Close() error { return m.core.Close() }

// principalFromAuthContext maps core/authn's request context onto a Principal,
// reading the claim names declared in Config.coreConfig. It returns nil when
// the context identifies neither a user nor a client.
func principalFromAuthContext(ac *coreauthn.AuthContext) *Principal {
	if ac == nil {
		return nil
	}
	switch {
	case ac.User != nil:
		u := ac.User
		return &Principal{
			Kind:        KindUser,
			UserID:      u.ID,
			IDPUserID:   u.IDPUserID,
			Roles:       u.Roles,
			Scopes:      u.Scopes,
			Email:       u.ExtraClaims.String(claimEmail),
			GivenName:   u.ExtraClaims.String(claimGivenName),
			PhoneNumber: u.ExtraClaims.String(claimPhoneNumber),
			OUID:        u.ExtraClaims.String(claimOUID),
			OUHandle:    u.ExtraClaims.String(claimOUHandle),
		}
	case ac.Client != nil:
		c := ac.Client
		return &Principal{
			Kind:     KindClient,
			ClientID: c.ClientID,
			Roles:    c.Roles,
			Scopes:   c.Scopes,
		}
	default:
		return nil
	}
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
