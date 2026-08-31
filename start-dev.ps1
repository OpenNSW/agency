# Run Agency backends and/or frontends with per-agency config.
#
# Usage:
#   .\start-dev.ps1 [--clean-run] [--env-file=PATH] <agency> [target]
#
#   <agency>  One of: npqs, fcau, cda, slpa, all
#             'all' fans out and starts every agency in parallel.
#   [target]  One of: all (default), backend, frontend
#
# Flags:
#   --clean-run       Wipe agency database(s) before starting.
#                     SQLite: deletes {agency}_applications.db files.
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
# Env-var precedence (highest to lowest):
#   parent shell env > --env-file > backend/.env (for backend vars) > script defaults
# i.e. $env:PORT=9000; .\start-dev.ps1 npqs honours the override; .env can fill in
# anything the parent didn't set; the per-agency defaults below are the floor.
#
# Examples:
#   .\start-dev.ps1 npqs              # NPQS backend + frontend
#   .\start-dev.ps1 fcau backend      # FCAU backend only
#   .\start-dev.ps1 ird frontend      # IRD frontend only
#   .\start-dev.ps1 all               # every backend + frontend, in parallel
#   .\start-dev.ps1 all backend       # every backend, no frontends
#   .\start-dev.ps1 all --clean-run   # wipe all agency DBs, then start
#
# Ctrl-C terminates every child process.

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$IDP_BASE_URL = 'https://localhost:8090'

# Single source of truth for per-agency config: BE_PORT, FE_PORT, IDP_CLIENT_ID, NSW_CLIENT_ID, APP_NAME, OU_HANDLE.
# Adding an agency means one entry here - nothing else.
$AGENCY_CONFIGS = [ordered]@{
    npqs    = @{ BE_PORT = 8081; FE_PORT = 5174; IDP_CLIENT_ID = 'OGA_PORTAL_APP_NPQS';    NSW_CLIENT_ID = 'NPQS_TO_NSW';    APP_NAME = 'National Plant Quarantine Service (NPQS)'; OU_HANDLE = 'npqs';    NSW_INBOUND_CLIENT_ID = 'NSW_TO_NPQS'    }
    fcau    = @{ BE_PORT = 8082; FE_PORT = 5175; IDP_CLIENT_ID = 'OGA_PORTAL_APP_FCAU';    NSW_CLIENT_ID = 'FCAU_TO_NSW';    APP_NAME = 'Food Control Administration Unit (FCAU)';  OU_HANDLE = 'fcau';    NSW_INBOUND_CLIENT_ID = 'NSW_TO_FCAU'    }
    cda     = @{ BE_PORT = 8083; FE_PORT = 5176; IDP_CLIENT_ID = 'OGA_PORTAL_APP_CDA';     NSW_CLIENT_ID = 'CDA_TO_NSW';     APP_NAME = 'Coconut Development Authority (CDA)';       OU_HANDLE = 'cda';     NSW_INBOUND_CLIENT_ID = 'NSW_TO_CDA'     }
    slpa    = @{ BE_PORT = 8084; FE_PORT = 5177; IDP_CLIENT_ID = 'OGA_PORTAL_APP_SLPA';    NSW_CLIENT_ID = 'SLPA_TO_NSW';    APP_NAME = 'Sri Lanka Ports Authority (SLPA)';          OU_HANDLE = 'slpa';    NSW_INBOUND_CLIENT_ID = 'NSW_TO_SLPA'    }
    customs = @{ BE_PORT = 8085; FE_PORT = 5178; IDP_CLIENT_ID = 'OGA_PORTAL_APP_CUSTOMS'; NSW_CLIENT_ID = 'CUSTOMS_TO_NSW'; APP_NAME = 'Sri Lanka Customs (CUSTOMS)';          OU_HANDLE = 'customs'; NSW_INBOUND_CLIENT_ID = 'NSW_TO_CUSTOMS' }
    sltb    = @{ BE_PORT = 8086; FE_PORT = 5179; IDP_CLIENT_ID = 'OGA_PORTAL_APP_SLTB';    NSW_CLIENT_ID = 'SLTB_TO_NSW';    APP_NAME = 'Sri Lanka Tea Board (SLTB)';                OU_HANDLE = 'sltb';    NSW_INBOUND_CLIENT_ID = 'NSW_TO_SLTB'    }
}


$ALL_AGENCIES = $AGENCY_CONFIGS.Keys | Sort-Object

function Show-Usage {
    $agencies = ($ALL_AGENCIES -join ', ') + ', all'
    Write-Host @"
Usage: .\start-dev.ps1 [--clean-run] [--env-file=PATH] <agency> [target]

  <agency>  One of: $agencies
  [target]  One of: all (default), backend, frontend

Flags:
  --clean-run       Wipe agency DB(s) before starting (migrations always run)
  --env-file=PATH   Load a root-level env file (non-clobbering);
  --env-file PATH   both forms are supported

Examples:
  .\start-dev.ps1 npqs              # NPQS backend + frontend
  .\start-dev.ps1 fcau backend      # FCAU backend only
  .\start-dev.ps1 all               # every agency, backends + frontends
  .\start-dev.ps1 all frontend      # every agency, frontends only
  .\start-dev.ps1 all --clean-run   # wipe all agency DBs, then start
"@ -ForegroundColor Yellow
    exit 1
}

# Parse flags and positional args manually to support kebab-case flags
# (PowerShell param() does not allow hyphens in parameter names).
$CLEAN_RUN  = $false
$ENV_FILE   = ''
$POSITIONAL = [System.Collections.Generic.List[string]]::new()

$i = 0
while ($i -lt $args.Count) {
    $arg = $args[$i]
    if ($arg -eq '--clean-run') {
        $CLEAN_RUN = $true
    } elseif ($arg -like '--env-file=*') {
        $ENV_FILE = $arg.Substring('--env-file='.Length)
    } elseif ($arg -eq '--env-file') {
        $i++
        if ($i -ge $args.Count -or $args[$i] -like '--*') {
            Write-Host "[start-dev] Error: --env-file requires a path value." -ForegroundColor Red
            exit 1
        }
        $ENV_FILE = $args[$i]
    } else {
        $POSITIONAL.Add($arg)
    }
    $i++
}

$Agency = if ($POSITIONAL.Count -gt 0) { $POSITIONAL[0] } else { '' }
$Target = if ($POSITIONAL.Count -gt 1) { $POSITIONAL[1] } else { 'all' }

if ($Agency -eq '') { Show-Usage }

if ($Target -notin @('all', 'backend', 'frontend')) {
    Write-Host "[start-dev] Unknown target '$Target'." -ForegroundColor Red
    Show-Usage
}

if (-not $AGENCY_CONFIGS.Contains($Agency) -and $Agency -ne 'all') {
    Write-Host "[start-dev] Unknown agency '$Agency'. Expected: $($ALL_AGENCIES -join ', '), all." -ForegroundColor Red
    exit 1
}

$ROOT_DIR     = $PSScriptRoot
$BACKEND_DIR  = Join-Path $ROOT_DIR 'backend'
$FRONTEND_DIR = Join-Path $ROOT_DIR 'frontend'

# Cross-platform process execution helpers (Windows cmd.exe vs POSIX sh).
$isWindows = ($env:OS -like '*Windows*') -or ($PSVersionTable.Platform -eq 'Win32NT') -or ($IsWindows)
$shellCmd  = if ($isWindows) { 'cmd.exe' } else { '/bin/sh' }
$shellArg  = if ($isWindows) { '/c' }      else { '-c' }

$jobs = [System.Collections.Generic.List[System.Diagnostics.Process]]::new()

function Stop-AllJobs {
    if ($jobs.Count -eq 0) { return }
    Write-Host ""
    Write-Host "[start-dev] Stopping $($jobs.Count) process(es)..."
    foreach ($p in $jobs) {
        if (-not $p.HasExited) {
            try {
                if ($isWindows) {
                    Start-Process taskkill.exe -ArgumentList '/F', '/T', '/PID', $p.Id -NoNewWindow -Wait | Out-Null
                } elseif ($PSVersionTable.PSVersion.Major -ge 7) {
                    $p.Kill($true)
                } else {
                    Start-Process kill -ArgumentList '-TERM', "-$($p.Id)" -NoNewWindow -Wait | Out-Null
                }
            } catch {
                try { $p.Kill() } catch { }
            }
        }
    }
    foreach ($p in $jobs) {
        try { $p.WaitForExit(3000) | Out-Null } catch { }
    }
}

# Source a .env file without clobbering vars already set in the env block.
# Preserves parent-shell overrides (e.g. $env:PORT=9000; .\start-dev.ps1 npqs).
function Merge-EnvFile {
    param([string]$Path, $Block)
    if (-not (Test-Path $Path)) { return }
    foreach ($line in Get-Content $Path) {
        if ($line -match '^\s*$' -or $line -match '^\s*#') { continue }
        if ($line -notmatch '=') { continue }
        $line = $line -replace '^export\s+', ''
        $idx  = $line.IndexOf('=')
        $key  = $line.Substring(0, $idx).Trim()
        $val  = $line.Substring($idx + 1).Trim()
        if ($val -match '^"(.*)"\s*(#.*)?$') {
            $val = $Matches[1]
        } elseif ($val -match "^'(.*)'\s*(#.*)?$") {
            $val = $Matches[1]
        } else {
            $val = ($val -split '\s+#')[0].Trim()
        }
        if ($key -notmatch '^[A-Za-z_][A-Za-z0-9_]*$') { continue }
        if (-not $Block.Contains($key)) { $Block[$key] = $val }
    }
}

function Clean-Databases {
    param([string[]]$Agencies)
    # Build env: parent shell > --env-file > backend/.env (highest to lowest).
    $dbEnv = [System.Environment]::GetEnvironmentVariables('Process').Clone()
    if ($ENV_FILE -ne '') { Merge-EnvFile -Path $ENV_FILE -Block $dbEnv }
    # Capture explicit DB_NAME from parent + --env-file before .env fills in defaults.
    # backend/.env's DB_NAME is excluded so per-agency names apply by default.
    $explicitDbName = if ($dbEnv.Contains('DB_NAME')) { $dbEnv['DB_NAME'] } else { $null }
    Merge-EnvFile -Path (Join-Path $BACKEND_DIR '.env') -Block $dbEnv

    $dbDriver = if ($dbEnv.Contains('DB_DRIVER')) { $dbEnv['DB_DRIVER'] } else { 'sqlite' }
    Write-Host "[start-dev] Cleaning agency databases (driver: $dbDriver)..."

    if ($dbDriver -eq 'sqlite') {
        foreach ($agency in $Agencies) {
            $dbPath = Join-Path $BACKEND_DIR "${agency}_applications.db"
            if (Test-Path $dbPath) {
                Write-Host "[start-dev]   Deleting SQLite DB for ${agency}: $dbPath"
                Remove-Item $dbPath -Force
            } else {
                Write-Host "[start-dev]   SQLite DB for ${agency} not found (nothing to delete): $dbPath"
            }
        }
    } elseif ($dbDriver -eq 'postgres') {
        $psqlCmd = if ($isWindows) { 'psql.exe' } else { 'psql' }
        if (-not (Get-Command $psqlCmd -ErrorAction SilentlyContinue)) {
            Write-Host "[start-dev] Error: psql required for Postgres DB cleaning but not found in PATH." -ForegroundColor Red
            exit 1
        }
        $dbHost     = if ($dbEnv.Contains('DB_HOST'))     { $dbEnv['DB_HOST'] }     else { 'localhost' }
        $dbPort     = if ($dbEnv.Contains('DB_PORT'))     { $dbEnv['DB_PORT'] }     else { '5432' }
        $dbUser     = if ($dbEnv.Contains('DB_USER'))     { $dbEnv['DB_USER'] }     else { 'postgres' }
        $dbPassword = if ($dbEnv.Contains('DB_PASSWORD')) { $dbEnv['DB_PASSWORD'] } else { 'changeme' }
        # Postgres uses per-agency databases to avoid concurrent migration conflicts.
        # If DB_NAME is set in parent env or --env-file, treat it as a shared-DB override.
        $env:PGPASSWORD = $dbPassword
        foreach ($agency in $Agencies) {
            $dbName = if ($explicitDbName) { $explicitDbName } else { "${agency}_nsw_agency_db" }
            Write-Host "[start-dev]   Dropping and recreating Postgres database: $dbName"
            & $psqlCmd -h $dbHost -p $dbPort -U $dbUser -d postgres -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$dbName' AND pid <> pg_backend_pid();" | Out-Null
            if ($LASTEXITCODE -ne 0) {
                Write-Host "[start-dev] Error: Failed to terminate connections to $dbName" -ForegroundColor Red
                Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue; exit $LASTEXITCODE
            }
            & $psqlCmd -h $dbHost -p $dbPort -U $dbUser -d postgres -c "DROP DATABASE IF EXISTS `"$dbName`";"
            if ($LASTEXITCODE -ne 0) {
                Write-Host "[start-dev] Error: Failed to drop Postgres database $dbName" -ForegroundColor Red
                Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue; exit $LASTEXITCODE
            }
            & $psqlCmd -h $dbHost -p $dbPort -U $dbUser -d postgres -c "CREATE DATABASE `"$dbName`";"
            if ($LASTEXITCODE -ne 0) {
                Write-Host "[start-dev] Error: Failed to create Postgres database $dbName" -ForegroundColor Red
                Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue; exit $LASTEXITCODE
            }
        }
        Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    } else {
        Write-Host "[start-dev] Unknown DB_DRIVER '$dbDriver'; skipping database clean." -ForegroundColor Yellow
    }
}

function Ensure-BrandingFile {
    param([string]$AgencyName, [string]$AppName)
    $ConfigDir = Join-Path $FRONTEND_DIR 'public/configs'
    $FilePath  = Join-Path $ConfigDir "${AgencyName}.branding.json"
    New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null
    $Content = @"
{
  "branding": {
    "systemName": "NSW",
    "appName": "$AppName",
    "logoUrl": "",
    "systemLogoUrl": "",
    "favicon": "",
    "portalName": "",
    "description": "",
    "heroImageUrl": "",
    "partnerLogos": [{"url": "", "alt": ""}]
  }
}
"@
    $Utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText($FilePath, $Content, $Utf8NoBom)
    Write-Host "[start-dev] Wrote branding file: $FilePath"
}

# Run-Migrations: apply all pending migrations for each agency DB.
#   Runs `go run ./cmd/migrate up` from BACKEND_DIR with the same DB_DRIVER /
#   DB_PATH values that Start-Backend uses, so the schema is ready before the
#   server starts. Always called before starting backends (after
#   Clean-Databases too, when --clean-run is set) since `migrate up` only
#   applies pending migrations and is a no-op on an up-to-date schema.
function Run-Migrations {
    param([string[]]$Agencies)
    # Build env: parent shell > --env-file > backend/.env (highest to lowest).
    $migrEnv = [System.Environment]::GetEnvironmentVariables('Process').Clone()
    if ($ENV_FILE -ne '') { Merge-EnvFile -Path $ENV_FILE -Block $migrEnv }
    $explicitDbName = if ($migrEnv.Contains('DB_NAME')) { $migrEnv['DB_NAME'] } else { $null }
    Merge-EnvFile -Path (Join-Path $BACKEND_DIR '.env') -Block $migrEnv

    $dbDriver = if ($migrEnv.Contains('DB_DRIVER')) { $migrEnv['DB_DRIVER'] } else { 'sqlite' }
    Write-Host "[start-dev] Running migrations (driver: $dbDriver)..."

    foreach ($agency in $Agencies) {
        $agencyEnv = $migrEnv.Clone()
        if ($dbDriver -eq 'sqlite') {
            Write-Host "[start-dev]   migrate up -> ${agency}_applications.db"
            $agencyEnv['DB_DRIVER'] = 'sqlite'
            $agencyEnv['DB_PATH']   = "./${agency}_applications.db"
        } else {
            $dbName = if ($explicitDbName) { $explicitDbName } else { "${agency}_nsw_agency_db" }
            Write-Host "[start-dev]   migrate up -> $dbName"
            $agencyEnv['DB_NAME'] = $dbName
        }
        $psi = [System.Diagnostics.ProcessStartInfo]::new($shellCmd, "$shellArg `"go run ./cmd/migrate up`"")
        $psi.WorkingDirectory = $BACKEND_DIR
        $psi.UseShellExecute  = $false
        foreach ($k in $agencyEnv.Keys) { $psi.EnvironmentVariables[$k] = [string]$agencyEnv[$k] }
        $proc = [System.Diagnostics.Process]::Start($psi)
        $proc.WaitForExit()
        if ($proc.ExitCode -ne 0) {
            Write-Host "[start-dev] Error: Migration failed for $agency (exit code $($proc.ExitCode))." -ForegroundColor Red
            exit $proc.ExitCode
        }
    }
}

function Start-Backend {
    param([string]$AgencyName)
    $cfg         = $AGENCY_CONFIGS[$AgencyName]
    $bePort      = $cfg.BE_PORT
    $nswClientId = $cfg.NSW_CLIENT_ID

    Write-Host "[start-dev] Starting $AgencyName backend  -> http://localhost:$bePort"

    $envBlock = [System.Environment]::GetEnvironmentVariables('Process').Clone()
    if ($ENV_FILE -ne '') { Merge-EnvFile -Path $ENV_FILE -Block $envBlock }

    # Per-agency values set before .env so .env cannot override them.
    # Final precedence: parent env > --env-file > per-agency defaults > .env > script fallback.
    if (-not $envBlock.Contains('PORT'))             { $envBlock['PORT']             = "$bePort"                                  }
    # Native dev serves the frontend via Vite (Start-Frontend), so the backend
    # must stay API-only. Point WEB_DIR at a path that won't exist so a stray
    # backend\web\ (e.g. from a `pnpm build --outDir ../backend/web` test) can't
    # make the server serve the SPA and then fatal on missing VITE_* config.
    if (-not $envBlock.Contains('WEB_DIR'))          { $envBlock['WEB_DIR']          = (Join-Path $BACKEND_DIR '.nonexistent-web') }
    if (-not $envBlock.Contains('DB_PATH'))          { $envBlock['DB_PATH']          = "./${AgencyName}_applications.db"          }
    if (-not $envBlock.Contains('DB_NAME'))          { $envBlock['DB_NAME']          = "${AgencyName}_nsw_agency_db"              }
    if (-not $envBlock.Contains('NSW_CLIENT_ID'))    { $envBlock['NSW_CLIENT_ID']    = $nswClientId                               }
    if (-not $envBlock.Contains('AUTH_EXPECTED_OU')) { $envBlock['AUTH_EXPECTED_OU'] = $cfg.OU_HANDLE                             }
    if (-not $envBlock.Contains('ALLOWED_ORIGINS'))  { $envBlock['ALLOWED_ORIGINS']  = "http://localhost:$($cfg.FE_PORT)"         }
    # Artifacts (task configs + forms + manifest.json) live OUTSIDE this repo,
    # per agency. Default to a sibling checkout of the one-trade-artifacts repo
    # (…/one-trade-artifacts/<agency>, relative to backend/). Override
    # ARTIFACT_LOCAL_ROOT to point elsewhere, or set ARTIFACT_LOADER_TYPE=github|s3
    # to load from a remote source instead.
    if (-not $envBlock.Contains('ARTIFACT_LOADER_TYPE')) { $envBlock['ARTIFACT_LOADER_TYPE'] = 'local'                                    }
    if (-not $envBlock.Contains('ARTIFACT_LOCAL_ROOT'))  { $envBlock['ARTIFACT_LOCAL_ROOT']  = "../../one-trade-artifacts/${AgencyName}" }

    $clientIds = $cfg.IDP_CLIENT_ID
    if ($cfg.Contains('NSW_INBOUND_CLIENT_ID') -and $cfg.NSW_INBOUND_CLIENT_ID -ne '') {
        $clientIds = "$clientIds,$($cfg.NSW_INBOUND_CLIENT_ID)"
    }
    if (-not $envBlock.Contains('AUTH_CLIENT_IDS'))  { $envBlock['AUTH_CLIENT_IDS']  = $clientIds                                 }
    if (-not $envBlock.Contains('NSW_CLIENT_SECRET')) { $envBlock['NSW_CLIENT_SECRET'] = '1234'                                   }

    $dotEnv = Join-Path $BACKEND_DIR '.env'
    if (Test-Path $dotEnv) {
        Merge-EnvFile -Path $dotEnv -Block $envBlock
    } else {
        Write-Host "[start-dev] WARNING: backend/.env not found - backend will fail if NSW_* vars are unset." -ForegroundColor Yellow
    }

    # DB_DRIVER falls back to sqlite only if not set by parent env, --env-file, or .env.
    if (-not $envBlock.Contains('DB_DRIVER')) { $envBlock['DB_DRIVER'] = 'sqlite' }

    # Seed the database before starting
    $seedFile = Join-Path $BACKEND_DIR "data/seed/${AgencyName}_users.json"
    if (Test-Path $seedFile) {
        Write-Host "[start-dev] Seeding $AgencyName database using $seedFile..."
        $seedPsi = [System.Diagnostics.ProcessStartInfo]::new($shellCmd, "$shellArg `"go run ./cmd/cli user add --file data/seed/${AgencyName}_users.json`"")
        $seedPsi.WorkingDirectory = $BACKEND_DIR
        $seedPsi.UseShellExecute  = $false
        foreach ($k in $envBlock.Keys) { $seedPsi.EnvironmentVariables[$k] = [string]$envBlock[$k] }
        $seedProc = [System.Diagnostics.Process]::Start($seedPsi)
        $seedProc.WaitForExit()
    }

    $psi = [System.Diagnostics.ProcessStartInfo]::new($shellCmd, "$shellArg `"go run ./cmd/server`"")
    $psi.WorkingDirectory = $BACKEND_DIR
    $psi.UseShellExecute  = $false
    foreach ($k in $envBlock.Keys) { $psi.EnvironmentVariables[$k] = [string]$envBlock[$k] }
    $jobs.Add([System.Diagnostics.Process]::Start($psi))
}

function Start-Frontend {
    param([string]$AgencyName)
    $cfg       = $AGENCY_CONFIGS[$AgencyName]
    $fePort    = $cfg.FE_PORT
    $bePort    = $cfg.BE_PORT
    $idpClient = $cfg.IDP_CLIENT_ID
    $appName   = $cfg.APP_NAME
    $ouHandle  = $cfg.OU_HANDLE

    Ensure-BrandingFile -AgencyName $AgencyName -AppName $appName

    Write-Host "[start-dev] Starting $AgencyName frontend -> http://localhost:$fePort (branding: $AgencyName, idp: $idpClient)"

    $envBlock = [System.Environment]::GetEnvironmentVariables('Process').Clone()
    if ($ENV_FILE -ne '') { Merge-EnvFile -Path $ENV_FILE -Block $envBlock }

    if (-not $envBlock.Contains('VITE_PORT'))                   { $envBlock['VITE_PORT']                  = "$fePort"                  }
    if (-not $envBlock.Contains('VITE_BRANDING_NAME'))          { $envBlock['VITE_BRANDING_NAME']          = $AgencyName               }
    if (-not $envBlock.Contains('VITE_API_BASE_URL'))           { $envBlock['VITE_API_BASE_URL']           = "http://localhost:$bePort" }
    if (-not $envBlock.Contains('VITE_IDP_BASE_URL'))           { $envBlock['VITE_IDP_BASE_URL']           = $IDP_BASE_URL             }
    if (-not $envBlock.Contains('VITE_IDP_CLIENT_ID'))          { $envBlock['VITE_IDP_CLIENT_ID']          = $idpClient                }
    if (-not $envBlock.Contains('VITE_IDP_SCOPES')) {
        $envBlock['VITE_IDP_SCOPES'] = @(
            'openid', 'profile', 'email', 'ou', 'role',
            'agency:application:read', 'agency:application:review', 'agency:application:feedback',
            'agency:consignment:read', 'agency:storage:read', 'agency:storage:write'
        ) -join ','
    }
    if (-not $envBlock.Contains('VITE_IDP_EXPECTED_OU_HANDLE')) { $envBlock['VITE_IDP_EXPECTED_OU_HANDLE'] = $ouHandle                 }
    if (-not $envBlock.Contains('VITE_APP_URL'))                { $envBlock['VITE_APP_URL']                = "http://localhost:$fePort" }

    $psi = [System.Diagnostics.ProcessStartInfo]::new($shellCmd, "$shellArg `"pnpm run dev`"")
    $psi.WorkingDirectory = $FRONTEND_DIR
    $psi.UseShellExecute  = $false
    foreach ($k in $envBlock.Keys) { $psi.EnvironmentVariables[$k] = [string]$envBlock[$k] }
    $jobs.Add([System.Diagnostics.Process]::Start($psi))
}

# Load optional root-level env file before per-agency defaults.
if ($ENV_FILE -ne '') {
    if (-not (Test-Path $ENV_FILE)) {
        Write-Host "[start-dev] Error: --env-file not found: $ENV_FILE" -ForegroundColor Red
        exit 1
    }
}

# Resolve the agency list to launch.
if ($Agency -eq 'all') {
    $agencyList = $ALL_AGENCIES
} else {
    $agencyList = @($Agency)
}

if ($CLEAN_RUN) {
    Clean-Databases -Agencies $agencyList
}
Run-Migrations -Agencies $agencyList

try {
    foreach ($a in $agencyList) {
        if ($Target -eq 'all' -or $Target -eq 'backend')  { Start-Backend  $a }
        if ($Target -eq 'all' -or $Target -eq 'frontend') { Start-Frontend $a }
    }

    Write-Host "[start-dev] $($jobs.Count) process(es) running. Logs from all processes will interleave below. Press Ctrl-C to stop."

    # Mirror bash 'wait': keep running until Ctrl-C even if individual processes crash.
    # Log exits as they happen but do not kill the remaining processes.
    $reported = @{}
    while ($true) {
        foreach ($p in $jobs) {
            if ($p.HasExited -and -not $reported[$p.Id]) {
                Write-Host "[start-dev] Process $($p.Id) exited with code $($p.ExitCode)." -ForegroundColor Yellow
                $reported[$p.Id] = $true
            }
        }
        Start-Sleep -Milliseconds 500
    }
} finally {
    Stop-AllJobs
}
