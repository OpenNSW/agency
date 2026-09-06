# Agency App

## Authentication configuration

This app uses Asgardeo/Thunder OIDC for sign-in.

None of these are read from the environment/`.env` at runtime any more (see
[Configuration](#configuration) below) — they're `web.runtime` fields in the
backend's `config.yaml` (see `backend/config.example.yaml`), served to the
browser at `/config.js`:

- `apiBaseURL`: Agency backend API base URL (for example `http://localhost:8081`)
- `idpBaseURL`: IdP base URL (for example `https://localhost:8090`)
- `idpClientID`: NSW Agency-specific IdP application client id
- `idpExpectedOU`: Required organization/OU handle for access restriction (e.g., `npqs`, `fcau`, `cda`, `slpa`)
- `appURL`: public URL of this Agency deployment
- `idpScopes` (optional): comma-separated scopes (defaults to `openid,profile,email,ou,role,agency:application:read,agency:application:review,agency:application:feedback,agency:consignment:read,agency:storage:read,agency:storage:write`)
- `idpExtraQueryParams`: extra `/authorize` parameters, query-string encoded (for example `resource=https://api.nsw-agency.local`). ThunderID requires an RFC 8707 `resource` indicator naming the AGENCY_API resource server; without it the `agency:*` scopes are dropped from the issued token. Optional only for an IdP that binds tokens by scope alone.

## Per-NSW Agency deployment model

Each Agency deployment should use its own IdP application configuration, set in
that agency's `backend/config/<agency>/config.yaml` (`web.runtime`):

- NPQS deployment
  - `idpClientID: AGENCY_PORTAL_APP_NPQS`
  - `idpExpectedOU: npqs`
- FCAU deployment
  - `idpClientID: AGENCY_PORTAL_APP_FCAU`
  - `idpExpectedOU: fcau`
- CDA deployment
  - `idpClientID: AGENCY_PORTAL_APP_CDA`
  - `idpExpectedOU: cda`
- SLPA deployment
  - `idpClientID: OGA_PORTAL_APP_SLPA`
  - `idpExpectedOU: slpa`

This allows IdP-level user access restriction per Agency app registration.

## Configuration

None of the fields above are actually read from the environment/`.env` at
runtime any more — they're the actual `web.runtime` keys in the backend's
`config.yaml`, served to the browser at `/config.js` as `window.__APP_CONFIG__`,
which `src/runtimeConfig.ts`'s `getEnv`/`getRequiredEnv` read (see
`backend/config.example.yaml`). `.env`/`.env.example` here only matter for
`vite.config.ts`'s own dev-server settings (`VITE_PORT`, and `VITE_API_BASE_URL`
as the `/config.js` proxy target) — see the repo-root README's "Running a
specific NSW Agency" section and `start-dev.sh`.

Branding (logo, favicon, portal name, description, hero image, partner logos) is
served the same way, as `window.__APP_CONFIG__.branding`, from the backend's `config.yaml`
`web.branding` section — not a separate `/configs/<name>.branding.json` fetch. `src/config.ts`'s
`initAppConfig()` reads it synchronously and validates it against a Zod schema
before the app renders, falling back to a hardcoded emergency config if it's
missing or invalid — and filling in `portalName`/`description` from that same
emergency default if a deployment's `web.branding` sets only the required
`systemName`/`appName`.

### Adding a new Agency instance

Add a `web.branding` section (`systemName` and `appName` are required; the rest
are optional) to that agency's `backend/config/<agency>/config.yaml` — see
`backend/config.example.yaml` for the full schema, and any existing
`backend/config/<agency>/config.yaml` for a worked example.

## Local development

The frontend has no local runtime-config fallback: it loads `/config.js` from
a real backend via the Vite dev server's proxy (see `vite.config.ts`), so a
bare `pnpm dev` here needs a backend already running and reachable at
`VITE_API_BASE_URL` (see `.env.example`; defaults to `http://localhost:8081`).
Prefer the repo-root [../start-dev.sh](../start-dev.sh), which starts both
together — see "Running a specific NSW Agency" below.

```bash
pnpm install
pnpm run dev
```

### Running a specific NSW Agency

Use the repo-root [../start-dev.sh](../start-dev.sh) to start the frontend (and optionally the backend) with the per-agency port and API URL:

```bash
# From the repo root
./start-dev.sh npqs frontend     # NPQS frontend on port 5174
./start-dev.sh fcau frontend     # FCAU frontend on port 5175
./start-dev.sh cda  frontend     # CDA  frontend on port 5176
./start-dev.sh slpa frontend     # SLPA frontend on port 5177
./start-dev.sh npqs              # also start the matching backend
```

Each name maps to a `backend/config/<name>/config.yaml` (see that file's `web.branding` section for its actual branding). To onboard a new agency, add a new `backend/config/<name>/config.yaml` (see `backend/config.example.yaml`) and a matching line to `start-dev.sh`'s `CONFIG_*` table.
