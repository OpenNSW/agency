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
| ----------------------------- | ------------------------------ | ----------------------------- | ------------------- |
| Agency (Go server: API + SPA) | `ghcr.io/opennsw/agency:<tag>` | `8081` (override with `PORT`) | `Service` + `Route` |

The server emits the SPA's runtime config (`VITE_*` keys) at `/config.js` from
`config.yaml`'s `web.runtime` section (§4.2), so the same image is reconfigurable per
environment/agency by editing that ConfigMap, without a rebuild.

The image is OpenShift-friendly out of the box:

- Runs as UID `1001` (`appuser`); no privileged mode or root is required, so it tolerates
  OpenShift's random UID policy.

### What ships in the image vs. what is supplied at deploy time

| Artifact                 | Source                                                 | How it reaches the pod                                                               |
|--------------------------|--------------------------------------------------------|--------------------------------------------------------------------------------------|
| `agency` server binary   | root `Dockerfile`                                      | Baked into image (`/app/agency`)                                                     |
| `migrate` CLI binary     | root `Dockerfile`                                      | Baked into image (`/app/migrate`)                                                    |
| `nswac` CLI binary       | root `Dockerfile`                                      | Baked into image (`/usr/local/bin/nswac`)                                            |
| Officer-portal SPA       | `frontend/` (built in the image)                       | Baked into image (`/app/web`, served by the server when `WEB_DIR` resolves)          |
| SQL migrations           | `backend/migrations/`                                  | **Baked into image** (`/app/migrations`)                                             |
| Task configs & forms     | External artifact source (GitHub repo or S3/R2 bucket) | **Fetched at runtime** via the artifact loader — not baked into the image (see §4.2) |
| User/role seed JSON      | `backend/data/seed/<agency>_users.json`                | **Mounted at deploy time** via ConfigMap (see §5)                                    |
| Non-secret server config | `config.yaml` (mirrors `backend/config.example.yaml`)  | Mounted from a `ConfigMap` (see §4.2)                                                |
| Secrets                  | env vars                                               | `Secret`, referenced from `config.yaml` via `{{env:NAME}}` placeholders (see §4.1)   |

---

## 2. Build and push the image

The image is built from the **repo root** (the build context needs both `backend/` and
`frontend/`). It bakes in the server binary, the `migrate` and `nswac` CLIs, the built SPA,
and the SQL migrations. Task configs and form templates are **not** baked in — they are
fetched at runtime by the artifact loader (§4.2). The `nswac` **binary** is in the image;
the seed **data** is supplied dynamically (§5), so you can re-seed without rebuilding.

The published image is a multi-arch manifest list covering `linux/amd64` and
`linux/arm64`, so one tag resolves correctly on x86_64 and arm64 (Graviton/Ampere)
nodes alike. To reproduce that locally you need `buildx`, which also pushes both
architectures under a single tag:

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t ghcr.io/opennsw/agency:<tag> --push .
```

For a single-architecture image for your own machine, `docker build -t
ghcr.io/opennsw/agency:<tag> .` followed by `docker push` still works.

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

`cmd/server` and `cmd/migrate` read **only** `config.yaml` (via the `CONFIG_PATH` env
var — the one setting that has to stay a literal env var, since it's needed to find the
file in the first place); there is no per-field env-var mapping for anything else. Any
value in it can be a literal or a `"{{env:NAME}}"` / `"{{file:/path}}"` placeholder — see
`backend/config.example.yaml` for the full schema. `cmd/cli` (the `nswac` binary used by
the seed Job in §5.3) is the one exception: it isn't part of this and still reads
`DB_DRIVER`/`DB_HOST`/etc. as plain env vars directly.

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
  # Only when artifactLoader.type: s3 with static credentials (must be set
  # together). Omit to use the pod's default AWS credential chain. For a private
  # GitHub source use ARTIFACT_GITHUB_TOKEN here instead.
  ARTIFACT_S3_ACCESS_KEY: "<r2-access-key>"
  ARTIFACT_S3_SECRET_KEY: "<r2-secret-key>"
```

These keys are never read directly — `config.yaml` below references them by name via
`"{{env:NAME}}"` placeholders, resolved by the app at container startup.

### 4.2 ConfigMap — config.yaml

`config.yaml` is authored here directly (mirroring `backend/config.example.yaml` 1:1 — see
that file for the full schema and every field's meaning) and mounted read-only into the
pod; there is no ConfigMap-of-flat-env-vars any more. This one file carries server,
database, inbound/outbound auth, and the `web.runtime` values the server serves to the
browser at `/config.js`.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: agency-config
  labels: { app: agency }
data:
  config.yaml: |
    port: "8081"
    allowedOrigins: ["https://agency.apps.example.com"]

    db:
      driver: postgres
      postgres:
        host: postgresql
        port: "5432"
        user: postgres
        password: "{{env:DB_PASSWORD}}"
        name: nsw_agency_db
        sslMode: require

    # Task configs and forms are fetched at runtime from an external source, not
    # baked into the image. Use "github" (pinned to an immutable ref) or "s3"
    # (e.g. Cloudflare R2) in a cluster — "local" only works with a mounted volume.
    artifactLoader:
      type: s3
      s3:
        bucket: one-trade-artifacts
        region: auto # "auto" for R2
        endpoint: https://<accountid>.r2.cloudflarestorage.com
        prefix: fcau
        accessKey: "{{env:ARTIFACT_S3_ACCESS_KEY}}"
        secretKey: "{{env:ARTIFACT_S3_SECRET_KEY}}"
      # For github instead:
      # type: github
      # github:
      #   owner: OpenNSW
      #   repo: one-trade-artifacts
      #   ref: "<tag-or-sha>"
      #   basePath: fcau

    # Browser runtime config, served at /config.js (see frontend/src/runtimeConfig.ts).
    # The API and SPA share one origin, so apiBaseURL and appURL both point at this route.
    web:
      dir: web
      runtime:
        brandingName: fcau
        apiBaseURL: https://agency.apps.example.com
        idpBaseURL: https://idp.example.com
        idpClientID: "<AGENCY_PORTAL_CLIENT_ID>"
        idpExpectedOU: fcau
        appURL: https://agency.apps.example.com
        idpScopes: openid,profile,email,ou,role,agency:application:read,agency:application:review,agency:application:feedback,agency:consignment:read,agency:storage:read,agency:storage:write
        # Must equal authn.audience below, or the agency:* scopes are dropped.
        idpExtraQueryParams: resource=https://api.nsw-agency.local

    authn:
      jwksURL: https://idp.example.com/oauth2/jwks
      issuer: https://idp.example.com
      audience: https://api.nsw-agency.local
      clientIDs: ["<SPA_AGENCY_PORTAL>", "NSW_TO_AGENCY"]
      expectedOU: fcau

    nsw:
      baseURL: https://nsw.example.com
      clientID: "<M2M_AGENCY_TO_NSW>"
      clientSecret: "{{env:NSW_CLIENT_SECRET}}"
      tokenURL: https://idp.example.com/oauth2/token
      tokenParams:
        resource: ["https://api.nsw-srilanka.local"]
      scopes:
        - "nsw:task:write"
        - "nsw:consignment:read"
        - "nsw:storage:read"
        - "nsw:storage:write"
```

> Do **not** set `insecureSkipTLSVerify` / `tokenInsecureSkipVerify` (`authn`/`nsw`) to
> `true` in production — those are dev-only TLS-skip flags. Make sure the cluster trusts
> the IdP/NSW certificate chain instead. These are now **enforced**: the backend refuses to
> start if either is `true` unless `environment: development`, so a stray insecure flag
> fails closed in production rather than silently trusting an unverified certificate. Do
> **not** set `environment: development` in a deployment.

### 4.3 ConfigMap — seed CLI env vars

`nswac` (§5.3's seed Job) is not part of the `config.yaml` migration above and still reads
its DB connection as plain env vars, so it needs its own small ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: agency-cli-env
  labels: { app: agency }
data:
  DB_DRIVER: "postgres"
  DB_HOST: "postgresql"
  DB_PORT: "5432"
  DB_USER: "postgres"
  DB_NAME: "nsw_agency_db"
  DB_SSLMODE: "require"
```

Keep these in sync with `config.yaml`'s `db:` section above by hand — they describe the
same database, but `nswac` has no access to `config.yaml`.

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
    {
      "name": "Jane Doe",
      "email": "jane@agency.gov.au",
      "roles": ["lab_officer"]
    }
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
            - configMapRef: { name: agency-cli-env }
            - secretRef: { name: agency-secrets }
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
          env:
            - name: CONFIG_PATH
              value: /app/config/config.yaml
          envFrom:
            - secretRef: { name: agency-secrets }
          volumeMounts:
            - name: config
              mountPath: /app/config
              readOnly: true
      containers:
        - name: agency
          image: ghcr.io/opennsw/agency:<tag>
          ports:
            - containerPort: 8081
          env:
            - name: CONFIG_PATH
              value: /app/config/config.yaml
          envFrom:
            - secretRef: { name: agency-secrets }
          volumeMounts:
            - name: config
              mountPath: /app/config
              readOnly: true
          readinessProbe:
            httpGet: { path: /health, port: 8081 }
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet: { path: /health, port: 8081 }
            initialDelaySeconds: 10
            periodSeconds: 15
      volumes:
        - name: config
          configMap:
            name: agency-config
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
> source configured in §4.2 (`config.yaml`'s `artifactLoader.type`), so no volume mount or
> baked-in content is needed for them. The server reads `manifest.json` from that source at startup and **fails fast**
> if it is unreachable, then fetches individual artifacts on demand. For reproducibility, pin
> the source to an immutable ref (a GitHub tag/SHA, or a versioned S3/R2 prefix) rather than a
> moving branch.

---

## 7. Deployment order

```bash
# 1. Config + secrets
oc apply -f secret.yaml
oc apply -f configmap.yaml       # agency-config (config.yaml) + agency-cli-env

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

| Setting (in `config.yaml`, §4.2)                        | NPQS              | FCAU              | CDA              | SLPA              |
| -------------------------------------------------------- | ----------------- | ----------------- | ---------------- | ----------------- |
| `authn.expectedOU` / `web.runtime.idpExpectedOU`          | `npqs`            | `fcau`            | `cda`            | `slpa`            |
| `web.runtime.brandingName`                                | `npqs`            | `fcau`            | `cda`            | `slpa`            |
| Seed file                                                 | `npqs_users.json` | `fcau_users.json` | `cda_users.json` | `slpa_users.json` |
| `nsw.clientID` / `web.runtime.idpClientID`                | agency-specific   | agency-specific   | agency-specific  | agency-specific   |

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
  pod: `oc run migrate-down --rm -it --restart=Never --image=<image> --command --/app/migrate down`
  (give it the same `config` volume mount + `CONFIG_PATH` + secret env as the init container in §6).
- **Re-seeding:** update `agency-seed-data` ConfigMap, then delete and re-apply the seed
  Job. Existing users are skipped, so it is safe to re-run.
- **Secrets:** keep `DB_PASSWORD` and `NSW_CLIENT_SECRET` only in the `Secret`. Never put
  them in the ConfigMap or image.
- **Scaling:** the server is stateless when using PostgreSQL, so `replicas` can be raised
  freely. Avoid SQLite (`config.yaml`'s `db.driver: sqlite`) on OpenShift — it does not
  survive pod restarts and cannot be shared across replicas.
