# Agency App

## Authentication configuration

This app uses Asgardeo/Thunder OIDC for sign-in.

Required environment variables:

- `VITE_BRANDING_NAME`: Name of Agency branding configuration (e.g. `npqs`, `fcau`, `cda`, `slpa`, or `default`)
- `VITE_API_BASE_URL`: Agency backend API base URL (for example `http://localhost:8081`)
- `VITE_IDP_BASE_URL`: IdP base URL (for example `https://localhost:8090`)
- `VITE_IDP_CLIENT_ID`: NSW Agency-specific IdP application client id
- `VITE_IDP_EXPECTED_OU_HANDLE`: Required organization/OU handle for access restriction (e.g., `npqs`, `fcau`, `cda`, `slpa`)
- `VITE_APP_URL`: public URL of this Agency deployment
- `VITE_IDP_SCOPES` (optional): comma-separated scopes (defaults to `openid,profile,email,ou`)
- `VITE_IDP_EXTRA_QUERY_PARAMS`: extra `/authorize` parameters, query-string encoded (for example `resource=https://api.nsw-agency.local`). ThunderID requires an RFC 8707 `resource` indicator naming the AGENCY_API resource server; without it the `agency:*` scopes are dropped from the issued token. Optional only for an IdP that binds tokens by scope alone.

## Per-NSW Agency deployment model

Each Agency deployment should use its own IdP application configuration.

Example:

- NPQS deployment
  - `VITE_BRANDING_NAME=npqs`
  - `VITE_IDP_CLIENT_ID=AGENCY_PORTAL_APP_NPQS`
  - `VITE_IDP_EXPECTED_OU_HANDLE=npqs`
- FCAU deployment
  - `VITE_BRANDING_NAME=fcau`
  - `VITE_IDP_CLIENT_ID=AGENCY_PORTAL_APP_FCAU`
  - `VITE_IDP_EXPECTED_OU_HANDLE=fcau`
- CDA deployment
  - `VITE_BRANDING_NAME=cda`
  - `VITE_IDP_CLIENT_ID=AGENCY_PORTAL_APP_CDA`
  - `VITE_IDP_EXPECTED_OU_HANDLE=cda`
- SLPA deployment
  - `VITE_BRANDING_NAME=slpa`
  - `VITE_IDP_CLIENT_ID=OGA_PORTAL_APP_SLPA`
  - `VITE_IDP_EXPECTED_OU_HANDLE=slpa`

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
3. Update your environment configuration (or `start-dev.sh`) to set `VITE_BRANDING_NAME` to your custom name (e.g., `VITE_BRANDING_NAME=custom`).

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
