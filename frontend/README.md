# Agency App

## Authentication configuration

This app uses Asgardeo/Thunder OIDC for sign-in.

The frontend no longer reads its own runtime config from env vars/`.env` —
it fetches `/config.js` from the paired backend and reads
`window.__APP_CONFIG__` (see [src/runtimeConfig.ts](src/runtimeConfig.ts)).
Only `VITE_PORT` and `VITE_API_BASE_URL` remain as `.env` vars, and only for
`vite.config.ts` itself (Node-side, never bundled into the app) — see
[.env.example](.env.example). Everything below is instead configured per
agency in the backend's `web.runtime` section (see
[backend/config.example.yaml](../backend/config.example.yaml) for the full
schema):

- `VITE_BRANDING_NAME` (`web.runtime.brandingName`): name of Agency branding configuration (e.g. `npqs`, `fcau`, `cda`, `slpa`, or `default`)
- `VITE_API_BASE_URL` (`web.runtime.apiBaseURL`): Agency backend API base URL (for example `http://localhost:8081`)
- `VITE_IDP_BASE_URL` (`web.runtime.idpBaseURL`): IdP base URL (for example `https://localhost:8090`)
- `VITE_IDP_CLIENT_ID` (`web.runtime.idpClientID`): NSW Agency-specific IdP application client id
- `VITE_IDP_EXPECTED_OU_HANDLE` (`web.runtime.idpExpectedOU`): Required organization/OU handle for access restriction (e.g., `npqs`, `fcau`, `cda`, `slpa`)
- `VITE_APP_URL` (`web.runtime.appURL`): public URL of this Agency deployment
- `VITE_IDP_SCOPES` (`web.runtime.idpScopes`, optional): comma-separated scopes (defaults to `openid,profile,email,ou,role,agency:application:read,agency:application:review,agency:application:feedback,agency:consignment:read,agency:storage:read,agency:storage:write`)
- `VITE_IDP_EXTRA_QUERY_PARAMS` (`web.runtime.idpExtraQueryParams`): extra `/authorize` parameters, query-string encoded (for example `resource=https://api.nsw-agency.local`). ThunderID requires an RFC 8707 `resource` indicator naming the AGENCY_API resource server; without it the `agency:*` scopes are dropped from the issued token. Optional only for an IdP that binds tokens by scope alone.

## Per-NSW Agency deployment model

Each Agency deployment should use its own IdP application configuration, set
in that agency's `backend/config/<agency>/config.yaml`.

Example:

- NPQS deployment
  - `brandingName: npqs`
  - `idpClientID: AGENCY_PORTAL_APP_NPQS`
  - `idpExpectedOU: npqs`
- FCAU deployment
  - `brandingName: fcau`
  - `idpClientID: AGENCY_PORTAL_APP_FCAU`
  - `idpExpectedOU: fcau`
- CDA deployment
  - `brandingName: cda`
  - `idpClientID: AGENCY_PORTAL_APP_CDA`
  - `idpExpectedOU: cda`
- SLPA deployment
  - `brandingName: slpa`
  - `idpClientID: OGA_PORTAL_APP_SLPA`
  - `idpExpectedOU: slpa`

This allows IdP-level user access restriction per Agency app registration.

## Configuration

NSW Agency instance branding is defined via JSON configuration files loaded dynamically at runtime.

### How it works

1. The frontend fetches the branding configuration file matching the name specified in `VITE_BRANDING_NAME` from `/configs/${VITE_BRANDING_NAME}.branding.json` (e.g., `/configs/npqs.branding.json`).
2. If `VITE_BRANDING_NAME` is not set, it defaults to `default`, requesting `/configs/default.branding.json`.
3. If the configured branding file fails to load, the app automatically falls back to fetching the default configuration `/configs/default.branding.json`.
4. If all fetches fail, a hardcoded emergency fallback config is loaded to keep the portal functional.
5. The retrieved configuration is validated against a Zod schema before the application renders.

### Adding a new Agency instance

1. Create a new JSON file under `public/configs/<name>.branding.json` (e.g., `public/configs/custom.branding.json`).
2. Edit the `branding.systemName` and `branding.appName` fields (required).
3. Set that agency's `backend/config/<agency>/config.yaml` `web.runtime.brandingName` to your custom name (e.g., `brandingName: custom`) — see [Authentication configuration](#authentication-configuration) above.

### Config schema

```json
{
  "branding": {
    "systemName": "NSW",
    "appName": "NSW Agency Officer Portal",
    "logoUrl": "",
    "systemLogoUrl": "",
    "favicon": "",
    "portalName": "NSW Agency Portal",
    "description": "A unified digital platform..."
  }
}
```

## Local development

```bash
pnpm install
pnpm run dev
```

### Running a specific NSW Agency

Use the repo-root [../start-dev.sh](../start-dev.sh) to start the frontend (and optionally the backend) with the per-agency port, branding name, API URL, and IdP client id:

```bash
# From the repo root
./start-dev.sh npqs frontend     # NPQS frontend on port 5174
./start-dev.sh fcau frontend     # FCAU frontend on port 5175
./start-dev.sh cda  frontend     # CDA  frontend on port 5176
./start-dev.sh slpa frontend     # SLPA frontend on port 5177
./start-dev.sh npqs              # also start the matching backend
```

Each name maps to a JSON file under [public/configs/](public/configs/) (`<name>.branding.json`). To onboard a new agency, copy [public/configs/default.branding.json](public/configs/default.branding.json), edit the `branding.*` fields, and add a new `case` to [../start-dev.sh](../start-dev.sh).
