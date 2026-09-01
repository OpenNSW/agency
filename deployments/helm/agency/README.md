# NSW Agency Helm Chart

Helm chart for deploying the **NSW Agency** application. A single image serves
both the API and the officer portal from one process, so the chart deploys a
single release. Supply your environment's settings by layering the example
override file from the parent `helm/` directory on top of the chart defaults.

## Files Included
- **[deployment.yaml](templates/deployment.yaml)**: Deployment, container, ports, environment variables, mounts, and probes.
- **[configmap.yaml](templates/configmap.yaml)**: Renders `config` (config.yaml) and `configFiles` (accompanying static files) into a ConfigMap, mounted read-only into the server and migration containers (when either is set).
- **[service.yaml](templates/service.yaml)**: Exposes the container port as a cluster-internal Service.
- **[route.yaml](templates/route.yaml)**: Exposes the Service externally via an OpenShift Route (when `route.enabled`).
- **[ingress.yaml](templates/ingress.yaml)**: Exposes the Service externally via a Kubernetes Ingress (when `ingress.enabled`).

### App config (config.yaml)

The server and migration Job both read a single `config.yaml` (see
`backend/config.example.yaml` for the full schema). Author it as Helm values
under `config:` — the chart renders it into a ConfigMap and mounts it at
`configMountPath` (default `/app/config`), and sets `CONFIG_PATH`
automatically so you don't have to. `configFiles` mounts any accompanying
static files config.yaml references by path (JSON Schemas, data-scope rules)
alongside it in the same directory. See `values-example.yaml` for a complete
worked example. A pod rolls automatically when `config`/`configFiles` content
changes (`checksum/config` annotation).

SECRET fields (`db.postgres.password`, `nsw.clientSecret`,
`artifactLoader.github.token`, `artifactLoader.s3.secretKey`) must stay as
`"{{env:NAME}}"` placeholders under `config:` — Helm does not template
`{{...}}` inside values data, so these pass through to the rendered
config.yaml untouched, for the app's own placeholder expansion to resolve at
startup from the env var named. Back that env var with a Kubernetes Secret
(see [Prerequisite: secrets](#prerequisite-secrets)) and reference it under
`env` with `valueFrom.secretKeyRef`, same as any other secret in this chart —
never write a literal secret value under `config:`.

## Layout

```
deployments/helm/
├── agency/            # this chart (templates + neutral defaults)
└── values-example.yaml    # complete example override (API + portal)
```

## Usage

```bash
helm install agency ./agency -f ../values-example.yaml
```

`values.yaml` holds only neutral defaults. The image, port, environment, and
ingress/route settings live in the example override file — copy it and fill in
your environment's URLs, client IDs, and OU handle. Note that **`image.tag` is
required** (there is no default — see [Releasing](#releasing)); the example file
sets one, or pass `--set image.tag=<version>`.

### Prerequisite: secrets

Secrets are **not** stored in the values files — they are referenced from a
Kubernetes Secret that you create out-of-band before installing:

```bash
kubectl create secret generic agency-secrets \
  --from-literal=db-password=... \
  --from-literal=nsw-client-secret=... \
  --from-literal=artifact-s3-access-key=... \
  --from-literal=artifact-s3-secret-key=...
```

The `artifact-s3-*` keys are only required when `config.artifactLoader.type`
is `s3` and you're using static S3/R2 credentials. Omit them for the `github`
loader or when relying on the pod's default AWS credential chain. For a
private GitHub source, add an `artifact-github-token` key instead and
reference it the same way.

## Accessing the portal (secure context required)

The officer portal logs in via OIDC with PKCE, which uses the Web Crypto API
(`window.crypto.subtle`). Browsers only expose Web Crypto in a **secure
context** — that means **HTTPS**, or plain HTTP on **`localhost`/`127.0.0.1`**.
On a plain-HTTP, non-localhost origin (e.g. `http://agency.example.com` through
an ingress), `crypto.subtle` is `undefined` and `signinRedirect()` fails
silently *after* fetching the discovery document — the page just sits there with
no redirect and no error.

So serve the portal one of these ways:

- **HTTPS** (production and recommended for any real host): terminate TLS at the
  ingress via `ingress.tls` (referencing a `kubernetes.io/tls` Secret) or at the
  OpenShift `route`, and browse `https://<host>`.
- **`localhost` over HTTP** (quick local testing): `kubectl port-forward
  svc/<release> 8080:80` and browse `http://localhost:8080`.

Whichever you pick, `VITE_APP_URL` (the OIDC `redirect_uri`) must **exactly
equal** the origin you browse and be registered as an allowed redirect URI in
the IdP. A quick check in the browser console: `window.isSecureContext` must be
`true`.

## Releasing

The **app image** and the **chart** are versioned and released independently,
because they change at different rates. Each has its own tag namespace:

| Artifact  | Tag            | Workflow                                                            | Publishes                                         |
|-----------|----------------|---------------------------------------------------------------------|---------------------------------------------------|
| App image | `v1.2.3`       | [`release.yml`](../../../.github/workflows/release.yml)             | `ghcr.io/opennsw/agency:1.2.3` + a GitHub Release |
| Chart     | `chart-v0.3.0` | [`release-chart.yml`](../../../.github/workflows/release-chart.yml) | `oci://ghcr.io/opennsw/charts/agency:0.3.0`   |

### `appVersion` vs `image.tag`

These are **not** the same thing, and the chart does not couple them:

- **`appVersion`** (in `Chart.yaml`) is metadata only — a human-readable label
  for "which app version this chart describes." It never sets the image tag.
- **`image.tag`** (in a values file, or `--set image.tag=`) is **required** and
  has no default. The chart deliberately fails to render if it is unset, so a
  missing/typo'd tag is caught at `helm template` time — not as an
  `ImagePullBackOff` at deploy time.

### Releasing the chart

Because chart releases are decoupled, cut them on their own cadence:

```bash
# 1. (Only if you want the chart's DEFAULT image to track a new app version)
#    Edit deployments/helm/agency/Chart.yaml:
#      appVersion: "1.2.3"   # metadata; also bump `version:` if you prefer
#    …and set a matching image.tag in your example/prod values file.
#
# 2. Tag and push. The chart version comes from the tag; appVersion is read
#    from Chart.yaml (NOT overridden by the workflow).
git tag chart-v0.3.0
git push origin chart-v0.3.0
# → published to oci://ghcr.io/opennsw/charts/agency:0.3.0
```

To publish manually (the workflow does exactly this):

```bash
echo "$GITHUB_TOKEN" | helm registry login ghcr.io -u <user> --password-stdin
helm package deployments/helm/agency --version 0.3.0
helm push agency-0.3.0.tgz oci://ghcr.io/opennsw/charts
```

### Development builds

[`build-dev-chart.yml`](../../../.github/workflows/build-dev-chart.yml) publishes
a prerelease chart on every push to `main`/`migration/*` that touches
`deployments/helm/**`, versioned `0.0.0-dev.<run-number>` (e.g. `0.0.0-dev.42`).
Prerelease versions sort below any stable release, so they are only installed
when asked for explicitly, and you must supply the dev `image.tag` to test
against:

```bash
helm install agency oci://ghcr.io/opennsw/charts/agency \
  --version 0.0.0-dev.42 \
  --set image.tag=dev-42-<sha> -f your-values.yaml
# or pull the latest prerelease:
helm pull oci://ghcr.io/opennsw/charts/agency --devel
```

## Consuming the published chart

OCI charts need no `helm repo add`. Install directly by reference:

```bash
helm install agency \
  oci://ghcr.io/opennsw/charts/agency --version 0.1.0 \
  -f values-example.yaml
```

> **The override file is NOT bundled with the published chart.**
> `helm package` only includes this chart directory, so `values-example.yaml`
> (which lives one level up in `deployments/helm/`) does not travel with the OCI
> artifact. A consumer gets the templates and the neutral `values.yaml` defaults
> only — you must supply your own `-f` values file. Grab a starting point from
> this repo's [`deployments/helm/`](../) directory and fill in the placeholder
> URLs, client IDs, and OU handles for your environment.

If the GHCR package is private, `helm registry login ghcr.io` first; otherwise
make the `charts/agency` package public in the org's package settings.

## Configuration Reference

See [values.yaml](values.yaml) for the full list of configurable options, and
the `values-example.yaml` override file for a complete, ready-to-edit example.