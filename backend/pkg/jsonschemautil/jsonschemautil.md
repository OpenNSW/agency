# jsonschemautil

- **`ValidateInstance`** : validate submitted data against the schema.
- **`StripReadOnly`** : delete server-owned fields (marked `"readOnly": true`) from submitted
  data before it's persisted or acted on, so a client can't tamper with values it was only ever
  shown, not meant to set (e.g. IDs, timestamps, computed totals).

These two support **different amounts of the JSON Schema spec**
1. validation defers entirely to
a general-purpose library
2. read-only stripping is a small, hand-written walk over a
limited subset. This document is the source of truth for that subset.

## ValidateInstance

Backed by [`github.com/google/jsonschema-go`](https://github.com/google/jsonschema-go), which
supports both Draft-07 and 2020-12 — effectively the full spec, including `allOf`/`anyOf`/`oneOf`
and non-local `$ref`s.


## StripReadOnly

Unlike `ValidateInstance`, this does **not** use the library's schema resolver, it's a small
recursive walk that only understands the subset of JSON Schema below. It also doesn't look at
the schema's declared `type` at all; it decides whether to recurse into an object or an array
based on the *instance value's* actual shape at runtime.

### Supported keywords

| Keyword | Supported? | Notes |
| --- | --- | --- |
| `properties` | Yes | |
| `patternProperties` | Yes | Every matching pattern applies, not just the first one |
| `additionalProperties` (schema form) | Yes | Only used when a key matches neither `properties` nor any `patternProperties` pattern |
| `items` (schema form) | Yes | Applies uniformly to every array index |
| `prefixItems` | Yes |  `items` is the overflow schema past the prefix |
| `items` (array form) | Yes |A legacy tuple form; `additionalItems` is the overflow schema past the tuple |
| `additionalItems` | Yes | Only used as overflow past a legacy tuple `items` |
| `$ref` | Partial | Only `#/$defs/<name>` / `#/definitions/<name>`, resolved one level (see [$ref resolution](#ref-resolution) section) |
| `allOf`, `anyOf`, `oneOf`, `not` | No | Not read at all |
| Any other `$ref` (remote, nested-path, `$dynamicRef`) | No | Left unresolved — see [$ref resolution](#ref-resolution) |

### Property matching precedence

For each key in an object instance, in order:

1. If `properties` has an entry for that key, it applies.
2. Every `patternProperties` entry whose regex matches the key **also** applies (all of them,
   not just the first.) So in accordance with the spec, a patternProperty marked `readOnly` can strip a key of a non-readOnly `properties` entry which has a key that matches a `patternProperties` pattern.
3. `additionalProperties` applies **only if neither of the above matched**.

The key is deleted if **any** schema that applies to it (from steps 1–3) is `readOnly: true`.

### Array item precedence

For an array instance, the schema for the item at index `i`:

1. `prefixItems` present → `prefixItems[i]` while `i` is within its length, then `items` for
   anything past it.
2. Else `items` given as an array (legacy tuple) → `items[i]` while `i` is within its length,
   then `additionalItems` for anything past it.
3. Otherwise → `items` applies to every index.

Only object fields *inside* an array item can be stripped. There's no support for removing a
whole array element because the item schema itself is `readOnly`.

### $ref resolution

- Only a direct, local reference — `"$ref": "#/$defs/<name>"` or `"$ref": "#/definitions/<name>"`
  — is followed, and only **one level**: a `$ref` that points at another `$ref` is not chased
  further.
- A schema that has both `$ref` and its own object/array keywords (`properties`,
  `patternProperties`, `additionalProperties`, `items`, `prefixItems`, or the legacy tuple
  `items`/`additionalItems`) applies **both**: its own keywords, and — if the `$ref` resolves —
  the target's keywords, as if the two schemas were merged. This holds even when the `$ref` can't
  be resolved: the schema's own keywords still apply, only the target's are unavailable.
- Any other form of `$ref` (a remote URL, a nested JSON Pointer path, `$dynamicRef`, etc.) is left
  unresolved: nothing from the *target* gets stripped (since there's no target to read), but the
  field or item's own sibling keywords are still honored — including `readOnly` declared directly
  alongside the unresolved `$ref` (see next section).
- An unresolvable `$ref` inside the schema is **not an error** here, unlike `ValidateInstance`'s
  resolve step — `StripReadOnly` has no separate "resolve" phase to fail; it just treats that
  target as unsupported. **So current implementation expects already validated schemas**.

### readOnly and $ref

`"readOnly": true` is honored in two independent places; either one is enough to strip the
field:

```json
{ "$ref": "#/$defs/Address", "readOnly": true }
```

- **As a sibling of `$ref`**, like above — honored even if the `$ref` itself can't be resolved.
- **Declared on the `$ref` target itself** — the `$defs`/`definitions` entry being pointed to has
  `"readOnly": true` directly on it.

### Other behavior

- `rawSchema` nil/empty → `instance` is returned unchanged (matches `ValidateInstance`).
- `instance` nil → treated as `{}`; the returned map is a **new** map, not the original `nil`.
- Non-nil `instance` → the returned map is the *same* underlying map, mutated in place. Copy
  `instance` first if you need the pre-strip data for anything else (logging, comparison, etc.).
- A `patternProperties` pattern that fails to compile as a Go regular expression (RE2) — e.g. one
  using an ECMA-262-only lookahead, lookbehind, or backreference — is skipped and logged as a
  warning, not treated as an error. In practice this shouldn't happen: if `StripReadOnly` only
  ever runs on a schema that already passed `ValidateInstance`, that schema's patterns were
  already compiled successfully during `ValidateInstance`'s resolve step.
- A malformed (unparsable) `rawSchema` returns an error wrapped in `ErrSchemaLoad`.
