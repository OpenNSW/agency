# `TaskConfig` — Full Reference

This document explains every field of the `taskconfig.TaskConfig` Go struct
(`internal/taskconfig/task_config.go`), part by part, with how each one is
actually consumed by the codebase. For the artifact-loading mechanics
(manifest, storage backends, resolution flow) see [`task-configs.md`](./task-configs.md) —
this doc focuses on the struct itself, field by field, including
`certificate`, which is a newer addition not yet covered there.

## The struct

```go
type TaskConfig struct {
    SchemaVersion     int                `json:"schemaVersion"`
    TaskCode          string             `json:"taskCode"`
    Meta              TaskMeta           `json:"meta"`
    Forms             TaskForms          `json:"forms"`
    Behavior          TaskBehavior       `json:"behavior"`
    Permissions       []Permission       `json:"permissions,omitempty"`
    Certificate       *TaskCertificate   `json:"certificate,omitempty"`
    ConsignmentFields []ConsignmentField `json:"consignmentFields,omitempty"`
    RefID             *TaskRefID         `json:"refid,omitempty"`
}
```

A single JSON file is loaded per `taskCode` from the artifact registry (kind
`task_config`). Every task is something an officer reviews, so three things
are required and enforced by `TaskConfig.Validate` at load time — a config
missing any of them fails to load:

- **`permissions`** — non-empty, and collectively grants `VIEW` and `REVIEW`
  to at least one role each (`FEEDBACK` is a valid action too, but optional —
  see the `permissions` section below).
- **`forms.review`** — every task has a review form for the officer to act
  on (see the `forms` section below).
- **`behavior`** — every task needs a resolution mode for the review outcome
  (see the `behavior` section below).

`behavior.statusMap`'s *values* being restricted to `APPROVED`/`REJECTED`/
`FEEDBACK_REQUESTED`, and `outcomeField`/`statusMap` themselves being
non-empty, are not enforced yet — see [Application status
lifecycle](#application-status-lifecycle-canonical-applies-to-every-task)
and the `behavior` section below for that target contract.

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
    "type": "statusMap",
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

## Application status lifecycle (canonical, applies to every task)

Every application, regardless of task type, moves through one fixed set of
statuses. This set is closed — no task config may resolve a review outcome
to a custom status string, and the old `"DONE"` fallback status is being
retired entirely.

| Status               | Set by                                                                                                | Terminal? |
|----------------------|-------------------------------------------------------------------------------------------------------|-----------|
| `PENDING`            | Injection (initial submission), and automatically when a trader resubmits after `FEEDBACK_REQUESTED`. | No        |
| `FEEDBACK_REQUESTED` | Officer review outcome, via `behavior.statusMap`.                                                     | No        |
| `APPROVED`           | Officer review outcome, via `behavior.statusMap`.                                                     | Yes       |
| `REJECTED`           | Officer review outcome, via `behavior.statusMap`.                                                     | Yes       |

Rules that fall out of this:

- **`statusMap` values are restricted to `APPROVED`, `REJECTED`, and
  `FEEDBACK_REQUESTED`.** `PENDING` is never a review outcome — it's only
  ever the injection default or the automatic result of a resubmission.
  `DONE` is retired: no review outcome may resolve to it.
- **`APPROVED` and `REJECTED` are terminal.** Once an application reaches
  either, re-injecting data for that task is rejected — the record is
  locked.
- **Resubmission only happens from `FEEDBACK_REQUESTED`.** A trader
  resubmitting data resets the application to `PENDING` and preserves
  whatever officer claim was already in place. Re-injecting data while an
  application is `PENDING` — including a `PENDING` application that was
  previously claimed and then released without feedback being requested —
  is also rejected; `PENDING` means "awaiting first review," not "open for
  edits."

> **Status:** the terminal-status and resubmission rules above are the
> target contract, not yet live. Today, `CreateApplication` preserves the
> claim on re-injection but does not yet reject re-injection outright for
> terminal or already-`PENDING` applications, and `TaskConfig.Validate` does
> not yet enforce the `statusMap` value restrictions below. Both are tracked
> as follow-up implementation work — task configs should be updated to
> comply now so that enforcement can land without breaking any deployment.
> (`forms.review`, `behavior`, and the closed `permissions` action set
> described elsewhere in this doc *are* enforced today.)

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

| Field         | Required | Purpose                                                                                                  |
|---------------|----------|----------------------------------------------------------------------------------------------------------|
| `title`       | yes      | Shown in the task list and as the review screen header.                                                  |
| `description` | no       | One-line subtitle shown under the title.                                                                 |
| `icon`        | no       | Icon hint. The frontend currently only renders `emoji:<char>`-prefixed values; anything else is ignored. |
| `category`    | no       | Grouping label shown in the task list, e.g. `Food Control`.                                              |

If the task config can't be loaded at all (not in the manifest, or a
transient loader miss), `Meta` is left zero-valued and these fields are
simply omitted from the API response — the application record itself still
loads.

## `forms` (`TaskForms`)

```go
type TaskForms struct {
    View   string `json:"view,omitempty"`
    Review string `json:"review"`
}
```

References to separately-stored form definitions (artifact kind
`generic_template`, see `forms.md`), resolved via `generictemplate.Load`.

| Field    | Required | Purpose                                                                                                                                                                |
|----------|----------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `view`   | no       | Form ID for the **read-only** rendering of the trader's submitted data. Attached to the response as `dataForm`. Omit if the task has nothing trader-submitted to show. |
| `review` | **yes**  | Form ID for the **officer's review action** form (approve/reject/etc). Attached as `agencyForm`. Every task is reviewable, so this is required — enforced by `TaskConfig.Validate`. |

When building an application response (`GET`), resolution is best-effort per
form: if a referenced form ID isn't found in the registry, the field is
simply omitted from the response and a warning is logged (`"view form not
found"` / `"review form not found"`) — it does not fail the whole request.

Injection (`CreateApplication`) is different: when `forms.view` is set, the
injected `data` is validated against that form's schema, and a load,
parse, or resolve failure for the form fails the request closed rather than
silently skipping validation.

## `behavior` (`TaskBehavior`, required)

```go
const DefaultOutcomeField = "review_outcome" // target: to be removed, see below

type TaskBehavior struct {
    Type         BehaviorType      `json:"type"`
    OutcomeField string            `json:"outcomeField,omitempty"`
    StatusMap    map[string]string `json:"statusMap,omitempty"`
}
```

Declaratively wires the officer's review submission to a final application
status, so the service doesn't need hardcoded outcome logic per task type.
`Behavior` is a value type, not a pointer — there's no meaningful "absent"
state to represent now that it's mandatory. `behavior` itself (a valid
`type`) is required — enforced by `TaskConfig.Validate` — since every task
is reviewable and so every task needs a resolution mode for the outcome.

> **Target contract (see [Application status
> lifecycle](#application-status-lifecycle-canonical-applies-to-every-task);
> not yet enforced):** beyond `behavior` itself being required, its
> internals are getting tighter too: `outcomeField` and `statusMap` are
> becoming required whenever `behavior.type` is `statusMap` (an `autoApprove`
> task legitimately has neither).
>
> `outcomeField`'s default (`DefaultOutcomeField`, `"review_outcome"`) is
> being removed along with it — every `statusMap` task will have to state
> `outcomeField` explicitly, even when the value happens to be
> `"review_outcome"`. This closes an implicit default the same way the
> `permissions`/`forms.review`/`behavior` fallbacks already have been:
> nothing about how a review outcome is resolved should be left unstated in
> the config.

| Field          | Required                                             | Purpose                                                                                                                                                                                                                    |
|----------------|------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `outcomeField` | target: yes, whenever `behavior.type` is `statusMap` | Key read from the `POST /api/v1/applications/{taskId}/review` request body. Currently defaults to `"review_outcome"` when unset; that default is being removed.                                                            |
| `statusMap`    | target: yes, whenever `behavior.type` is `statusMap` | Maps the outcome field's value (e.g. `"approve"`) to the status stored on the application. Target: must be non-empty and cover every outcome value the review form's schema can produce — no more, no fewer left unmapped. |

### Allowed `statusMap` values

Every value in `statusMap` must be exactly one of these three
(case-sensitive, uppercase):

| Value                | Meaning                                               |
|----------------------|-------------------------------------------------------|
| `APPROVED`           | Officer approved. Terminal.                           |
| `REJECTED`           | Officer rejected. Terminal.                           |
| `FEEDBACK_REQUESTED` | Officer sent the task back to the trader for changes. |

`PENDING` and `DONE` are **not** valid `statusMap` values: `PENDING` is only
ever set by injection or an automatic resubmission-reset, never by a review
outcome, and `DONE` is retired entirely — see the lifecycle section above.

Resolution, on review submission:
1. Read `body[outcomeField]` (or `body["review_outcome"]` if `outcomeField` is unset).
2. Look that value up in `statusMap`.
3. **Target behavior:** if `statusMap` doesn't contain the value, or the
   field is missing from the body entirely, the review request is rejected
   (400) rather than silently resolving to a status.
   **Current behavior, until enforcement lands:** the status silently
   defaults to `"DONE"` — this is exactly the fallback being removed.

The set of valid outcome values (`approve`, `reject`, `pass`, `fail`, …) is
whatever the **review form's own schema** allows (typically a `oneOf`) —
`statusMap` must have one entry per value that form can actually produce.

`Behavior.Type` being empty (i.e. `behavior` omitted from the JSON, leaving
the field at its zero value) or unrecognized already fails
`TaskConfig.Validate` today — the same way a missing `permissions` does.
Once the target contract above lands, a `statusMap`-type behavior with no
`statusMap`/`outcomeField`, or a `statusMap` containing a value outside the
three above, will fail to load the same way.

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

**Actions are a closed set: `VIEW`, `REVIEW`, and `FEEDBACK`. `VIEW` and
`REVIEW` must each be granted to at least one role, collectively, across the
config's `permissions` entries; `FEEDBACK` is a valid action but not
required.** Also enforced by `TaskConfig.Validate`. An action string outside
that set (a typo, or a leftover from before this was enforced) fails the
config to load, rather than silently 403ing whoever tries to use it later.
`VIEW`/`REVIEW` don't have to go to the same role — a config might grant
`trader` only `VIEW` and `officer` `REVIEW` — but every task, having a
review form, needs someone who can view it and someone who can decide it.
`FEEDBACK` is left optional because not every task's officer workflow sends
data back to the trader for changes.

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

  | Route                                            | Required action |
  |--------------------------------------------------|-----------------|
  | `GET /api/v1/applications/{taskId}`              | `VIEW`          |
  | `POST /api/v1/applications/{taskId}/review`      | `REVIEW`        |
  | `POST /api/v1/applications/{taskId}/feedback`    | `FEEDBACK`      |
  | `POST /api/v1/applications/{taskId}/claim`       | `REVIEW`        |
  | `POST /api/v1/applications/{taskId}/release`     | `REVIEW`        |
  | `POST /api/v1/applications/{taskId}/certificate` | `REVIEW`        |

  The route wiring in `cmd/server/main.go` still compares the required
  action against the config's `actions` as a plain string (no shared Go
  constant between the two), but since `TaskConfig.Validate` now restricts
  `actions` to exactly `VIEW`/`REVIEW`/`FEEDBACK`, those are the only values
  that can appear on either side.

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
`POST /api/v1/applications/{taskId}/certificate` (handled by
`internal/certificate/handler.go`).

| Field        | Required                          | Purpose                                                                                                                                                                                                         |
|--------------|-----------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `templateId` | yes (if `certificate` is present) | ID of the certificate template to render. Copied onto `Application.CertificateTemplateID`; the generate handler 404s if it's empty on the application.                                                          |
| `dataSchema` | no                                | A JSON Schema, validated **client-side**, describing the shape of the `data` payload the generate request must send (e.g. requiring a `certificate_id` field). Copied onto `Application.CertificateDataSchema`. |

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

## `refid` (`*TaskRefID`, optional — nil-able)

Declares that this task's applications get an agency-issued reference ID,
generated by [`github.com/OpenNSW/core/refid`](https://github.com/OpenNSW/core/tree/main/refid)
when the application is first injected and written into the reviewer response.
Omit it entirely for tasks that need no reference number.

```go
type TaskRefID struct {
    Issuer string            `json:"issuer"`
    IDType string            `json:"idType"`
    Path   string            `json:"path"`
    Params map[string]string `json:"params,omitempty"`
}
```

```json
"refid": {
  "issuer": "NPQS",
  "idType": "application_id",
  "path": "/reference_number",
  "params": { "officeCode": "/nppo_office_location" }
}
```

| Field | Meaning |
| --- | --- |
| `issuer`, `idType` | Which configured format to generate. Both required. |
| `path` | JSON Pointer into the reviewer response where the ID is written. |
| `params` | The format's inputs, each a JSON Pointer into this task's **injected data**. |

**The format itself lives in the deployment's config**, not here — `refIDGen`
in `config.yaml` (see [`config.example.yaml`](../config.example.yaml)) declares
the issuers, their segments and any controlled lists. This split is deliberate:
which numbers an agency issues is a deployment decision, while which tasks get
one is a task decision. A task naming an `(issuer, idType)` the deployment
hasn't configured **fails the inject** rather than quietly skipping — an
application with no reference ID is not something to discover later.

`params` is what lets one task config serve every office: `officeCode` is read
off the incoming application rather than fixed, so a per-office counter needs
no config per office. `refid` ignores params a format doesn't consume, so they
can be declared generously; a param the format *does* require but which cannot
be resolved from the injected data fails the inject as a `400`.

**Generated exactly once, on first inject.** Re-injecting an existing
application keeps the number it already has, and a trader resubmitting after a
feedback request keeps it too. Counter state lives in the `refid_sequences`
table (migration `000010`).

**The review form needs a control at `path`** or the officer never sees the
number — `path` targets the same document `forms.review` binds to, surfaced by
the API as `agencyActionData`. Every other field of that document stays absent
until the officer fills it in, which is expected: status and `reviewedAt` are
what mark an application reviewed, not a non-empty reviewer response. Note that
a review form whose schema sets `"additionalProperties": false` without
declaring the field would reject the officer's submission.

> Making that control read-only is up to whoever authors the form, and is a
> client-side convention only: review submissions are not validated
> server-side, so an officer can still overwrite the number. Enforcing that
> properly needs backend validation of the review payload.

## Migration checklist for existing task configs

`permissions` (non-empty, must collectively grant `VIEW`/`REVIEW`, no actions
outside the `VIEW`/`REVIEW`/`FEEDBACK` set), `forms.review`, and `behavior`
being present are already enforced — a config missing any of them fails to
load today, not just under the target contract below.

What's *not* enforced yet is the `statusMap` value contract. Existing task
configs need to be brought in line with it before that validation is turned
on. For every task config (i.e. all of them, since `forms.review` is now
mandatory):

1. Add a `behavior.statusMap` entry if one is missing (only relevant for
   `behavior.type: "statusMap"` — `autoApprove` tasks have neither).
2. List every outcome value the review form's schema (`forms.review`) can
   actually produce, and map each one to exactly one of `APPROVED`,
   `REJECTED`, or `FEEDBACK_REQUESTED`.
3. Remove or fix any value that isn't one of those three — in particular,
   nothing should map to `"DONE"` or `"PENDING"`.
4. Double-check there's no outcome value the form can produce that's
   missing from the map — once enforcement lands, an unmapped outcome will
   be rejected outright (400) instead of silently becoming `"DONE"`.

---

## Where the whole struct is loaded from

- Storage/manifest mechanics, the resolution flow for `GET /applications/{taskId}`,
  and how to add a brand-new task config file are covered in
  [`task-configs.md`](./task-configs.md) — nothing here changes that flow, this
  document only adds the `certificate` field that file doesn't yet mention.
- Parsing/validation entry point: `internal/taskconfig/taskconfigart/taskconfigart.go`
  (`loadable.Parse` → `json.Unmarshal` into `taskconfig.TaskConfig`).
- Primary struct definition: `internal/taskconfig/task_config.go`.