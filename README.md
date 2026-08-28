# NSW Agency

An additional system provided along with the NSW (National Single Window) platform to enable private or government agencies to review and approve trader-submitted data as part of the NSW workflow.

This repo contains both two components of the Agency system:

- [backend/](backend/) — Go service that holds agency side application state, talks to the NSW core backend over OAuth2 M2M, and serves the frontend.
- [frontend/](frontend/) — React/Vite SPA used by agency officers to review submissions from the NSW core backend.

The same codebase is deployed per agency, with branding and identity selected via env vars at runtime.

## Quick start

You need the Identity Provider(IdP) running first in order to login in Agency Portal. This live in the [NSW Core](https://github.com/OpenNSW/nsw):

```bash
# In the NSW Core
cd idp && docker compose up -d
```

Then in this repo:

```bash
# Backend
cd backend
cp .env.example .env       # tweak NSW_* to point at your NSW backend + IdP
go run ./cmd/server

# Frontend (new terminal)
cd frontend
cp .env.example .env       # set VITE_IDP_CLIENT_ID, VITE_API_BASE_URL
pnpm install
pnpm dev
```

### Running a specific NSW Agency

Use [start-dev.sh](start-dev.sh) at the repo root to launch the per-agency backend and/or frontend with the right ports, DB file, IdP client id, and branding:

```bash
./start-dev.sh npqs              # backend + frontend for NPQS
./start-dev.sh fcau backend      # only backend for FCAU
./start-dev.sh ird frontend      # only frontend for IRD
./start-dev.sh cda               # backend + frontend for CDA
./start-dev.sh default           # generic branding/ports

# Fleet mode: bring up every agency at once
./start-dev.sh all               # all 4 backends (8081-8084) + frontends (5174-5177)
./start-dev.sh all backend       # only the backends
./start-dev.sh all frontend      # only the frontends
```

Every process runs in its own process group (`set -m`), so `Ctrl-C` cleanly stops the whole fleet — including the compiled binary `go run` spawns underneath. Logs from all processes interleave on the same terminal.

| Agency | Backend port | DB file                        | NSW M2M client | Frontend port | Branding config                              | IdP client id            |
| ---------- | ------------ | ------------------------------ | -------------- | ------------- | -------------------------------------------- | ------------------------ |
| NPQS       | 8081         | `backend/npqs_applications.db` | `NPQS_TO_NSW`  | 5174          | `frontend/public/configs/npqs.branding.json` | `OGA_PORTAL_APP_NPQS` |
| FCAU       | 8082         | `backend/fcau_applications.db` | `FCAU_TO_NSW`  | 5175          | `frontend/public/configs/fcau.branding.json` | `OGA_PORTAL_APP_FCAU` |
| CDA        | 8083         | `backend/cda_applications.db`  | `CDA_TO_NSW`   | 5176          | `frontend/public/configs/cda.branding.json`  | `OGA_PORTAL_APP_CDA`  |
| SLPA       | 8084         | `backend/slpa_applications.db` | `SLPA_TO_NSW`  | 5177          | `frontend/public/configs/slpa.branding.json` | `OGA_PORTAL_APP_SLPA` |

The branding-config paths above are gitignored — copy them from [default.branding.json](frontend/public/configs/default.branding.json) (see below).

The script sets `PORT`, `DB_PATH`, `NSW_CLIENT_ID` for the backend and `VITE_PORT`, `VITE_BRANDING_NAME`, `VITE_API_BASE_URL`, `VITE_IDP_CLIENT_ID`, `VITE_APP_URL` for the frontend. Any of these can be overridden by exporting them before invoking the script (other backend/frontend env vars — OAuth secrets, IdP base URL, etc. — still come from `backend/.env` and `frontend/.env`).

Only [default.branding.json](frontend/public/configs/default.branding.json) is tracked in git — per-NSW Agency branding files (`npqs.branding.json`, `fcau.branding.json`, `cda.branding.json`, `slpa.branding.json`) are gitignored because branding is deployment-specific. To run a Agency locally, copy the default and edit it:

```bash
cd frontend/public/configs
cp default.branding.json npqs.branding.json   # then edit appName, portalName, description, etc.
```

To add a brand-new NSW Agency, create `<name>.branding.json` the same way and add a matching `case` to [start-dev.sh](start-dev.sh).

## Prerequisites

### 1. NSW backend reachable

NSW Agency calls the NSW core backend's `/api/v1/tasks` endpoint to return review results. Set `NSW_API_BASE_URL` in [backend/.env](backend/.env) accordingly (default: `http://localhost:8080`).

### 2. M2M OAuth2 client

Each Agency instance authenticates to NSW with its own M2M client. For local dev the IdP bootstrap creates a generic `AGENCY_TO_NSW` client; production deployments use agency-specific clients (`NPQS_TO_NSW`, etc.).

## Architecture

NSW Agency is decoupled from the NSW core monorepo — it communicates over HTTP only:

```
trader-app → nsw-backend → (POST /api/v1/inject) → NSW Agency-backend ← NSW Agency-app
                  ▲                                       │
                  └────── (POST /api/v1/tasks, OAuth2 M2M)┘
```

- Own database (SQLite or PostgreSQL, per `DB_*` env vars) — not shared with NSW.
- Templates fetched from [OpenNSW/one-trade-templates](https://github.com/OpenNSW/one-trade-templates) at startup.
- No Temporal integration — Agency is a stateless HTTP microservice.

For details see [backend/docs/architecture.md](backend/docs/architecture.md).

## Releases

Tagging `vX.Y.Z` triggers [.github/workflows/release.yml](.github/workflows/release.yml), which builds and publishes the single consolidated image (the Go server serves both the API and the officer-portal SPA from one process):

- `ghcr.io/opennsw/agency:X.Y.Z`

The image is a multi-arch manifest list (`linux/amd64` and `linux/arm64`), so one
tag resolves per node architecture. Its manifest-list digest is included in the
GitHub Release notes.
