# Agency App

## Authentication configuration

This app uses Asgardeo/Thunder OIDC for sign-in.

Required environment variables:

- `VITE_API_BASE_URL`: Agency backend API base URL (for example `http://localhost:8081`)
- `VITE_IDP_BASE_URL`: IdP base URL (for example `https://localhost:8090`)
- `VITE_IDP_CLIENT_ID`: NSW Agency-specific IdP application client id
- `VITE_IDP_EXPECTED_OU_HANDLE`: Required organization/OU handle for access restriction (e.g., `npqs`, `fcau`, `cda`, `slpa`)
- `VITE_APP_URL`: public URL of this Agency deployment
- `VITE_IDP_SCOPES` (optional): comma-separated scopes (defaults to `openid,profile,email,ou,role,agency:application:read,agency:application:review,agency:application:feedback,agency:consignment:read,agency:storage:read,agency:storage:write`)
- `VITE_IDP_EXTRA_QUERY_PARAMS`: extra `/authorize` parameters, query-string encoded (for example `resource=https://api.nsw-agency.local`). ThunderID requires an RFC 8707 `resource` indicator naming the AGENCY_API resource server; without it the `agency:*` scopes are dropped from the issued token. Optional only for an IdP that binds tokens by scope alone.

## Per-NSW Agency deployment model

Each Agency deployment should use its own IdP application configuration.

Example:

- NPQS deployment
  - `VITE_IDP_CLIENT_ID=AGENCY_PORTAL_APP_NPQS`
  - `VITE_IDP_EXPECTED_OU_HANDLE=npqs`
- FCAU deployment
  - `VITE_IDP_CLIENT_ID=AGENCY_PORTAL_APP_FCAU`
  - `VITE_IDP_EXPECTED_OU_HANDLE=fcau`
- CDA deployment
  - `VITE_IDP_CLIENT_ID=AGENCY_PORTAL_APP_CDA`
  - `VITE_IDP_EXPECTED_OU_HANDLE=cda`
- SLPA deployment
  - `VITE_IDP_CLIENT_ID=OGA_PORTAL_APP_SLPA`
  - `VITE_IDP_EXPECTED_OU_HANDLE=slpa`

This allows IdP-level user access restriction per Agency app registration.

## Configuration

None of the `VITE_*` variables above are actually read from the environment/`.env`
at runtime any more — they're the *names* the backend's `config.yaml` (`web.runtime`)
serves to the browser at `/config.js` as `window.__APP_CONFIG__`, which `src/runtimeConfig.ts`'s
`getEnv`/`getRequiredEnv` read (see `backend/config.example.yaml`). `.env`/`.env.example`
here only matter for `vite.config.ts`'s own dev-server settings (`VITE_PORT`, and
`VITE_API_BASE_URL` as the `/config.js` proxy target) — see the repo-root README's
"Running a specific NSW Agency" section and `start-dev.sh`.

Branding (logo, favicon, portal name, description, hero image, partner logos) is
served the same way, as `window.__APP_BRANDING__`, from the backend's `config.yaml`
`web.branding` section — not a separate `/configs/<name>.branding.json` fetch. `src/config.ts`'s
`initAppConfig()` reads it synchronously and validates it against a Zod schema
before the app renders, falling back to a hardcoded emergency config if it's
missing or invalid.

### Adding a new Agency instance

Add a `web.branding` section (`systemName` and `appName` are required; the rest
are optional) to that agency's `backend/config/<agency>/config.yaml` — see
`backend/config.example.yaml` for the full schema, and any existing
`backend/config/<agency>/config.yaml` for a worked example.

## Local development

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
