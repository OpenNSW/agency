# Task Configurations

A **task config** is the per-`taskCode` JSON file that drives the agency officer review UI. For each `taskCode` that the NSW workflow can inject, a task config defines:

- **UI metadata** — title, description, icon, and category shown in the task list and review screen header.
- **Form references** — which [forms](./forms.md) to render for the trader-submitted data view and the officer's review action.
- **Behavior** — how the officer's review outcome maps to a final application status.

Forms themselves are stored separately and referenced by ID; the same form can be reused across multiple task configs. See [`forms.md`](./forms.md) for the form file structure.

## Storage and Loading

Task configs (and forms) are **not** stored in this repo. They are loaded through the
[`core/artifact`](https://github.com/OpenNSW/core/tree/main/artifact) registry from a
single source selected at startup — a local directory, a GitHub repo, or an S3 bucket
(see [Configuration](#configuration)).

Each source holds a `manifest.json` at its root that catalogs every artifact as a row of
`(id, kind, version, path)`. Task configs use kind `task_config`; the config's `taskCode`
(or its filename) is the `id`. Example manifest row:

```json
{ "id": "moh:fcau:health_cert:v1", "kind": "task_config", "version": "", "path": "task-configs/moh_fcau_health_cert_v1.json" }
```

The registry fetches a config by `taskCode` on demand: it looks up the path in the
manifest, loads the bytes through the loader, and parses + validates them into a
`taskconfig.TaskConfig`.

## Schema

```json
{
  "taskCode": "fcau_general_application_v1",
  "meta": {
    "title": "General Food Export Application Review",
    "description": "Review the general application for food control administration.",
    "icon": "emoji:📋",
    "category": "Food Control"
  },
  "forms": {
    "view": "fcau_general_application_v1_view",
    "review": "fcau_general_application_v1_review"
  },
  "behavior": {
    "outcomeField": "review_outcome",
    "statusMap": {
      "approve": "APPROVED",
      "reject": "REJECTED",
      "needs_more_info": "FEEDBACK_REQUESTED"
    }
  }
}
```

| Field                    | Required | Description                                                                                                                          |
|--------------------------|----------|--------------------------------------------------------------------------------------------------------------------------------------|
| `taskCode`               | optional | Logical task code. If omitted, the filename (without `.json`) is used.                                                               |
| `meta.title`             | yes      | Display title shown in the task list and review screen header.                                                                       |
| `meta.description`       | no       | One-line description shown under the title.                                                                                          |
| `meta.icon`              | no       | Icon hint. Currently the frontend renders only `emoji:<char>`-prefixed values; other formats are ignored.                            |
| `meta.category`          | no       | Category label shown in the task list (e.g. `Food Control`).                                                                         |
| `forms.view`             | no       | Form ID for the read-only display of the trader's submitted data. Omit if the task has no trader-side data to display.               |
| `forms.review`           | no       | Form ID for the officer's review action form. Omit if there's no review action.                                                      |
| `behavior.outcomeField`  | no       | Name of the field in the review submission body whose value is looked up in `statusMap`. Defaults to `review_outcome`.               |
| `behavior.statusMap`     | no       | Maps the outcome field's value to a final application status. If absent or no key matches, status defaults to `DONE`.                |
| `referenceNumber`        | no       | Enables server-generated reference numbers for the task. Omit it and no reference number is generated. See [Reference Numbers](#reference-numbers). |
| `referenceNumber.field`  | no       | Review-form field prefilled with the generated value. Defaults to `reference_number`.                                                |
| `referenceNumber.agencyCode` | no   | Counter the value is drawn from. Task codes sharing an agency code share one series. Defaults to the task code.                      |
| `referenceNumber.prefix` | no       | Text prepended to the zero-padded sequence value (e.g. `034/`).                                                                      |
| `referenceNumber.minDigits` | no    | Zero-padded width of the sequence value (e.g. `5` renders sequence 1 as `00001`).                                                    |

## Resolution Flow

When `GET /api/v1/applications/{taskId}` is called:

1. The application record is loaded from the database; it carries `taskCode`.
2. The task configuration is resolved by `taskCode` from the artifact registry:
   - **Hit** → returns the parsed config.
   - **Miss** (not in the manifest, or the loader can't fetch it) → returns an error; the response omits all metadata and form fields, and a warning is logged.
3. Each non-empty form reference in the config is resolved from the registry (kind `generic_template`):
   - Hit → form JSON is attached to the response as `dataForm` (view) or `agencyForm` (review).
   - Miss → a warning is logged and the field is omitted.
4. If the config declares `referenceNumber` and the application does not have one yet, a number is allocated from the configured counter, persisted on the record, and returned as `referenceNumber`. See [Reference Numbers](#reference-numbers).
5. On review submission via `POST /api/v1/applications/{taskId}/review`, if `behavior.statusMap` is set and the request body contains a matching `review_outcome` value, the application's status is set accordingly. Otherwise it defaults to `DONE`.

## Adding a New Task

These steps happen in the **artifacts source** (the local dir / GitHub repo / S3 bucket the loader points at), not in this repo.

1. Decide the `taskCode` that NSW will inject for this task type (e.g. `moh:fcau:health_cert:v1`).

2. Author the form file(s) and add a `generic_template` row per form to `manifest.json`. See [`forms.md`](./forms.md) for the file structure.

3. Create the task config file, e.g. `task-configs/moh_fcau_health_cert_v1.json`:

   ```json
   {
     "taskCode": "moh:fcau:health_cert:v1",
     "meta": {
       "title": "Health Certificate Review",
       "icon": "emoji:🏥",
       "category": "Food Control"
     },
     "forms": {
       "review": "moh_fcau_health_cert_v1_review"
     },
     "behavior": {
       "statusMap": {
         "approve": "APPROVED",
         "reject":  "REJECTED"
       }
     }
   }
   ```

4. Add a `task_config` row for it to `manifest.json`:

   ```json
   { "id": "moh:fcau:health_cert:v1", "kind": "task_config", "version": "", "path": "task-configs/moh_fcau_health_cert_v1.json" }
   ```

5. Restart the Agency service — the manifest is read once at startup, then artifacts are fetched on demand.

## Status Mapping

The `behavior.statusMap` field declaratively wires the officer's review action to the final application status, removing the need for hardcoded outcome logic in the service.

- The review form is expected to produce a field whose value identifies the outcome. By default this field is `review_outcome`; configure `behavior.outcomeField` to read from a different field name.
- The values that field can produce (`approve`, `reject`, `pass`, `fail`, …) are defined by the review form's schema (typically via `oneOf`).
- Each possible value should appear as a key in `statusMap`; the mapped value becomes the application's stored status.
- If `statusMap` is absent, the outcome field is missing from the submission, or the value doesn't match any key, the status defaults to `DONE`.

Common statuses used by the frontend:

| Status               | Meaning                                               |
|----------------------|-------------------------------------------------------|
| `PENDING`            | Awaiting officer review (set at injection).           |
| `APPROVED`           | Officer approved.                                     |
| `REJECTED`           | Officer rejected.                                     |
| `FEEDBACK_REQUESTED` | Officer sent the task back to the trader for changes. |
| `DONE`               | Generic completion when no `statusMap` matches.       |

## Reference Numbers

A task whose config carries a `referenceNumber` block gets an application reference
number minted by the agency service, so officers never type one in and two applications
can never share one.

```json
"referenceNumber": {
  "field": "reference_number",
  "agencyCode": "FCAU",
  "prefix": "034/",
  "minDigits": 5
}
```

- The number is allocated the first time `GET /api/v1/applications/{taskId}` resolves the
  application, then stored on the record. Later reads return the stored value and never
  advance the counter.
- Counters live in the `reference_sequences` table, keyed by `(agencyCode, prefix)`, and
  are incremented with a single atomic upsert, so concurrent requests cannot collide.
- Values are rendered as `<prefix><sequence zero-padded to minDigits>`; with the example
  above sequence 1 becomes `034/00001`. A counter that outgrows `minDigits` keeps
  counting: sequence 100000 becomes `034/100000`.
- The response carries the value as `referenceNumber` and the target field name as
  `referenceNumberField`. The frontend merges it into the review form data, so marking
  the field read-only in the form's `uiSchema` (`"options": { "readonly": true }`) shows
  it without letting the officer edit it.
- On review submission the server overwrites `referenceNumber.field` in the payload with
  the stored value, so a client that bypasses the read-only UI cannot substitute its own.

Task configs and forms are not tracked in this repo. They are provided per deployment through the artifact loader, configured by the variables below.

## Configuration

The loader backend is chosen by `ARTIFACT_LOADER_TYPE`; only the selected backend's variables are read. The manifest (`manifest.json`) and the artifacts it catalogs are fetched through the same loader, so both share one origin.

| Variable                    | Backend | Description                                                        |
|-----------------------------|---------|--------------------------------------------------------------------|
| `ARTIFACT_LOADER_TYPE`      | all     | `local`, `github`, or `s3` (default `local`).                      |
| `ARTIFACT_LOCAL_ROOT`       | local   | Directory that artifact paths and `manifest.json` resolve against. |
| `ARTIFACT_GITHUB_OWNER`     | github  | Repository owner.                                                  |
| `ARTIFACT_GITHUB_REPO`      | github  | Repository name.                                                   |
| `ARTIFACT_GITHUB_REF`       | github  | Branch, tag, or commit SHA (prefer an immutable tag/SHA).          |
| `ARTIFACT_GITHUB_BASE_PATH` | github  | Optional in-repo directory prefix.                                 |
| `ARTIFACT_GITHUB_TOKEN`     | github  | Token for private repos / higher rate limits.                      |
| `ARTIFACT_S3_BUCKET`        | s3      | Bucket name.                                                       |
| `ARTIFACT_S3_REGION`        | s3      | AWS region.                                                        |
| `ARTIFACT_S3_ENDPOINT`      | s3      | Optional custom endpoint for S3-compatible stores.                 |
| `ARTIFACT_S3_PREFIX`        | s3      | Optional in-bucket key prefix.                                     |

See [`.env.example`](../.env.example) for the complete set (including the remaining `ARTIFACT_GITHUB_*` / `ARTIFACT_S3_*` options).