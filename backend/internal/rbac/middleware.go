package rbac

import (
	"context"
	"errors"
	"net/http"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/nsw-agency/backend/internal/auth"
	"github.com/OpenNSW/nsw-agency/backend/internal/taskconfig"
	"github.com/OpenNSW/nsw-agency/backend/internal/taskconfig/taskconfigart"
)

// TaskCodeResolver resolves a task's task_code from its task_id.
type TaskCodeResolver interface {
	GetTaskCode(ctx context.Context, taskID string) (string, error)
}

// Middleware enforces role-based access control on task routes.
type Middleware struct {
	roleService      *RoleService
	taskCodeResolver TaskCodeResolver
	artifactRegistry *artifact.Registry
}

// NewMiddleware creates a new RBAC Middleware.
func NewMiddleware(roleService *RoleService, taskCodeResolver TaskCodeResolver, artifactRegistry *artifact.Registry) *Middleware {
	return &Middleware{
		roleService:      roleService,
		taskCodeResolver: taskCodeResolver,
		artifactRegistry: artifactRegistry,
	}
}

// RequireAction returns middleware that enforces the given action is permitted
// for the authenticated user on the requested task. If the task config defines
// no permissions, all authenticated users are allowed (current behaviour preserved).
func (m *Middleware) RequireAction(action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			taskID := r.PathValue("taskId")
			if taskID == "" {
				httputil.Error(w, r, http.StatusBadRequest, "taskId is required")
				return
			}

			taskCode, err := m.taskCodeResolver.GetTaskCode(ctx, taskID)
			if err != nil {
				httputil.InternalServerError(w, r, "rbac: failed to resolve task code", err, "taskId", taskID)
				return
			}

			cfg, err := taskconfigart.Load(ctx, m.artifactRegistry, taskCode)
			if err != nil {
				if errors.Is(err, artifact.ErrNotFound) {
					// No task config for this code — preserve current behaviour,
					// allow all authenticated users.
					next.ServeHTTP(w, r)
					return
				}
				// A genuine load failure (network, credentials, malformed config)
				// must fail closed: allowing the request through on a transient
				// loader error would silently bypass RBAC.
				httputil.InternalServerError(w, r, "rbac: failed to load task config", err, "taskCode", taskCode)
				return
			}
			if len(cfg.Permissions) == 0 {
				// No permissions defined — allow all authenticated users.
				next.ServeHTTP(w, r)
				return
			}

			authCtx := auth.GetAuthContext(ctx)
			if authCtx == nil || authCtx.User == nil {
				httputil.Error(w, r, http.StatusUnauthorized, "unauthorized")
				return
			}

			roles, err := m.roleService.GetRolesForUser(authCtx.User.ID)
			if err != nil {
				httputil.InternalServerError(w, r, "rbac: failed to get roles for user", err, "userID", authCtx.User.ID)
				return
			}

			_, allowedActions := ResolveAccess(roles, cfg.Permissions)
			if !hasAction(allowedActions, action) {
				httputil.Error(w, r, http.StatusForbidden, "access denied")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ResolveAccess returns whether the user has access to the task (isAccessible)
// and the union of actions they may perform, based on their roles and the task's
// permission configuration. Returns (false, nil) when no role matches.
func ResolveAccess(roles []RoleRecord, permissions []taskconfig.Permission) (bool, []string) {
	roleSet := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		roleSet[r.Name] = struct{}{}
	}

	isAccessible := false
	seen := make(map[string]struct{})
	var actions []string
	for _, p := range permissions {
		if _, ok := roleSet[p.Role]; !ok {
			continue
		}
		isAccessible = true
		for _, a := range p.Actions {
			if _, exists := seen[a]; !exists {
				seen[a] = struct{}{}
				actions = append(actions, a)
			}
		}
	}
	return isAccessible, actions
}

// hasAction reports whether action exists in the provided actions slice.
func hasAction(actions []string, action string) bool {
	for _, a := range actions {
		if a == action {
			return true
		}
	}
	return false
}
