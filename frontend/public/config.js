// Placeholder only — never actually served. In dev, vite.config.ts proxies
// /config.js to the backend before Vite's static public-dir middleware would
// reach this file; in prod, the Go server registers an explicit /config.js
// route ahead of its static file server (see backend/internal/web). This file
// exists only so a build that serves dist/ directly, bypassing both, still
// gets a defined (if empty) window.__APP_CONFIG__ instead of a 404.
window.__APP_CONFIG__ = window.__APP_CONFIG__ || {}
