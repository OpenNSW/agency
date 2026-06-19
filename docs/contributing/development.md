# Development Guide

This guide will help you set up your local development environment for contributing to NSW Agency.

## Prerequisites

Before you begin, ensure you have the following installed:

-   [Go](https://go.dev/dl/) (1.24 or later)
-   [Node.js](https://nodejs.org/) (20 or later, with pnpm)
-   [GCC](https://gcc.gnu.org/) (required by `go-sqlite3` CGO dependency)
-   [Docker](https://www.docker.com/products/docker-desktop/) and Docker Compose
-   [Git](https://git-scm.com/downloads)

## Initial Setup

1.  **Fork and clone the repository:**
    ```bash
    git clone https://github.com/YOUR_USERNAME/nsw-agency.git
    cd nsw-agency
    ```

2.  **Add the upstream remote:**
    ```bash
    git remote add upstream https://github.com/OpenNSW/nsw-agency.git
    ```

3.  **Bootstrap Environment Variables:**
    Copy the example configurations for the backend and frontend:
    ```bash
    cp backend/.env.example backend/.env
    cp frontend/.env.example frontend/.env
    ```

4.  **Install Frontend Dependencies:**
    ```bash
    cd frontend
    pnpm install
    ```

## Development Workflow

### Running Locally

First, ensure the core NSW backing services are running (via `nsw-srilanka` compose setup), then start the agency portals:

```bash
# Start all configured agency portals (NPQS, FCAU, CDA, SLPA)
./start-dev.sh all

# Wipe databases and start fresh (clean run)
./start-dev.sh all --clean-run
```

### Running Tests

**Backend Unit Tests:**
```bash
cd backend
go test ./...
```

**Frontend Quality Checks:**
```bash
cd frontend
pnpm lint
pnpm typecheck
```

**Build Validation:**
```bash
# Build the Go backend binary
cd backend
go build ./cmd/server

# Build the React frontend production bundle
cd frontend
pnpm build
```

## Code Style and Standards

### Backend Go Code Style

-   Follow standard Go idioms and conventions.
-   Run format checks using `go fmt ./...` before committing.
-   Run `go vet ./...` to check for common mistakes.

### Frontend React Style

-   Follow TSX/React best practices.
-   Run ESLint formatting before staging files: `pnpm lint`.

### Commit Messages

Write clear, descriptive commit messages:

-   Use the imperative mood ("Add feature" not "Added feature")
-   Keep the first line under 50 characters
-   Add a blank line and detailed explanation if needed
-   We follow [Conventional Commits](https://www.conventionalcommits.org/) (e.g., `feat(backend): add Postgres support` or `fix(frontend): adjust layout padding`).

### Code Review Checklist

Before submitting a pull request, ensure:

-   [ ] Code follows project style guidelines.
-   [ ] All tests pass locally.
-   [ ] New code includes appropriate tests.
-   [ ] Documentation is updated if needed.
-   [ ] Commit messages are clear and descriptive.
-   [ ] No merge conflicts with `main` branch.

## Project Structure

```
nsw-agency/
├── backend/             # Go backend application
│   ├── cmd/             # Server and database migration entry points
│   ├── internal/        # Backend business logic, handlers, and stores
│   ├── data/            # Local configuration templates, seeds, and schemas
│   └── migrations/      # SQLite/PostgreSQL schema migrations
├── frontend/            # React/Vite Single Page Application (SPA)
│   ├── src/             # SPA application source code
│   └── public/          # Public assets & branding configurations
└── docs/                # Project documentation
```

## Getting Help

-   Check existing [Issues](https://github.com/OpenNSW/nsw-agency/issues)
-   See [Reporting Issues](reporting-issues.md) for how to submit bug reports and feature requests
-   Review the main [CONTRIBUTING.md](../CONTRIBUTING.md)
