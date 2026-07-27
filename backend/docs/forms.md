# Forms

Forms are [JSON Forms](https://jsonforms.io/) definitions (`schema` + `uiSchema`) that the Agency frontend renders for two purposes:

- **View forms** — read-only renderings of the trader-submitted data shown on the review screen.
- **Review forms** — interactive forms the officer fills in to record their review action.

A form file is purpose-agnostic: the same file can be referenced as a view form by one task and a review form by another. Forms are referenced by ID from [task configs](./task-configs.md) — they are not bound to a `taskCode` themselves.

## Storage and Loading

Forms are **not** stored in this repo. Like [task configs](./task-configs.md), they are
loaded through the [`core/artifact`](https://github.com/OpenNSW/core/tree/main/artifact)
registry from the single source configured at startup — a local directory, a GitHub repo,
or an S3 bucket (see [Configuration](./task-configs.md#configuration)).

Forms use artifact kind `generic_template`; the form ID is the manifest row's `id`. Example
manifest row:

```json
{ "id": "moh_fcau_health_cert_v1_review", "kind": "generic_template", "version": "", "path": "forms/moh_fcau_health_cert_v1_review.json" }
```

The registry fetches a form by ID on demand: it looks up the path in the manifest and loads
the raw JSON bytes through the loader. Forms are resolved by the ID(s) referenced from a
[task config's](./task-configs.md) `forms.view` / `forms.review` fields, not read in bulk at
startup.

## File Structure

Each form file is a top-level object with two keys: `schema` and `uiSchema`.

```json
{
  "schema": {
    "type": "object",
    "required": ["review_outcome"],
    "properties": {
      "review_outcome": {
        "type": "string",
        "title": "Review Outcome",
        "oneOf": [
          { "const": "approve", "title": "Approve" },
          { "const": "reject",  "title": "Reject" }
        ]
      },
      "rejection_reason": { "type": "string", "title": "Reason / Comments" }
    }
  },
  "uiSchema": {
    "type": "VerticalLayout",
    "elements": [
      { "type": "Control", "scope": "#/properties/review_outcome" },
      { "type": "Control", "scope": "#/properties/rejection_reason", "options": { "multi": true } }
    ]
  }
}
```

- `schema` follows standard [JSON Schema](https://json-schema.org/) and is used for both validation and field-title lookup.
- `uiSchema` follows [JSON Forms UI Schema](https://jsonforms.io/docs/uischema/) and controls layout, rules, and rendering options.

No fields are required by the Agency service itself — the form is forwarded to the frontend verbatim. Field requirements (such as `review_outcome` for status-mapping behavior) come from the task config that *references* the form, not from the form file. See [`task-configs.md`](./task-configs.md) for the contract.

## Adding a New Form

These steps happen in the **artifacts source** (the local dir / GitHub repo / S3 bucket the loader points at), not in this repo.

1. Create a `.json` file, e.g. `forms/moh_fcau_health_cert_v1_review.json`. Use any naming convention you like; a useful one is `<taskCode>_view` or `<taskCode>_review` to make the relationship obvious.

2. Populate it with `schema` and `uiSchema`. Validate by running `jq . forms/<file>.json` or pasting into any JSON Forms playground.

3. Add a `generic_template` row for it to `manifest.json`:

   ```json
   { "id": "moh_fcau_health_cert_v1_review", "kind": "generic_template", "version": "", "path": "forms/moh_fcau_health_cert_v1_review.json" }
   ```

4. Reference it from a task config (see [`task-configs.md`](./task-configs.md)):

   ```json
   {
     "forms": { "review": "moh_fcau_health_cert_v1_review" }
   }
   ```

5. Restart the Agency service — the manifest is read once at startup, then artifacts are fetched on demand.