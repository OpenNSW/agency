#!/usr/bin/env bash
# Run Agency backends and/or frontends with per-agency config.
#
# Usage:
#   ./start-dev.sh [--clean-run] [--env-file=PATH] <agency> [target]
#
#   <agency>  One of: npqs, fcau, cda, slpa, all
#             'all' fans out and starts every agency in parallel.
#   [target]  One of: all (default), backend, frontend
#
# Flags:
#   --clean-run       Wipe agency database(s) before starting.
#                     SQLite: deletes each agency's db.sqlite.path file (from
#                     its config/<agency>/config.yaml — {agency}_applications.db
#                     by default).
#                     Postgres: drops and recreates the database.
#                     Migrations (go run ./cmd/migrate up) always run before
#                     the server starts, clean-run or not, so the schema is
#                     current either way.
#   --env-file=PATH   Load additional env vars (non-clobbering) before
#   --env-file PATH   per-agency defaults. Useful for sharing a root .env.
#
# Each agency maps to its own:
#   - backend HTTP port and SQLite DB file
#   - frontend dev server port
#   - frontend branding config (public/configs/<agency>.branding.json)
#   - IdP client id
#
# Each agency's backend settings (ports aside, see below) are NOT generated
# by this script — they're the checked-in backend/config/<agency>/config.yaml
# (see backend/config.example.yaml for the schema). Edit that file directly to
# change an agency's config; this script only picks which one to use
# (CONFIG_PATH) and supplies secrets/DB/CLI settings that stay plain env vars.
#
# Env-var precedence for what IS still env-var driven (DB_DRIVER, DB_PATH,
# NSW_CLIENT_SECRET, USER_CUSTOM_DATA_SCHEMA_PATH, CONFIG_PATH itself):
#   parent shell env > --env-file > backend/.env > script defaults
#
# Examples:
#   ./start-dev.sh npqs              # NPQS backend + frontend
#   ./start-dev.sh fcau backend      # FCAU backend only
#   ./start-dev.sh ird frontend      # IRD frontend only
#   ./start-dev.sh all               # every backend + frontend, in parallel
#   ./start-dev.sh all backend       # every backend, no frontends
#   ./start-dev.sh all --clean-run   # wipe all agency DBs, then start
#
# Ctrl-C terminates every child process (each runs in its own process group).

set -euo pipefail
# Enable job control so each backgrounded subshell becomes its own process
# group leader — that lets us kill `go run`'s grandchild binary on cleanup.
set -m

IDP_BASE_URL="https://localhost:8090" # For frontend Vite proxying to a local IdP instance; not used by backend.

# AGENCY_API resource-server identifier. Becomes the token's `aud` and is matched
# verbatim, so it must stay byte-identical to nsw-srilanka's resource-servers.json.
# Only used by the frontend now — the backend's audience is baked into its
# own config/<agency>/config.yaml (authn.audience/nsw.tokenParams.resource).
AGENCY_API_AUDIENCE="${AGENCY_API_AUDIENCE:-https://api.nsw-agency.local}"

# Orchestration-only per-agency config: "BE_PORT|FE_PORT|IDP_CLIENT_ID|APP_NAME|OU_HANDLE".
# Everything the backend itself needs (client ids, OU handle, NSW settings,
# ports, ...) lives in backend/config/<agency>/config.yaml instead — this
# table only feeds the frontend dev server and this script's own logging, and
# BE_PORT/FE_PORT here must match that agency's config.yaml (port/allowedOrigins).
# Adding an agency means one line here plus a new config/<agency>/config.yaml.
# (Scalar vars rather than `declare -A` so this works on stock macOS bash 3.2.)
CONFIG_npqs="8081|5174|OGA_PORTAL_APP_NPQS|National Plant Quarantine Service (NPQS)|npqs"
CONFIG_fcau="8082|5175|OGA_PORTAL_APP_FCAU|Food Control Administration Unit (FCAU)|fcau"
CONFIG_cda="8083|5176|OGA_PORTAL_APP_CDA|Coconut Development Authority (CDA)|cda"
CONFIG_slpa="8084|5177|OGA_PORTAL_APP_SLPA|Sri Lanka Ports Authority (SLPA)|slpa"
CONFIG_customs="8085|5178|OGA_PORTAL_APP_CUSTOMS|Sri Lanka Customs (CUSTOMS)|customs"
CONFIG_sltb="8086|5179|OGA_PORTAL_APP_SLTB|Sri Lanka Tea Board (SLTB)|sltb"


# Agencies (every CONFIG_* ), alphabetised for predictable launch order in 'all' mode
#  Derived from the config above so adding an agency only requires editing the CONFIG_* block.
ALL_AGENCIES=()
while IFS= read -r _v; do
  _agency="${_v#CONFIG_}"
  [[ "$_agency" == "default" ]] && continue
  ALL_AGENCIES+=("$_agency")
done < <(compgen -A variable CONFIG_ | sort)
unset _v _agency

usage() {
  cat <<EOF >&2
Usage: $0 [--clean-run] [--env-file=PATH] <agency> [target]

  <agency>  One of: ${ALL_AGENCIES[*]}, all
  [target]  One of: all (default), backend, frontend

Flags:
  --clean-run       Wipe agency DB(s) before starting (migrations always run)
  --env-file=PATH   Load a root-level env file (non-clobbering);
  --env-file PATH   both forms are supported

Examples:
  $0 npqs                       # NPQS backend + frontend
  $0 fcau backend               # FCAU backend only
  $0 all                        # every agency, backends + frontends
  $0 all frontend               # every agency, frontends only
  $0 all --clean-run            # wipe all agency DBs, then start
EOF
  exit 1
}

CLEAN_RUN=false
ENV_FILE=""
POSITIONAL=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --clean-run)
      CLEAN_RUN=true
      ;;
    --env-file=*)
      ENV_FILE="${1#*=}"
      ;;
    --env-file)
      shift
      if [[ $# -eq 0 ]] || [[ "$1" == --* ]]; then
        echo "[start-dev] Error: --env-file requires a path value." >&2
        usage
      fi
      ENV_FILE="$1"
      ;;
    *)
      POSITIONAL+=("$1")
      ;;
  esac
  shift
done

AGENCY="${POSITIONAL[0]:-}"
TARGET="${POSITIONAL[1]:-all}"

[[ -z "$AGENCY" ]] && usage

case "$TARGET" in
  all|backend|frontend) ;;
  *)
    echo "Unknown target '$TARGET'." >&2
    usage
    ;;
esac

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
FRONTEND_DIR="$ROOT_DIR/frontend"

PIDS=()

cleanup() {
  # Avoid recursion if the trap fires more than once.
  trap - EXIT INT TERM
  if (( ${#PIDS[@]} > 0 )); then
    echo
    echo "[start-dev] Stopping ${#PIDS[@]} process(es)..."
    for pid in "${PIDS[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then
        # Negative PID -> signal the whole process group (set -m makes each
        # background subshell its own pgroup leader with pgid == pid).
        kill -TERM "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
      fi
    done
    # Give processes a moment to shut down gracefully
    sleep 1
    # Force kill any remaining processes in the group
    for pid in "${PIDS[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then
        kill -KILL "-$pid" 2>/dev/null || kill -KILL "$pid" 2>/dev/null || true
      fi
    done
    wait 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

# Sets BE_PORT, FE_PORT, IDP_CLIENT_ID, APP_NAME, OU_HANDLE for the given
# agency (orchestration/frontend only — see the CONFIG_* table above).
resolve_agency() {
  local varname="CONFIG_$1"
  local config="${!varname:-}"
  if [[ -z "$config" ]]; then
    echo "Unknown agency '$1'. Expected: ${ALL_AGENCIES[*]}, all." >&2
    return 1
  fi
  IFS='|' read -r BE_PORT FE_PORT IDP_CLIENT_ID APP_NAME OU_HANDLE <<<"$config"
  APP_NAME="${APP_NAME:-$1}"
}

# Source a .env file without clobbering vars already set in the environment.
# This preserves parent-shell overrides (e.g. DB_DRIVER=postgres ./start.sh npqs).
source_env_nonclobber() {
  local file=$1 line key value
  [[ -f "$file" ]] || return 0
  while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in ''|\#*) continue ;; esac
    [[ "$line" == *=* ]] || continue
    key="${line%%=*}"
    key="${key#export }"
    [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
    # Already set in env (even to empty string) -> skip.
    [[ -n "${!key+x}" ]] && continue
    value="${line#*=}"
    # Strip surrounding double or single quotes.
    if [[ "$value" =~ ^\"(.*)\"$ ]]; then
      value="${BASH_REMATCH[1]}"
    elif [[ "$value" =~ ^\'(.*)\'$ ]]; then
      value="${BASH_REMATCH[1]}"
    fi
    export "$key=$value"
  done <"$file"
}

# sqlite_path_from_config extracts db.sqlite.path from a config.yaml file (a
# light grep matching this repo's own fixed 2-space/4-space indentation —
# not a full YAML parser, but sufficient for the checked-in
# config/<agency>/config.yaml files this script points CONFIG_PATH at) so
# seeding, cleanup, and logging target the same SQLite file cmd/server and
# cmd/migrate will use, even if a config.yaml is edited to a non-default
# path. Falls back to $2 when the file has no such line (e.g. driver:
# postgres) or can't be read.
sqlite_path_from_config() {
  local config_path="$1" default="$2"
  local path
  path=$(sed -n 's/^ \{4\}path:[[:space:]]*//p' "$config_path" 2>/dev/null | head -1)
  printf '%s' "${path:-$default}"
}

# ---------------------------------------------------------------------------
# clean_databases: wipe agency DB(s) before starting.
#   SQLite   -> delete each agency's db.sqlite.path file, resolved from its
#               config/<agency>/config.yaml (see sqlite_path_from_config)
#   Postgres -> terminate connections, drop, and recreate the database
# ---------------------------------------------------------------------------
clean_databases() {
  local agencies=("$@")
  local db_driver="${DB_DRIVER:-sqlite}"

  echo "[start-dev] Cleaning agency databases (driver: $db_driver)..."

  if [[ "$db_driver" == "sqlite" ]]; then
    for agency in "${agencies[@]}"; do
      # Resolve CONFIG_PATH the same way start_backend/run_migrations do
      # (relative to BACKEND_DIR, since that's their cwd when they read it).
      local agency_config="${CONFIG_PATH:-config/${agency}/config.yaml}"
      [[ "$agency_config" == /* ]] || agency_config="$BACKEND_DIR/$agency_config"
      local sqlite_path
      sqlite_path=$(sqlite_path_from_config "$agency_config" "./${agency}_applications.db")
      local db_path
      if [[ "$sqlite_path" == /* ]]; then
        db_path="$sqlite_path"
      else
        db_path="$BACKEND_DIR/${sqlite_path#./}"
      fi
      if [[ -f "$db_path" ]]; then
        echo "[start-dev]   Deleting SQLite DB for $agency: $db_path"
        rm -f "$db_path"
      else
        echo "[start-dev]   SQLite DB for $agency not found (nothing to delete): $db_path"
      fi
    done

  elif [[ "$db_driver" == "postgres" ]]; then
    if ! command -v psql >/dev/null 2>&1; then
      echo "[start-dev] Error: psql required for Postgres DB cleaning but not found in PATH." >&2
      exit 1
    fi
    local db_host="${DB_HOST:-localhost}"
    local db_port="${DB_PORT:-5432}"
    local db_user="${DB_USER:-postgres}"
    local db_password="${DB_PASSWORD:-changeme}"
    local db_name="${DB_NAME:-nsw_agency_db}"
    # Postgres uses a single shared database; warn if only a subset of agencies
    # was selected since this will wipe data for all agencies, not just the chosen ones.
    if [[ "${#agencies[@]}" -lt "${#ALL_AGENCIES[@]}" ]]; then
      echo "[start-dev] Warning: Postgres uses a shared database ($db_name). --clean-run will wipe data for ALL agencies, not just: ${agencies[*]}." >&2
    fi
    echo "[start-dev]   Dropping and recreating Postgres database: $db_name"
    PGPASSWORD="$db_password" psql -h "$db_host" -p "$db_port" -U "$db_user" -d postgres -c \
      "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$db_name' AND pid <> pg_backend_pid();" \
      >/dev/null
    PGPASSWORD="$db_password" psql -h "$db_host" -p "$db_port" -U "$db_user" -d postgres \
      -c "DROP DATABASE IF EXISTS \"$db_name\";"
    PGPASSWORD="$db_password" psql -h "$db_host" -p "$db_port" -U "$db_user" -d postgres \
      -c "CREATE DATABASE \"$db_name\";"

  else
    echo "[start-dev] Unknown DB_DRIVER '$db_driver'; skipping database clean." >&2
  fi
}

# run_migrations: apply all pending migrations for each agency DB.
#   cmd/migrate reads the same config/<agency>/config.yaml start_backend
#   points cmd/server at (its db section), so the schema is migrated against
#   exactly the DB each backend will connect to. Always called before
#   starting backends (after clean_databases too, when --clean-run is set)
#   since `migrate up` only applies pending migrations and is a no-op on an
#   up-to-date schema.
#
#   Each agency's config.yaml is read separately even if several point at the
#   same shared Postgres DB (e.g. a developer edited them all to do so) —
#   redundant but harmless, since migrate up is idempotent.
# ---------------------------------------------------------------------------
run_migrations() {
  local agencies=("$@")
  (
    cd "$BACKEND_DIR"
    # Source .env non-clobber so NSW_CLIENT_SECRET/DB_PASSWORD (whichever
    # config.yaml's "{{env:NAME}}" placeholders reference) are available.
    # Parent-shell values still win.
    if [[ -f .env ]]; then
      source_env_nonclobber .env
    fi
    # LoadAndExpand resolves every "{{env:NAME}}" in config.yaml, including
    # nsw.clientSecret, even though migrate's own Config struct never reads
    # it — so it still needs a default here, same as start_backend below.
    export NSW_CLIENT_SECRET="${NSW_CLIENT_SECRET:-1234}"

    echo "[start-dev] Running migrations..."
    for agency in "${agencies[@]}"; do
      local config_path="${CONFIG_PATH:-config/${agency}/config.yaml}"
      echo "[start-dev]   migrate up -> $config_path"
      CONFIG_PATH="$config_path" go run ./cmd/migrate up
    done
  )
}

ensure_branding_file() {
  local agency=$1 app_name=$2
  local config_dir="$ROOT_DIR/frontend/public/configs"
  local file="$config_dir/${agency}.branding.json"
  mkdir -p "$config_dir"
  cat >"$file" <<EOF
{
  "branding": {
    "systemName": "NSW",
    "appName": "${app_name}",
    "logoUrl": "",
    "systemLogoUrl": "",
    "favicon": "",
    "portalName": "",
    "description": "",
    "heroImageUrl": "",
    "partnerLogos": [{"url": "", "alt": ""}]
  }
}
EOF
  echo "[start-dev] Wrote branding file: $file"
}

start_backend() {
  local agency=$1
  resolve_agency "$agency"
  (
    cd "$BACKEND_DIR"
    # The Go server does not autoload .env — source it (non-clobber) so
    # NSW_CLIENT_SECRET (referenced from config/<agency>/config.yaml as
    # "{{env:NSW_CLIENT_SECRET}}") reaches the process. Parent-shell values
    # still win over .env.
    if [[ -f .env ]]; then
      source_env_nonclobber .env
    fi
    export NSW_CLIENT_SECRET="${NSW_CLIENT_SECRET:-1234}"

    # A parent-shell/--env-file CONFIG_PATH still wins, as an escape hatch to
    # point at a config.yaml other than this agency's checked-in one.
    export CONFIG_PATH="${CONFIG_PATH:-config/${agency}/config.yaml}"

    # Still plain env vars: read directly by cmd/cli (seeding, below) — its
    # own config, unlike cmd/server's and cmd/migrate's, wasn't part of this
    # config.yaml migration. DB_PATH's default is derived from the same
    # config.yaml cmd/server/cmd/migrate read, so seeding always targets the
    # same database even if that file customizes db.sqlite.path.
    export DB_DRIVER="${DB_DRIVER:-sqlite}"
    export DB_PATH="${DB_PATH:-$(sqlite_path_from_config "$CONFIG_PATH" "./${agency}_applications.db")}"
    if [[ -z "${USER_CUSTOM_DATA_SCHEMA_PATH:-}" && -f "config/${agency}/user-custom-data-schema.json" ]]; then
      export USER_CUSTOM_DATA_SCHEMA_PATH="./config/${agency}/user-custom-data-schema.json"
    fi

    echo "[start-dev] Starting $agency backend  -> http://localhost:$BE_PORT (db: $DB_PATH, config: $CONFIG_PATH)"

    local seed_file="./data/seed/${agency}_users.json"
    if [[ -f "$seed_file" ]]; then
      echo "[start-dev] Seeding $agency database using $seed_file..."
      go run ./cmd/cli user add --file "$seed_file" || true
    fi

    exec go run ./cmd/server
  ) &
  PIDS+=("$!")
}

start_frontend() {
  local agency=$1
  resolve_agency "$agency"
  echo "[start-dev] Starting $agency frontend -> http://localhost:$FE_PORT (branding: $agency, idp: $IDP_CLIENT_ID)"
  (
    cd "$FRONTEND_DIR"
    # Vite autoloads frontend/.env but only reads VITE_PORT from process env.
    ensure_branding_file "$agency" "$APP_NAME"
    VITE_PORT="${VITE_PORT:-$FE_PORT}" \
    VITE_BRANDING_NAME="${VITE_BRANDING_NAME:-$agency}" \
    VITE_API_BASE_URL="${VITE_API_BASE_URL:-http://localhost:$BE_PORT}" \
    VITE_IDP_BASE_URL="${VITE_IDP_BASE_URL:-$IDP_BASE_URL}" \
    VITE_IDP_CLIENT_ID="${VITE_IDP_CLIENT_ID:-$IDP_CLIENT_ID}" \
    VITE_IDP_EXTRA_QUERY_PARAMS="${VITE_IDP_EXTRA_QUERY_PARAMS:-resource=$AGENCY_API_AUDIENCE}" \
    VITE_IDP_SCOPES="${VITE_IDP_SCOPES:-openid,profile,email,ou,role,agency:application:read,agency:application:review,agency:application:feedback,agency:consignment:read,agency:storage:read,agency:storage:write}" \
    VITE_IDP_EXPECTED_OU_HANDLE="${VITE_IDP_EXPECTED_OU_HANDLE:-$OU_HANDLE}" \
    VITE_APP_URL="${VITE_APP_URL:-http://localhost:$FE_PORT}" \
    exec pnpm run dev
  ) &
  PIDS+=("$!")
}

# Load optional root-level env file before per-agency defaults.
if [[ -n "$ENV_FILE" ]]; then
  if [[ ! -f "$ENV_FILE" ]]; then
    echo "[start-dev] Error: --env-file not found: $ENV_FILE" >&2
    exit 1
  fi
  source_env_nonclobber "$ENV_FILE"
fi

# Resolve the agency list to launch.
if [[ "$AGENCY" == "all" ]]; then
  AGENCIES=("${ALL_AGENCIES[@]}")
else
  # Validate it's a known agency without polluting globals (subshell).
  ( resolve_agency "$AGENCY" > /dev/null ) || usage
  AGENCIES=("$AGENCY")
fi

if [[ "$CLEAN_RUN" == "true" ]]; then
  clean_databases "${AGENCIES[@]}"
fi
run_migrations "${AGENCIES[@]}"

for o in "${AGENCIES[@]}"; do
  [[ "$TARGET" == "all" || "$TARGET" == "backend"  ]] && start_backend  "$o"
  [[ "$TARGET" == "all" || "$TARGET" == "frontend" ]] && start_frontend "$o"
done

echo "[start-dev] ${#PIDS[@]} process(es) running. Logs from all processes will interleave below. Press Ctrl-C to stop."
wait
