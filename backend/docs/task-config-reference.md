# `TaskConfig` — Full Reference

This document explains every field of the `taskconfig.TaskConfig` Go struct
(`internal/taskconfig/task_config.go`), part by part, with how each one is
actually consumed by the codebase. For the artifact-loading mechanics
(manifest, storage backends, resolution flow) see [`task-configs.md`](./task-configs.md) —
this doc focuses on the struct itself, field by field, including
`permissions` and `certificate`, which are newer additions not yet covered
there.

## The struct

```go
type TaskConfig struct {
    TaskCode    string           `json:"taskCode"`
    Meta        TaskMeta         `json:"meta"`
    Forms       TaskForms        `json:"forms"`
    Behavior    *TaskBehavior    `json:"behavior,omitempty"`
    Permissions []Permission     `json:"permissions,omitempty"`
    Certificate *TaskCertificate `json:"certificate,omitempty"`
}
```

A single JSON file is loaded per `taskCode` from the artifact registry (kind
`task_config`). Every field is optional **except `permissions`, which is
required and must be non-empty** (see the `permissions` section below) —
enforced by `TaskConfig.Validate` at load time. A config with only
`meta.title` and a non-empty `permissions` is valid; a config with no
`permissions` at all fails to load.

## Full example

This example exercises every field, including the two not shown in
`task-configs.md`:

```json
{
  "taskCode": "moh:fcau:health_cert:v1",
  "meta": {
    "title": "Health Certificate Review",
    "description": "Review the health certificate application for food export.",
    "icon": "emoji:🏥",
    "category": "Food Control"
  },
  "forms": {
    "view": "moh_fcau_health_cert_v1_view",
    "review": "moh_fcau_health_cert_v1_review"
  },
  "behavior": {
    "outcomeField": "review_outcome",
    "statusMap": {
      "approve": "APPROVED",
      "reject": "REJECTED",
      "needs_more_info": "FEEDBACK_REQUESTED"
    }
  },
  "permissions": [
    { "role": "lab_officer", "actions": ["VIEW", "REVIEW"] },
    { "role": "supervisor",  "actions": ["VIEW", "REVIEW", "FEEDBACK"] }
  ],
  "certificate": {
    "templateId": "moh_health_cert_template_v1",
    "dataSchema": {
      "type": "object",
      "required": ["certificate_id"],
      "properties": {
        "certificate_id": { "type": "string" }
      }
    }
  }
}
```

---

## `taskCode` (string, optional)

```go
TaskCode string `json:"taskCode"`
```

The logical task type this config applies to — e.g. `moh:fcau:health_cert:v1`.
This is the value NSW injects onto an `Application` record (`record.TaskCode`)
and is used as the lookup key into the artifact registry
(`taskconfigart.Load(ctx, registry, record.TaskCode)`).

- If omitted from the JSON, the artifact's filename (without `.json`) is used
  as the effective ID/manifest key instead — the field itself isn't
  defaulted in Go, the manifest `id` just takes over.
- Every other field in this struct is scoped to this one task code; a
  deployment has one config file per distinct task code it wants to
  customize.

## `meta` (`TaskMeta`, required in practice)

```go
type TaskMeta struct {
    Title       string `json:"title"`
    Description string `json:"description,omitempty"`
    Icon        string `json:"icon,omitempty"`
    Category    string `json:"category,omitempty"`
}
```

Pure UI display metadata, surfaced verbatim on `Application.Title` /
`.Description` / `.Icon` / `.Category` by `internal/application/service.go`
whenever the config is found (both in the list endpoint and the single-task
`GET` endpoint).

| Field         | Required | Purpose                                                                                    |
|---------------|----------|---------------------------------------------------------------------------------------------|
| `title`       | yes      | Shown in the task list and as the review screen header.                                     |
| `description` | no       | One-line subtitle shown under the title.                                                    |
| `icon`        | no       | Icon hint. The frontend currently only renders `emoji:<char>`-prefixed values; anything else is ignored. |
| `category`    | no       | Grouping label shown in the task list, e.g. `Food Control`.                                  |

If the task config can't be loaded at all (not in the manifest, or a
transient loader miss), `Meta` is left zero-valued and these fields are
simply omitted from the API response — the application record itself still
loads.

## `forms` (`TaskForms`)

```go
type TaskForms struct {
    View   string `json:"view,omitempty"`
    Review string `json:"review,omitempty"`
}
```

References to separately-stored form definitions (artifact kind
`generic_template`, see `forms.md`), resolved via `generictemplate.Load`.

| Field    | Required | Purpose                                                                                          |
|----------|----------|---------------------------------------------------------------------------------------------------|
| `view`   | no       | Form ID for the **read-only** rendering of the trader's submitted data. Attached to the response as `dataForm`. Omit if the task has nothing trader-submitted to show. |
| `review` | no       | Form ID for the **officer's review action** form (approve/reject/etc). Attached as `agencyForm`. Omit if the task has no review action. |

Resolution is best-effort per form: if a referenced form ID isn't found in
the registry, the field is simply omitted from the response and a warning is
logged (`"view form not found"` / `"review form not found"`) — it does not
fail the whole request.

## `behavior` (`*TaskBehavior`, optional — nil-able)

```go
const DefaultOutcomeField = "review_outcome"

type TaskBehavior struct {
    OutcomeField string            `json:"outcomeField,omitempty"`
    StatusMap    map[string]string `json:"statusMap,omitempty"`
}
```

Declaratively wires the officer's review submission to a final application
status, so the service doesn't need hardcoded outcome logic per task type.

| Field          | Required | Purpose                                                                                                   |
|----------------|----------|-------------------------------------------------------------------------------------------------------------|
| `outcomeField` | no       | Key read from the `POST /api/v1/applications/{taskId}/review` request body. Defaults to `"review_outcome"` (`DefaultOutcomeField`) when empty. |
| `statusMap`    | no       | Maps the outcome field's value (e.g. `"approve"`) to the status stored on the application (e.g. `"APPROVED"`). |

Resolution, on review submission:
1. Read `body[outcomeField]` (or `body["review_outcome"]` if `outcomeField` is unset).
2. Look that value up in `statusMap`.
3. If `Behavior` is nil, `statusMap` doesn't contain the value, or the field
   is missing from the body entirely, the status defaults to `"DONE"`.

The set of valid outcome values (`approve`, `reject`, `pass`, `fail`, …) is
whatever the **review form's own schema** allows (typically a `oneOf`) —
`statusMap` should have one entry per value that form can actually produce.

Common resulting statuses:

| Status               | Meaning                                          |
|----------------------|---------------------------------------------------|
| `PENDING`             | Awaiting officer review (set at injection).       |
| `APPROVED`            | Officer approved.                                 |
| `REJECTED`            | Officer rejected.                                 |
| `FEEDBACK_REQUESTED`  | Officer sent the task back to the trader.         |
| `DONE`                | Generic completion when nothing else matched.     |

Being a pointer, `Behavior` is entirely optional at the JSON level — a config
with no `"behavior"` key is functionally identical to one whose review always
falls through to `DONE`.

## `permissions` (`[]Permission`, required, non-empty)

```go
type Permission struct {
    Role    string   `json:"role"`
    Actions []string `json:"actions"`
}
```

Per-task, per-role access control. Each entry says: users holding `role` may
perform the actions listed in `actions` on applications of this task code.

**`permissions` must be present and non-empty, and every entry must have a
non-empty `role` and at least one `action`.** This is enforced by
`TaskConfig.Validate` (`internal/taskconfig/task_config.go`), called from
`taskconfigart.loadable.Parse` on every load. A config JSON file that omits
`permissions` (or sets it to `[]`) fails to load — it is not a valid task
config, and the artifact registry surfaces a genuine (non-`ErrNotFound`)
error for it. This closes off what used to be an implicit default: earlier,
an empty `Permissions` was interpreted as "every authenticated user may
perform every action" on that task.

**A task code with no config at all is denied by default, not opened.**
Both `rbac.Middleware.RequireAction` and the application service's access
resolution treat the registry's `ErrNotFound` the same way they'd treat an
explicit empty-permissions config: nobody is authorized. In practice this
means a `taskCode` NSW can inject but that this deployment hasn't yet
authored a config for is fully inaccessible — `VIEW`/`REVIEW`/`FEEDBACK` all
403 — until a config with `permissions` is added for it. This is a
deliberate secure-by-default choice: an unconfigured task fails closed
rather than silently granting everyone access.

How the action set is evaluated once a valid config is loaded
(`internal/rbac/middleware.go`, `ResolveAccess`):
1. Build the set of role names the current user holds.
2. For each `Permission` entry whose `Role` matches one of the user's roles,
   the task becomes "accessible" and its `Actions` are unioned in (deduped)
   into the allowed-actions set.
3. A role the user *doesn't* hold contributes nothing — permissions are
   purely additive across the roles a user has.

Two places consume the result differently:

- **List endpoint** (`GET /api/v1/applications`) — uses only the boolean
  "accessible" result to *filter out* applications the user has no role for
  at all; tasks that fail this check are silently excluded from the list.
- **Single-task endpoint / route middleware** — uses the *action set*.
  `rbac.Middleware.RequireAction(action)` is applied per-route in
  `cmd/server/main.go` and 403s if the resolved actions don't contain the
  route's required action:

  | Route                                         | Required action |
  |------------------------------------------------|------------------|
  | `GET /api/v1/applications/{taskId}`             | `VIEW`           |
  | `POST /api/v1/applications/{taskId}/review`     | `REVIEW`         |
  | `POST /api/v1/applications/{taskId}/feedback`   | `FEEDBACK`       |
  | `POST /api/v1/applications/{taskId}/claim`      | `REVIEW`         |
  | `POST /api/v1/applications/{taskId}/release`    | `REVIEW`         |
  | `POST /api/v1/applications/{taskId}/certificate`| `REVIEW`         |

  Action strings are otherwise free-form (whatever the config's `actions`
  arrays contain is compared verbatim against the route's required string —
  there's no fixed enum in Go), but in practice the deployed configs use
  `VIEW`, `REVIEW`, and `FEEDBACK` to line up with these routes.

If the task config can't be resolved at all for a request (`ErrNotFound`),
both call sites deny access: the RBAC route middleware responds 403 directly
(no permissions exist to check), and `buildApplication` (used by
`GetApplication`, `GetApplicationByTaskCode`, and indirectly
`ReviewApplication`) calls `rbac.ResolveAccess(roles, nil)`, which returns no
allowed actions since there are no `Permission` entries to match a role
against. A **genuine** load failure (bad credentials, malformed JSON,
network error) is treated differently from a **missing** config in both
places — it hard-fails the request (500) rather than silently denying or
granting access, since a transient loader error shouldn't be
indistinguishable from either a real "no config" or a real "has config"
state.

## `certificate` (`*TaskCertificate`, optional — nil-able)

```go
type TaskCertificate struct {
    TemplateID string          `json:"templateId"`
    DataSchema json.RawMessage `json:"dataSchema,omitempty"`
}
```

Lets an officer generate a certificate while reviewing this task, via
`POST /api/v1/certificates/generate` (handled by
`internal/certificate/handler.go`).

| Field        | Required | Purpose                                                                                                   |
|--------------|----------|---------------------------------------------------------------------------------------------------------|
| `templateId` | yes (if `certificate` is present) | ID of the certificate template to render. Copied onto `Application.CertificateTemplateID`; the generate handler 404s if it's empty on the application. |
| `dataSchema` | no       | A JSON Schema, validated **client-side**, describing the shape of the `data` payload the generate request must send (e.g. requiring a `certificate_id` field). Copied onto `Application.CertificateDataSchema`. |

Why `dataSchema` is separate from `forms.review`'s own schema (straight from
the doc comment in the struct): the review form's required fields include
things — a signed certificate upload, an authorized signature — that can
only exist *after* the certificate has already been generated and printed.
Reusing the review form's schema verbatim for the generate-time validation
would deadlock: it'd require fields that don't exist yet at generation time.
`certificate.dataSchema` is deliberately its own, smaller schema scoped to
just what's needed to generate the certificate.

If `Certificate` is nil, `Application.CertificateTemplateID` and
`.CertificateDataSchema` are left empty, and the certificate-generation route
will reject the request for that application.

---

## Where the whole struct is loaded from

- Storage/manifest mechanics, the resolution flow for `GET /applications/{taskId}`,
  and how to add a brand-new task config file are covered in
  [`task-configs.md`](./task-configs.md) — nothing here changes that flow, this
  document only adds the `permissions` and `certificate` fields that file
  doesn't yet mention.
- Parsing/validation entry point: `internal/taskconfig/taskconfigart/taskconfigart.go`
  (`loadable.Parse` → `json.Unmarshal` into `taskconfig.TaskConfig`).
- Primary struct definition: `internal/taskconfig/task_config.go`.