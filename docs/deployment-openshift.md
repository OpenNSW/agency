# Deploying NSW Agency on OpenShift

This guide describes how to deploy the NSW Agency service to OpenShift,
including the **database migration** flow (baked into the image) and the **user/role seed**
flow (mounted dynamically at deploy time).

The whole app ships as a **single image**: one Go server serves both the API and the
officer-portal SPA from the same process and port. There is no separate frontend workload.

It is written for a per-agency deployment model (NPQS, FCAU, IRD, CDA) backed by PostgreSQL.

---

## 1. Architecture overview

Each agency runs **one** workload:

| Workload                      | Image                          | Container port                | Exposed via         |
|-------------------------------|--------------------------------|-------------------------------|---------------------|
| Agency (Go server: API + SPA) | `ghcr.io/opennsw/agency:<tag>` | `8081` (override with `PORT`) | `Service` + `Route` |

The server emits the SPA's runtime config (`VITE_*`) at `/runtime-env.js` from its
environment, so the same image is reconfigurable per environment/agency without a rebuild.

The image is OpenShift-friendly out of the box:

- Runs as UID `1001` (`appuser`); no privileged mode or root is required, so it tolerates
  OpenShift's random UID policy.

### What ships in the image vs. what is supplied at deploy time

| Artifact                            | Source                                                 | How it reaches the pod                                                               |
|-------------------------------------|--------------------------------------------------------|--------------------------------------------------------------------------------------|
| `agency` server binary              | root `Dockerfile`                                      | Baked into image (`/app/agency`)                                                     |
| `migrate` CLI binary                | root `Dockerfile`                                      | Baked into image (`/app/migrate`)                                                    |
| `nswac` CLI binary                  | root `Dockerfile`                                      | Baked into image (`/usr/local/bin/nswac`)                                            |
| Officer-portal SPA                  | `frontend/` (built in the image)                       | Baked into image (`/app/web`, served by the server when `WEB_DIR` resolves)          |
| SQL migrations                      | `backend/migrations/`                                  | **Baked into image** (`/app/migrations`)                                             |
| Task configs                & forms | External artifact source (GitHub repo or S3/R2 bucket) | **Fetched at runtime** via the artifact loader — not baked into the image (see §4.2) |
| User/role seed JSON                 | `backend/data/seed/<agency>_users.json`                | **Mounted at deploy time** via ConfigMap (see §5)                                    |
| All configuration / secrets         | env vars                                               | `Secret` + `ConfigMap` (see §4)                                                      |

---

## 2. Build and push the image

The image is built from the **repo root** (the build context needs both `backend/` and
`frontend/`). It bakes in the server binary, the `migrate` and `nswac` CLIs, the built SPA,
and the SQL migrations. Task configs and form templates are **not** baked in — they are
fetched at runtime by the artifact loader (§4.2). The `nswac` **binary** is in the image;
the seed **data** is supplied dynamically (§5), so you can re-seed without rebuilding.

```bash
docker build -t ghcr.io/opennsw/agency:<tag> .
docker push   ghcr.io/opennsw/agency:<tag>
```

> Tagged releases (`vX.Y.Z`) are built and published automatically by
> [.github/workflows/release.yml](../.github/workflows/release.yml); the commands above are
> for ad-hoc/local builds.

---

## 3. Provision PostgreSQL

The server supports `sqlite` and `postgres`. For OpenShift use PostgreSQL — pods are
ephemeral and SQLite on an emptyDir would be lost on restart.

Provision a Postgres instance (OpenShift template, operator, or an external managed DB) and
note the connection details. Each agency may use a separate database, or a single shared
database — the migrations and seed are idempotent per database.

---

## 4. Create the configuration objects

### 4.1 Secret — credentials

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: agency-secrets
  labels: { app: agency }
type: Opaque
stringData:
  DB_PASSWORD: "<postgres-password>"
  NSW_CLIENT_SECRET: "<m2m-client-secret>"
  # Only when ARTIFACT_LOADER_TYPE=s3 with static credentials (must be set
  # together). Omit to use the pod's default AWS credential chain. For a private
  # GitHub source use ARTIFACT_GITHUB_TOKEN here instead.
  ARTIFACT_S3_ACCESS_KEY: "<r2-access-key>"
  ARTIFACT_S3_SECRET_KEY: "<r2-secret-key>"
```

### 4.2 ConfigMap — non-secret config

A single ConfigMap holds all non-secret config for the one workload — server, database,
inbound/outbound auth, and the `VITE_*` values the server serves to the browser.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: agency-config
  labels: { app: agency }
data:
  # Server
  PORT: "8081"
  ALLOWED_ORIGINS: "https://agency.apps.example.com"

  # Database (PostgreSQL)
  DB_DRIVER: "postgres"
  DB_HOST: "postgresql"
  DB_PORT: "5432"
  DB_USER: "postgres"
  DB_NAME: "nsw_agency_db"
  DB_SSLMODE: "require"
  MIGRATION_DIR: "/app/migrations"

  # Artifact loader — where task configs, forms, and manifest.json are fetched
  # from at runtime. Use "github" (pin an immutable ref) or "s3" (e.g. R2) in
  # production; "local" only fits a mounted volume. Only the selected backend's
  # vars are read. See backend/.env.example for the full set.
  ARTIFACT_LOADER_TYPE: "s3"
  ARTIFACT_S3_BUCKET: "one-trade-artifacts"
  ARTIFACT_S3_REGION: "auto"
  ARTIFACT_S3_ENDPOINT: "https://<accountid>.r2.cloudflarestorage.com"
  ARTIFACT_S3_PREFIX: "fcau"
  # For github instead:
  # ARTIFACT_LOADER_TYPE: "github"
  # ARTIFACT_GITHUB_OWNER: "OpenNSW"
  # ARTIFACT_GITHUB_REPO: "one-trade-artifacts"
  # ARTIFACT_GITHUB_REF: "<tag-or-sha>"
  # ARTIFACT_GITHUB_BASE_PATH: "fcau"

  # Inbound auth (validate JWTs from SPA / NSW)
  AUTH_JWKS_URL: "https://idp.example.com/oauth2/jwks"
  AUTH_ISSUER: "https://idp.example.com"
  AUTH_AUDIENCE: "AGENCY_API"
  AUTH_CLIENT_IDS: "<SPA_AGENCY_PORTAL>,<M2M_NSW_TO_AGENCY>"
  AUTH_EXPECTED_OU: "fcau"

  # Outbound M2M to NSW API
  NSW_API_BASE_URL: "https://nsw.example.com/api/v1"
  NSW_CLIENT_ID: "<M2M_AGENCY_TO_NSW>"
  NSW_TOKEN_URL: "https://idp.example.com/oauth2/token"
  NSW_SCOPES: "nsw:task:write,nsw:consignment:read"

  # Browser runtime config (served at /runtime-env.js). The API and SPA share one
  # origin now, so VITE_API_BASE_URL and VITE_APP_URL both point at this route.
  VITE_BRANDING_NAME: "fcau"
  VITE_API_BASE_URL: "https://agency.apps.example.com"
  VITE_APP_URL: "https://agency.apps.example.com"
  VITE_IDP_BASE_URL: "https://idp.example.com"
  VITE_IDP_CLIENT_ID: "<AGENCY_PORTAL_CLIENT_ID>"
  VITE_IDP_SCOPES: "openid,profile,email,ou,role"
  VITE_IDP_EXPECTED_OU_HANDLE: "fcau"
```

> Do **not** set `AUTH_JWKS_INSECURE_SKIP_VERIFY` / `NSW_TOKEN_INSECURE_SKIP_VERIFY` in
> production — those are dev-only TLS-skip flags. Make sure the cluster trusts the IdP/NSW
> certificate chain instead. These flags are now **enforced**: the backend refuses to start
> if either is `true` unless `APP_ENV=development`, so a stray insecure flag fails closed in
> production rather than silently trusting an unverified certificate. Do **not** set
> `APP_ENV=development` in a deployment.

---

## 5. Migrations and seed

### 5.1 Migrations — init container (runs every rollout)

Migrations are baked into the image at `/app/migrations` and applied by the `migrate up`
command. Run it as an **init container** so the schema is up to date before the server
starts on every rollout. `migrate up` is idempotent — already-applied migrations are skipped.

This init container is defined inside the Deployment in §6.

### 5.2 Seed data — mount dynamically via ConfigMap

The seed JSON is **not** baked into the image — supply it at deploy time so it can change
without rebuilding. Create a ConfigMap from the agency seed file:

```bash
oc create configmap agency-seed-data \
  --from-file=fcau_users.json=backend/data/seed/fcau_users.json
```

The file format (see `backend/cmd/cli`):

```json
{
  "users": [
    { "name": "Jane Doe", "email": "jane@agency.gov.au", "roles": ["lab_officer"] }
  ]
}
```

### 5.3 Seed Job — run on demand

Seeding is a one-shot, idempotent operation (existing users are skipped), so run it as a
`Job` rather than wiring it into the pod lifecycle. Re-run it whenever the seed ConfigMap
changes.

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: agency-seed
  labels: { app: agency }
spec:
  backoffLimit: 3
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: seed
          image: ghcr.io/opennsw/agency:<tag>
          command: ["nswac", "user", "add", "--file", "/seed/fcau_users.json"]
          envFrom:
            - configMapRef: { name: agency-config }
            - secretRef:    { name: agency-secrets }
          volumeMounts:
            - name: seed-data
              mountPath: /seed
              readOnly: true
      volumes:
        - name: seed-data
          configMap:
            name: agency-seed-data
```

Run it (and re-run after updating the ConfigMap):

```bash
oc apply -f seed-job.yaml
oc delete job agency-seed --ignore-not-found && oc apply -f seed-job.yaml   # to re-run
oc logs -f job/agency-seed
```

> The seed Job depends on the schema existing. Run it **after** the first rollout
> (whose init container applies the migrations), or add a matching `migrate up` init
> container to the Job if you want it fully standalone.

---

## 6. Deployment, Service, Route

A single Deployment runs the server, which serves both the API and the officer-portal SPA
on port `8081`.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agency
  labels: { app: agency }
spec:
  replicas: 2
  selector:
    matchLabels: { app: agency }
  template:
    metadata:
      labels: { app: agency }
    spec:
      # Apply pending migrations before the server starts. Idempotent.
      initContainers:
        - name: migrate
          image: ghcr.io/opennsw/agency:<tag>
          command: ["/app/migrate", "up"]
          envFrom:
            - configMapRef: { name: agency-config }
            - secretRef:    { name: agency-secrets }
      containers:
        - name: agency
          image: ghcr.io/opennsw/agency:<tag>
          ports:
            - containerPort: 8081
          envFrom:
            - configMapRef: { name: agency-config }
            - secretRef:    { name: agency-secrets }
          readinessProbe:
            httpGet: { path: /health, port: 8081 }
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet: { path: /health, port: 8081 }
            initialDelaySeconds: 10
            periodSeconds: 15
---
apiVersion: v1
kind: Service
metadata:
  name: agency
  labels: { app: agency }
spec:
  selector: { app: agency }
  ports:
    - port: 8081
      targetPort: 8081
---
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: agency
  labels: { app: agency }
spec:
  to: { kind: Service, name: agency }
  port: { targetPort: 8081 }
  tls: { termination: edge }
```

> **Task configs and form templates** are fetched at runtime by the artifact loader from the
> source configured in §4.2 (`ARTIFACT_LOADER_TYPE`), so no volume mount or baked-in content
> is needed. The server reads `manifest.json` from that source at startup and **fails fast**
> if it is unreachable, then fetches individual artifacts on demand. For reproducibility, pin
> the source to an immutable ref (a GitHub tag/SHA, or a versioned S3/R2 prefix) rather than a
> moving branch.

---

## 7. Deployment order

```bash
# 1. Config + secrets
oc apply -f secret.yaml
oc apply -f config.yaml

# 2. Seed ConfigMap (from repo file; task configs & forms are fetched at runtime via the artifact loader)
oc create configmap agency-seed-data \
  --from-file=fcau_users.json=backend/data/seed/fcau_users.json

# 3. Deploy (init container runs `migrate up` automatically)
oc apply -f agency.yaml
oc rollout status deploy/agency

# 4. Seed users/roles (after schema exists)
oc apply -f seed-job.yaml
oc logs -f job/agency-seed
```

---

## 8. Per-agency matrix

Deploy the same image per agency, changing only configuration:

| Setting | NPQS | FCAU | CDA | SLPA |
| --- | --- | --- | --- | --- |
| `AUTH_EXPECTED_OU` / `VITE_IDP_EXPECTED_OU_HANDLE` | `npqs` | `fcau` | `cda` | `slpa` |
| `VITE_BRANDING_NAME` | `npqs` | `fcau` | `cda` | `slpa` |
| Seed file | `npqs_users.json` | `fcau_users.json` | `cda_users.json` | `slpa_users.json` |
| `NSW_CLIENT_ID` / `VITE_IDP_CLIENT_ID` | agency-specific | agency-specific | agency-specific | agency-specific |

Use a separate namespace (or a name suffix) per agency, e.g. `agency-fcau`.

---

## 9. Verification

```bash
# Migrations applied
oc logs deploy/agency -c migrate

# Health — the runtime image is slim (no curl/wget inside the pod), so probe
# the endpoint through the Route from your machine instead.
curl -k "https://$(oc get route agency -o jsonpath='{.spec.host}')/health"

# Seeded users (check Job output)
oc logs job/agency-seed   # → "nswac: successfully imported N user(s)"

# Route (serves both the portal and the API)
oc get route agency
```

---

## 10. Operational notes

- **Re-running migrations:** every rollout runs `migrate up` via the init container; it is a
  no-op when there is nothing pending. Roll back the last migration manually with a one-off
  pod: `oc run migrate-down --rm -it --restart=Never --image=<image> --command --
  /app/migrate down` (wire in the same env).
- **Re-seeding:** update `agency-seed-data` ConfigMap, then delete and re-apply the seed
  Job. Existing users are skipped, so it is safe to re-run.
- **Secrets:** keep `DB_PASSWORD` and `NSW_CLIENT_SECRET` only in the `Secret`. Never put
  them in the ConfigMap or image.
- **Scaling:** the server is stateless when using PostgreSQL, so `replicas` can be raised
  freely. Avoid SQLite (`DB_DRIVER=sqlite`) on OpenShift — it does not survive pod restarts
  and cannot be shared across replicas.
