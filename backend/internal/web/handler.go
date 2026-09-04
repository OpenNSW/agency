// Package web serves the built officer-portal SPA from a directory on disk so
// the API and the frontend can ship in a single image.
//
// It also serves /config.js, rendered from RuntimeConfig loaded at startup,
// which the browser loads before the app bundle to populate window.__APP_CONFIG__.
// This is the single source of runtime config — there is no entrypoint script
// writing a file. /config.js is served independently of the SPA asset dir: it is
// available even when this process is API-only (asset dir absent), since the
// frontend's Vite dev server proxies /config.js to this same backend instance
// (see frontend/vite.config.ts) rather than reimplementing config assembly.
package web

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Handler serves /config.js always, and the built officer-portal SPA when its
// asset dir exists. Build it with NewHandler, which marshals the runtime config
// once and stats the asset dir, then wire ServeConfig (always) and ServeSPA
// (only if ServesSPA) onto a mux — see cmd/server/main.go.
type Handler struct {
	dir            string
	fileServer     http.Handler
	spaAvailable   bool
	runtimePayload []byte // pre-marshaled window.__APP_CONFIG__ JSON object
}

// NewHandler builds a Handler for cfg. cfg.Runtime should already be validated
// (see Config.Validate) — /config.js is served regardless of whether the SPA
// asset dir exists, since dev's Vite proxy and prod's bundled SPA both need it.
// A missing cfg.Dir is not an error: ServesSPA reports false and the caller
// skips registering ServeSPA (native dev's API-only mode). Any other os.Stat
// error (permissions, I/O) is a real problem, not an absent dir, so it's
// returned rather than silently treated as API-only.
func NewHandler(cfg Config) (*Handler, error) {
	// json.Marshal handles JS string escaping safely (replacing the old
	// hand-rolled awk escaper); omitempty drops unset optional keys.
	payload, err := json.Marshal(cfg.Runtime)
	if err != nil {
		return nil, err
	}
	h := &Handler{runtimePayload: payload}

	fi, statErr := os.Stat(cfg.Dir)
	switch {
	case statErr == nil && fi.IsDir():
		h.dir = cfg.Dir
		h.fileServer = http.FileServer(http.Dir(cfg.Dir))
		h.spaAvailable = true
	case statErr != nil && !errors.Is(statErr, fs.ErrNotExist):
		return nil, statErr
	}
	return h, nil
}

// ServesSPA reports whether cfg.Dir existed at NewHandler time, i.e. whether
// ServeSPA should be registered at all.
func (h *Handler) ServesSPA() bool {
	return h.spaAvailable
}

// ServeConfig serves /config.js. The browser loads it via <script src> before
// the app bundle, so window.__APP_CONFIG__ is available synchronously to the
// SPA's module-level config reads. Never cached.
func (h *Handler) ServeConfig(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w.Header())
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	_, _ = w.Write([]byte("window.__APP_CONFIG__ = "))
	_, _ = w.Write(h.runtimePayload)
	_, _ = w.Write([]byte(";\n"))
}

// ServeSPA serves the built frontend with SPA fallback: requests for paths that
// don't map to a real file return index.html so client-side routing works.
// Hashed assets under /assets/ are cached immutably; index.html is never cached.
// (/config.js is served by ServeConfig, not from disk.)
//
// A missing path that *looks* like a static asset (under assets/ or with a file
// extension) returns 404 rather than the index.html shell. Otherwise the browser
// receives HTML for a missing .js/.css and fails with "Unexpected token '<'".
// This matches the old nginx config, whose /assets/ location had no index
// fallback. The os.Stat below is served from the OS page cache, so it is cheap.
//
// Only meaningful when ServesSPA() is true — callers must not register this
// route otherwise (see cmd/server/main.go).
func (h *Handler) ServeSPA(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w.Header())

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}

	info, statErr := os.Stat(filepath.Join(h.dir, filepath.FromSlash(name)))
	if statErr != nil || info.IsDir() {
		if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		// Unknown client-side route -> serve the SPA shell.
		h.serveIndex(w)
		return
	}

	switch {
	case strings.HasPrefix(name, "assets/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case name == "index.html":
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	}
	h.fileServer.ServeHTTP(w, r)
}

func (h *Handler) serveIndex(w http.ResponseWriter) {
	data, err := os.ReadFile(filepath.Join(h.dir, "index.html"))
	if err != nil {
		http.Error(w, "frontend not available", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	_, _ = w.Write(data)
}

// setSecurityHeaders mirrors the headers nginx set on SPA responses in the
// previous split deployment. Only the frontend responses carry these.
func setSecurityHeaders(h http.Header) {
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
}
