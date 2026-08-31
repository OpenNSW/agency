# Data Scoping

Restricting which consignments (and applications) an officer can see, based on comparing their own
[`users.custom_data`](./employee-custom-data.md) against a consignment's
[`custom_data`](./consignment-custom-data.md) — e.g. an officer whose own custom data says
`district: "Colombo"` only sees consignments whose custom data agrees. This is the follow-up both of
those documents' "Future work" sections anticipated, implemented as its own package,
`internal/datascope`, rather than folded into either.

This is a different feature from filtering by *who handled* a consignment (joining
`applications.claimed_by` against `users.custom_data`, still not built — see
[employee-custom-data.md](./employee-custom-data.md#future-work)). Data scoping compares the
*requesting* officer's own attributes against the data, regardless of who touched it.

## Configuring rules

Set `DATA_SCOPE_RULES_PATH` to a JSON file of rules (see [`.env.example`](../.env.example)):

```dotenv
DATA_SCOPE_RULES_PATH=./config/npqs/data-scope-rules.json
```

```json
{
  "rules": [
    { "consignmentField": "/location/district", "userField": "/assignedDistrict" }
  ]
}
```

Both `consignmentField` and `userField` are [JSON Pointers](https://datatracker.ietf.org/doc/html/rfc6901)
(`/a/b/c`) — `consignmentField` into a consignment's `custom_data`, `userField` into the requesting
officer's own `custom_data`. Multiple rules AND together: every rule's value must match for a
consignment (or its applications) to be visible. Left unset, this feature is a complete no-op — every
officer sees everything, exactly as today.

Unlike `consignmentField`s two source documents, these pointers get spliced directly into a SQL
JSON-path expression (Postgres `#>`, SQLite `json_extract`) rather than bound as a query parameter —
neither dialect accepts a bound parameter for the path itself, only for the value being compared. So
each pointer segment is restricted to `[A-Za-z0-9_-]+`, stricter than RFC 6901 otherwise allows, and
rejected at startup if it isn't. Two rules can't target the same `consignmentField` either — a second
rule matching a `consignmentField` already used would silently overwrite the first's resolved value.

## Fail-closed semantics

If an officer's own `custom_data` doesn't have the value a configured `userField` points at — the
attribute was simply never set on them — they see **zero** consignments/applications for that
request, not an unfiltered list. This applies per-request, to every rule: one missing attribute
empties the whole result, even if other rules would have resolved fine. There's no error surfaced to
the officer for this; it looks like there's nothing to see, the same as a search returning no
matches.

A genuine failure to look up the officer's own custom data (a database error, not "attribute not
set") is a different case — that surfaces as an ordinary 500, not an empty page.

## Who's scoped

Only human officers (`authn.KindUser` principals) are scoped. A machine/M2M caller (e.g. NSW's
inject client, `authn.KindClient`) is entirely unaffected — nothing here touches `POST
/api/v1/inject`, and a request with no principal in context at all (shouldn't happen on an
authenticated route, but defensively) is treated the same as unrestricted rather than fail-closed.

There is no role-based bypass (e.g. an "admin sees everything" carve-out) — `internal/rbac` has no
concept of an elevated role today, and this feature doesn't invent one. Every configured rule applies
to every officer uniformly.

## Where it applies

- `GET /api/v1/consignments` — filtered in SQL (`consignment.Store.List`), so pagination/totals stay
  correct under a scope filter rather than fetching everything and discarding rows in Go.
- `GET /api/v1/applications` — same, via a join from `applications` to its parent `consignments` row
  (`application.ApplicationStore.List`).
- `GET /api/v1/applications/{taskId}` — a single-row check in Go against the already-loaded parent
  consignment's `custom_data` (`ApplicationStore.GetByTaskID`/`GetByConsignmentAndTaskCode` preload
  `Consignment` for this). An out-of-scope `taskId` returns 404 (`ErrApplicationNotFound`), not 403 —
  it shouldn't be possible to tell an out-of-scope application even exists.
- `consignment.Service.GetConsignment` gets the same single-item check for parity, even though no
  route calls it today (`GET /api/v1/consignments/{id}` doesn't exist yet).
- `POST /api/v1/applications/{taskId}/claim` and `.../review` resolve scope
  (`service.checkScope`, the same helper `buildApplication` uses) before any other check — in
  particular, before the claim-ownership check, so an out-of-scope *unclaimed* application still
  404s rather than surfacing `ErrApplicationNotClaimedByYou` (403), which would otherwise leak that
  the record exists. `.../feedback` is covered transitively: it calls `GetApplication` first.
- `.../release` is the deliberate exception — it does **not** check scope. Scope can drift after a
  valid claim (a later task's `consignmentFields` push can overwrite the same target field; a
  deployer can edit the rules file), and there's no admin/force-release path in this codebase, so
  failing closed here would permanently strand a claim that's drifted out of scope: unclaimable by
  anyone else, unreviewable and unreleasable by the claimant. Releasing doesn't grant new access or
  leak anything the claimant doesn't already know, unlike Claim or Review — so it always succeeds
  for whoever currently holds the claim, regardless of their current scope.

## Example: NPQS

[`config/npqs/data-scope-rules.json`](../config/npqs/data-scope-rules.json) in this repo is a real,
manually-verified example: `{"consignmentField": "/officeLocation", "userField": "/assignedOffice"}`,
paired with an officer in [`data/seed/npqs_users.json`](../data/seed/npqs_users.json) seeded with
`assignedOffice: "Office 1"` (Katunayake). Submitting a consignment tagged to Office 1 makes it
visible to this officer; submitting one tagged to any other office makes it invisible to them — no
second officer account is needed to demonstrate both sides of the fail-closed check.

This file is **inert on its own** — nothing in this repo ever writes `/officeLocation` onto a
consignment's `custom_data`. That happens via a `consignmentFields` rule
(`{"source": "/nppo_office_location", "target": "/officeLocation"}`) on `npqs_application_review_v1`
in `OpenNSW/one-trade-artifacts` (task configs live there, not here — see
[consignment-custom-data.md](./consignment-custom-data.md#pushing-fields-from-a-task)), which must be
deployed alongside this file for NPQS officers to see anything at all: per the fail-closed semantics
above, a consignment with no `/officeLocation` set never matches, so every officer would otherwise see
zero results. The generic filtering mechanism itself (SQL predicate construction, pagination/total
correctness under a scope) is covered by this repo's own tests
(`TestConsignmentStore_List_ScopedByCustomData`, `TestApplicationStore_List_ScopedByConsignmentCustomData`)
independent of any specific field name.

## Limitations

- **No GIN index on `consignments.custom_data` yet.** `consignments` is a live, high(er)-write,
  request-path table (see [consignment-custom-data.md](./consignment-custom-data.md#storage)), so
  adding one wasn't bundled into this feature — it's the deliberate follow-up that migration's
  comment anticipated, now that `custom_data` sits in an actual `WHERE` clause rather than just
  being merged. Correct without it, just a full scan on Postgres at scale.
- **Equality only.** A rule can only assert "these two values are equal" — no ranges, prefixes, or
  containment. A richer predicate would need a different config shape and, likely, no longer being
  expressible as a flat SQL equality per rule.