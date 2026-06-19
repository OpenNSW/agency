# Contributing to NSW Agency

Contributions are welcome. For a detailed guide on setting up your local environment, creating feature branches, running unit tests, and validating builds, see the [Development Guide](contributing/development.md).

## Project Overview

The `nsw-agency` repository contains the pluggable portals system that enables government or private agencies to review and approve trader submissions:
- **`backend/`**: Go application server that holds the agency-side application state, talks to the NSW core backend via OAuth2 M2M APIs, and implements SQLite/Postgres persistence.
- **`frontend/`**: React/Vite Single Page Application (SPA) for agency officers.

## Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix | When to use |
|---|---|
| `feat:` | New feature |
| `fix:` | Bug fix |
| `refactor:` | Code change that is not a feature or fix |
| `docs:` | Documentation only |
| `build:` | Build system / dependency changes |
| `ci:` | CI/CD changes |
| `chore:` | Maintenance (tooling, config, etc.) |
| `test:` | Test-only changes |

Use `scope` in parentheses for context: `feat(backend): add Postgres support` or `fix(frontend): adjust layout padding`.
Use `!` after the type for breaking changes: `feat!(auth): rename ExpectedOU field`.

## Reporting Issues

If you find a bug or have a feature request, please follow our [Reporting Issues Guide](contributing/reporting-issues.md).
