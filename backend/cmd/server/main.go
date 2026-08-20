package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/trace"
	"github.com/OpenNSW/nsw-agency/backend/internal/application"
	"github.com/OpenNSW/nsw-agency/backend/internal/authn"
	"github.com/OpenNSW/nsw-agency/backend/internal/consignment"
	"github.com/OpenNSW/nsw-agency/backend/internal/feedback"
	"github.com/OpenNSW/nsw-agency/backend/internal/logging"
	"github.com/OpenNSW/nsw-agency/backend/internal/nswclient"
	"github.com/OpenNSW/nsw-agency/backend/internal/rbac"
	"github.com/OpenNSW/nsw-agency/backend/internal/storage"
	"github.com/OpenNSW/nsw-agency/backend/internal/user"
	"github.com/OpenNSW/nsw-agency/backend/internal/web"
)

func main() {
	logging.ConfigureLogging(os.Stdout)

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("FATAL: failed to load configuration: %v", err)
	}

	dbTarget := cfg.DB.SQLite.Path
	if cfg.DB.Driver == "postgres" {
		dbTarget = net.JoinHostPort(cfg.DB.Postgres.Host, cfg.DB.Postgres.Port) + "/" + cfg.DB.Postgres.Name
	}

	slog.Info("NSW Agency service configuration",
		"db_driver", cfg.DB.Driver,
		"db_target", dbTarget,
		"port", cfg.Port,
	)

	// Initialize database store
	store, err := application.NewApplicationStore(cfg.DB)
	if err != nil {
		log.Fatalf("failed to create application store: %v", err)
	}

	// Initialize user store
	userStore, err := user.NewUserStore(cfg.DB)
	if err != nil {
		log.Fatalf("failed to create user store: %v", err)
	}
	defer func() {
		if err := userStore.Close(); err != nil {
			slog.Error("failed to close user store", "error", err)
		}
	}()

	// Initialize auth manager, resolving callers to seeded user records.
	authManager, err := authn.NewManager(&userProfileAdapter{store: userStore}, cfg.Authn)
	if err != nil {
		log.Fatalf("failed to initialize auth manager: %v", err)
	}
	defer func() {
		if err := authManager.Close(); err != nil {
			slog.Error("failed to close auth manager", "error", err)
		}
	}()

	// NSW client: anti-corruption layer that owns the NSW HTTP transport,
	// OAuth2 credentials, and wire protocol.
	nswClient := nswclient.New(cfg.NSW)

	// Bound startup on the remote artifact store: a slow or unreachable backend
	// (GitHub / S3) must fail fast with a clear error rather than hang the boot.
	initCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	artifactLoader, err := loaders.New(initCtx, cfg.ArtifactLoader)
	if err != nil {
		log.Fatalf("failed to initialize artifact loader: %v", err)
	}

	artifactRegistry := artifact.NewRegistry(artifactLoader)

	manifestCfg, err := artifact.LoadManifest(initCtx, artifactLoader)
	if err != nil {
		log.Fatalf("failed to load artifact manifest: %v", err)
	}
	if err := artifact.RegisterFromConfig(artifactRegistry, manifestCfg); err != nil {
		log.Fatalf("failed to register artifacts from manifest: %v", err)
	}

	// Initialize RBAC Service and Middleware
	roleService := rbac.NewRoleService(store.DB())
	rbacMiddleware := rbac.NewMiddleware(roleService, store, artifactRegistry)

	// Initialize consignment service (read-only summaries over the shared DB connection)
	consignmentStore := consignment.NewConsignmentStore(store.DB())
	consignmentHandler := consignment.NewHandler(consignment.NewService(consignmentStore))

	// Initialize Agency service
	service := application.NewService(store, artifactRegistry, nswClient, roleService)
	defer func() {
		if err := service.Close(); err != nil {
			slog.Error("failed to close service", "error", err)
		}
	}()

	// Initialize handlers
	handler, err := application.NewHandler(service, cfg.MaxRequestBytes)
	if err != nil {
		slog.Error("failed to create Agency handler", "error", err)
		return
	}

	profileSvc := user.NewProfileService(roleService)
	profileHandler := user.NewProfileHandler(profileSvc)

	// Initialize storage handler (delegates NSW backend calls to nswClient)
	storageHandler, err := storage.NewHandler(nswClient, cfg.MaxRequestBytes)
	if err != nil {
		slog.Error("failed to create storage handler", "error", err)
		return
	}

	feedbackHandler, err := feedback.NewHandler(service, cfg.MaxRequestBytes)
	if err != nil {
		slog.Error("failed to create feedback handler", "error", err)
		return
	}

	// Set up HTTP routes
	mux := http.NewServeMux()
	// Health check
	mux.HandleFunc("GET /health", handler.HandleHealth)

	// Shared auth middleware: requires a valid IdP token whose client_id is in
	// AUTH_CLIENT_IDS and aud=AGENCY_API.
	protect := authManager.RequireAuthMiddleware()

	// Endpoint for services to inject data (service-to-service M2M). Protected by
	// the same auth middleware; the NSW->Agency M2M client (e.g. NSW_TO_NPQS) is
	// whitelisted in AUTH_CLIENT_IDS so NSW core authenticates with its
	// client_credentials token.
	mux.Handle("POST /api/v1/inject", protect(http.HandlerFunc(handler.HandleInjectData)))

	// Endpoints for UI to fetch and manage applications (protected by JIT user auth)
	mux.Handle("GET /api/v1/consignments", protect(http.HandlerFunc(consignmentHandler.HandleGetConsignments)))
	mux.Handle("GET /api/v1/applications", protect(http.HandlerFunc(handler.HandleGetApplications)))
	mux.Handle("GET /api/v1/users/me", protect(http.HandlerFunc(profileHandler.HandleMe)))
	mux.Handle("GET /api/v1/applications/{taskId}", protect(rbacMiddleware.RequireAction("VIEW")(http.HandlerFunc(handler.HandleGetApplication))))
	mux.Handle("POST /api/v1/applications/{taskId}/review", protect(rbacMiddleware.RequireAction("REVIEW")(http.HandlerFunc(handler.HandleReviewApplication))))
	mux.Handle("POST /api/v1/applications/{taskId}/feedback", protect(rbacMiddleware.RequireAction("FEEDBACK")(http.HandlerFunc(feedbackHandler.HandleFeedback))))
	mux.Handle("POST /api/v1/storage", protect(http.HandlerFunc(storageHandler.HandleCreateUpload)))
	mux.Handle("GET /api/v1/storage/{key}", protect(http.HandlerFunc(storageHandler.HandleGetUploadURL)))

	// Serve the built officer-portal SPA from this same process. The "/" pattern
	// is the most general match, so the specific API, /health and /runtime-env.js
	// routes take precedence. /runtime-env.js exposes cfg.Web.Runtime
	// (window.__APP_CONFIG__) so the SPA reads config synchronously. cfg.Web.Dir
	// is relative to the working dir (/app/web in the image); when it is absent
	// (e.g. local API-only dev where the frontend runs via its own dev server),
	// skip serving rather than failing.
	if spa, err := web.NewHandler(cfg.Web); err != nil {
		slog.Warn("frontend not served", "asset_dir", cfg.Web.Dir, "error", err)
	} else {
		if err := cfg.Web.Validate(); err != nil {
			log.Fatalf("FATAL: serving the frontend but its config is invalid: %v", err)
		}
		mux.Handle("GET /runtime-env.js", http.HandlerFunc(spa.ServeRuntimeEnv))
		mux.Handle("GET /", http.HandlerFunc(spa.ServeSPA))
		slog.Info("serving officer portal alongside the API", "asset_dir", cfg.Web.Dir)
	}

	// Set up graceful shutdown
	serverAddr := fmt.Sprintf(":%s", cfg.Port)

	// CORS middleware
	allowAll := len(cfg.AllowedOrigins) == 1 && cfg.AllowedOrigins[0] == "*"
	allowedSet := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		allowedSet[o] = struct{}{}
	}

	corsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowAll {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if _, ok := allowedSet[origin]; ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		mux.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:              serverAddr,
		Handler:           trace.TraceMiddleware(corsHandler),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	// Channel to listen for interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		slog.Info("starting Agency service", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("failed to start server", "error", err)
			quit <- syscall.SIGTERM
		}
	}()

	// Wait for interrupt signal
	<-quit
	slog.Info("shutting down Agency service...")

	// Create a context with timeout for graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Attempt graceful shutdown of HTTP server
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	} else {
		slog.Info("server gracefully stopped")
	}

	slog.Info("NSW Agency service stopped")
}
