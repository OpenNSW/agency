# Architecture

This document describes the internal architecture of the Agency service module.

## Layered Architecture

The service follows a standard three-layer architecture with a clear separation of concerns:

```
┌────────────────────────────────────────────────────┐
│                   main.go                          │
│          (server setup, routing, shutdown)         │
└───────────────────────┬────────────────────────────┘
                        │
┌───────────────────────▼────────────────────────────┐
│                  Handler Layer                     │
│           handler.go + utils.go                    │
│     (HTTP request parsing, response formatting)    │
└───────────────────────┬────────────────────────────┘
                        │
┌───────────────────────▼────────────────────────────┐
│                  Service Layer                     │
│                  service.go                        │
│  (validation, business logic, callback dispatch)   │
└──────────┬────────────────────────────┬────────────┘
           │                            │
┌──────────▼──────────┐    ┌────────────▼────────────┐
│    Store Layer      │    │     Form Store          │
│     store.go        │    │      form.go            │
│  (GORM + SQLite)    │    │ (in-memory JSON cache)  │
└─────────────────────┘    └─────────────────────────┘
```

### Handler Layer (`handler.go`, `utils.go`)

Responsible for:
- HTTP method validation
- Request body parsing and path parameter extraction
- Delegating to the service layer
- Writing JSON responses and error responses

The handler knows nothing about the database or business rules. It translates HTTP concerns into service calls.

### Service Layer (`service.go`)

Responsible for:
- Input validation (required fields, value constraints)
- Coordinating between the store and form store
- Attaching the correct review form to application responses
- Dispatching HTTP callbacks to the originating NSW service after a review
- Defining the `Service` interface for testability

Key design decisions:
- The service layer owns the HTTP client for callbacks (30-second timeout)
- Form attachment uses a fallback strategy: match by metadata, fall back to default form
- Pagination defaults (page 1, page size 20, max 100) are enforced here

### Store Layer (`store.go`)

Responsible for:
- Database connection management (SQLite via GORM)
- CRUD operations on `ApplicationRecord`
- Custom `JSONB` type that serializes `map[string]any` as JSON text in SQLite

The store opens a connection but never changes the schema. Schema comes from the
SQL files in [`migrations/`](../migrations), applied by `cmd/migrate` -- see
[Database Schema](#database-schema) below and [Migrations](./migrations.md) for the
file format, including driver-specific (Postgres/SQLite) SQL blocks.

### Form Store (`form.go`)

Responsible for:
- Loading all `.json` files from the forms directory into memory at startup
- Serving form definitions by ID
- Constructing form IDs from application metadata (`type:verificationId`)

## Dependency Flow

Dependencies are injected top-down via constructors:

```go
config, err := LoadConfig()
// handle error...
store     := NewApplicationStore(config)
formStore := NewFormStore(config.FormsPath, config.DefaultFormID)
service   := NewService(config, store, formStore)
handler   := NewHandler(service, config.NSWAPIBaseURL)
```

The `Service` interface allows the handler to depend on an abstraction rather than a concrete implementation, enabling mock-based testing.

## Database Schema

Single table: `applications`

| Column              | Type         | Description                                        |
|---------------------|--------------|----------------------------------------------------|
| `task_id`           | TEXT         | Primary key (provided by NSW workflow)             |
| `consignment_id`    | TEXT         | Related consignment identifier                     |
| `data`              | TEXT (JSON)  | Trader-submitted data                              |
| `meta`              | TEXT (JSON)  | Form selection metadata (`type`, `verificationId`) |
| `reviewer_response` | TEXT (JSON)  | Complete reviewer response payload                 |
| `status`            | VARCHAR(50)  | `PENDING`, `APPROVED`, or `REJECTED`               |
| `reviewed_at`       | DATETIME     | Timestamp of review completion                     |
| `created_at`        | DATETIME     | Record creation time                               |
| `updated_at`        | DATETIME     | Last modification time                             |

JSON columns use a custom `JSONB` type that implements Go's `driver.Valuer` and `sql.Scanner` interfaces to serialize `map[string]any` as JSON text in SQLite.

## Request Lifecycle

### Inject (data coming in from NSW)

```
NSW Workflow ──POST──▶ HandleInjectData ──▶ CreateApplication
                                                │
                                          validate fields
                                          convert Meta to JSONB
                                          store with PENDING status
                                                │
                                          ◀── 201 Created
```

### Review (officer submitting a decision)

```
Officer UI ──POST──▶ HandleReviewApplication ──▶ ReviewApplication
                                                      │
                                                 validate decision field
                                                 update status in DB
                                                 build TaskResponse payload
                                                 POST callback to /api/v1/tasks/{id}
                                                      │
                                                ◀── 200 OK
                                                      │
                                                      ▼
                                               NSW Workflow Engine
                                          (receives AGENCY_VERIFICATION action)
```

## Concurrency and Shutdown

The server uses Go's standard `net/http` server with signal-based graceful shutdown:

1. `SIGINT` or `SIGTERM` received
2. `server.Shutdown()` called with 30-second timeout
3. In-flight requests complete
4. Database connection closed
5. Process exits

## CORS

A CORS middleware wraps all routes, allowing `GET`, `POST`, `PUT`, `DELETE`, and `OPTIONS` methods. Allowed origins are configured via the `ALLOWED_ORIGINS` environment variable (space-separated). Defaults to `*` for development; production deployments should set explicit origins (e.g. `ALLOWED_ORIGINS="https://app.example.com https://admin.example.com"`).