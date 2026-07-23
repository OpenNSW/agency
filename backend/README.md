# Agency Portal Backend

A standalone Go microservice that acts as a verification hub for external agencies within the [NSW (National Single Window)](../README.md) trade facilitation platform. It enables the NSW core service to inject data for review, supports configurable dynamic forms per agency, and sends callback responses to the originating service upon review completion.

## How It Fits Into NSW

The Agency service is an implementation of the **Agency Service Module (Agency SM)** described in the NSW architecture. It embodies the "state vs. data decoupling" principle:

- **NSW Core Workflow Engine (CWE)** manages process state (e.g., "Waiting for Approval")
- **NSW Agency Service Module** manages domain data (e.g., phytosanitary inspection details)

Each agency runs its own instance with its own database, ensuring data isolation and sovereignty.

```
┌─────────────────┐         POST /api/v1/inject           ┌──────────────────┐
│                 │ ──────────────────────────────────────▶│                  │
│  NSW Core       │                                        │   Agency Service    │
│    Service      │◀────────────────────────────────────── │   (per agency)   │
│                 │     POST {serviceUrl} (callback)       │                  │
└─────────────────┘                                        └──────────────────┘
                                                                   │
                                                            ┌──────┴──────┐
                                                            │  DB Store   │
                                                            │ (SQLite/PG) │
                                                            └─────────────┘
```

## Features

- **Data Injection** – External services POST data for Agency review via `/api/v1/inject`
- **Task Configurations** – Per-taskCode metadata (title, icon, category), form references, and outcome-to-status mapping
- **Dynamic Forms** – Reusable [JSON Forms](https://jsonforms.io/) definitions referenced by ID from task configs
- **Paginated Listings** – Fetch applications with status filtering and pagination
- **Review Workflow** – Approve/Reject driven by configurable status maps
- **Callback Responses** – Automatically POSTs review results back to the originating service
- **Per-agency Isolation** – Each Agency instance has its own database and port
- **Graceful Shutdown** -- Signal-based shutdown with in-flight request draining

## Getting Started

### Prerequisites

- Go 1.26+
- GCC (required by `go-sqlite3` CGO dependency)

### Run Locally

The server needs an artifact source. For local development point `ARTIFACT_LOCAL_ROOT`
at a directory containing a `manifest.json` and the artifacts it catalogs (e.g. a
checkout of the agency's artifacts repo). `start-dev.sh` sets this per agency for you.

```bash
cd backend

# Run against a local artifacts checkout (port 8081, SQLite at ./agency_applications.db)
ARTIFACT_LOCAL_ROOT=../../one-trade-artifacts/npqs go run ./cmd/server

# Run with custom config (SQLite)
PORT=8081 DB_PATH=./npqs.db ARTIFACT_LOCAL_ROOT=../../one-trade-artifacts/npqs go run ./cmd/server

# Run with custom config (PostgreSQL)
DB_DRIVER=postgres DB_NAME=npqs_db DB_USER=postgres DB_PASSWORD=changeme \
  ARTIFACT_LOCAL_ROOT=../../one-trade-artifacts/npqs go run ./cmd/server
```

The database is auto-created and auto-migrated on first startup.

### Build

```bash
go build -o bin/agency ./cmd/server
./bin/agency
```

### Running Multiple Agency Instances

Each Agency should run as a separate instance:

```bash
# Terminal 1 -- NPQS (National Plant Quarantine Service)
PORT=8081 DB_PATH=./npqs_applications.db ARTIFACT_LOCAL_ROOT=../../one-trade-artifacts/npqs go run ./cmd/server

# Terminal 2 -- FCAU (Food Control Administration Unit)
PORT=8082 DB_PATH=./fcau_applications.db ARTIFACT_LOCAL_ROOT=../../one-trade-artifacts/fcau go run ./cmd/server
```

In practice, use [`../start-dev.sh <agency>`](../start-dev.sh), which sets the port, DB path, and per-agency `ARTIFACT_LOCAL_ROOT` automatically.

### Configuration

All configuration is via environment variables:

| Variable                         | Description                                                                           | Default                        |
|----------------------------------|---------------------------------------------------------------------------------------|--------------------------------|
| `PORT`                           | HTTP server port                                                                      | `8081`                         |
| `DB_DRIVER`                      | Database driver (`sqlite`, `postgres`)                                                | `sqlite`                       |
| `DB_PATH`                        | Path to SQLite database file                                                          | `./agency_applications.db`     |
| `DB_HOST`                        | PostgreSQL host                                                                       | `localhost`                    |
| `DB_PORT`                        | PostgreSQL port                                                                       | `5432`                         |
| `DB_USER`                        | PostgreSQL user                                                                       | `postgres`                     |
| `DB_PASSWORD`                    | PostgreSQL password                                                                   | `changeme`                     |
| `DB_NAME`                        | PostgreSQL database name                                                              | `agency_db`                    |
| `DB_SSLMODE`                     | PostgreSQL SSL mode                                                                   | `require`                      |
| `ARTIFACT_LOADER_TYPE`           | Artifact source backend: `local`, `github`, or `s3`                                   | `local`                        |
| `ARTIFACT_LOCAL_ROOT`            | Root dir the `local` loader resolves task configs, forms, and `manifest.json` against | required for `local`           |
| `ALLOWED_ORIGINS`                | Comma-separated CORS origins (`*` to allow all)                                       | `*`                            |
| `NSW_API_BASE_URL`               | NSW API base URL for calling NSW endpoints                                            | `http://localhost:8080/api/v1` |
| `NSW_CLIENT_ID`                  | OAuth2 client ID for Agency -> NSW                                                    | required                       |
| `NSW_CLIENT_SECRET`              | OAuth2 client secret for Agency -> NSW                                                | required                       |
| `NSW_TOKEN_URL`                  | OAuth2 token endpoint URL                                                             | required                       |
| `NSW_SCOPES`                     | Optional comma-separated OAuth2 scopes                                                | empty                          |
| `NSW_TOKEN_INSECURE_SKIP_VERIFY` | DEV-only: skip TLS verification for token fetch                                       | `false`                        |

Task configs and forms are loaded through the [`core/artifact`](https://github.com/OpenNSW/core/tree/main/artifact) registry rather than from the repo. A single loader (`local`, `github`, or `s3`) fetches a `manifest.json` and the artifacts it catalogs from one source. The `github`/`s3` backends have their own `ARTIFACT_GITHUB_*` / `ARTIFACT_S3_*` variables — see [`.env.example`](.env.example) for the full set.

## API Reference

See [docs/api.md](docs/api.md) for complete API documentation with request/response examples.

**Quick overview:**

| Method | Endpoint                               | Description                                |
|--------|----------------------------------------|--------------------------------------------|
| `GET`  | `/health`                              | Health check                               |
| `POST` | `/api/v1/inject`                       | Inject data for review (called by NSW)     |
| `GET`  | `/api/v1/applications`                 | List applications (paginated, filterable)  |
| `GET`  | `/api/v1/applications/{taskId}`        | Get single application with review form    |
| `POST` | `/api/v1/applications/{taskId}/review` | Submit review decision (triggers callback) |

## Documentation

Detailed documentation lives in the [`docs/`](docs/) folder:

| Document                                    | Description                                                                                |
|---------------------------------------------|--------------------------------------------------------------------------------------------|
| [Architecture](docs/architecture.md)        | System design, layered architecture, data flow                                             |
| [API Reference](docs/api.md)                | Complete endpoint docs with examples                                                       |
| [Task Configurations](docs/task-configs.md) | Per-taskCode metadata, form references, and status-mapping behavior; how to add a new task |
| [Forms](docs/forms.md)                      | JSON Forms file structure and how to add new forms referenced from task configs            |
| [NSW Integration](docs/nsw-integration.md)  | How Agency connects to the NSW workflow engine                                             |

## Project Structure

```
backend/
├── cmd/server/
│   └── main.go                 # Entry point, server setup, graceful shutdown
├── internal/
│   ├── config.go               # Environment-based configuration
│   ├── handler.go              # HTTP handlers for all endpoints
│   ├── service.go              # Business logic, callback dispatch
│   ├── store.go                # GORM-based application repository
│   ├── task_config.go          # TaskConfigStore -- per-taskCode UI metadata and form refs
│   ├── form.go                 # FormStore -- pure JSON Forms definitions
│   ├── utils.go                # JSON response helpers
│   ├── database/               # Driver setup and connection (SQLite + PostgreSQL)
│   ├── feedback/               # Trader feedback endpoint
│   └── storage/                # Upload/download URL handling for file attachments
├── pkg/
│   ├── httpclient/             # OAuth2-aware HTTP client used for outbound NSW calls
│   └── httputil/               # Shared HTTP helpers
├── data/                       # Seed data (users/roles). Task configs and forms are
│                               # NOT here — they load via the artifact loader from an
│                               # external source (local dir, GitHub, or S3).
├── docs/                       # Documentation
├── Dockerfile
├── workload.yaml
├── .env.example                # Example environment configuration
├── go.mod
└── go.sum
```

## Contributing

Please read the project-level [CONTRIBUTING.md](../docs/CONTRIBUTING.md) before submitting changes.

When working on the Agency module:

1. All application code lives in `internal/` (unexported package)
2. Task configs and form definitions live outside this repo and load through the artifact loader (a `manifest.json` plus the artifacts it catalogs). See [`docs/task-configs.md`](docs/task-configs.md) and [`docs/forms.md`](docs/forms.md).
3. The service uses Go's standard `net/http` with `http.ServeMux` -- no external routing frameworks
4. Database migrations are handled automatically by GORM's `AutoMigrate`
5. Run `go vet ./...` and `go build ./...` before submitting PRs

## License

Distributed under the Apache 2.0 License. See [LICENSE](../LICENSE) for more information.
