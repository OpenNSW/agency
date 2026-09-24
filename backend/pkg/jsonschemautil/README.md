# jsonschemautil

- **`ValidateInstance`** : validate an instance against a JSON Schema.
- **`StripReadOnly`** : delete every field the schema marks `"readOnly": true` from the instance.

## StripReadOnly

A small recursive walk that only understands the subset of JSON Schema below. It doesn't look at
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
| `$ref` | Partial | Only `#/$defs/<name>` / `#/definitions/<name>`, resolved one level (see [$ref](#ref) section) |
| `allOf`, `anyOf`, `oneOf`, `not` | No | Not read at all |

### Property matching precedence

For each key in an object instance, in order:

1. If `properties` has an entry for that key, it applies.
2. Every `patternProperties` entry whose regex matches the key **also** applies (all of them,
   not just the first).
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

### $ref

- Only a direct, local reference — `"$ref": "#/$defs/<name>"` or `"$ref": "#/definitions/<name>"`
  — is followed, and only **one level**: a `$ref` that points at another `$ref` is not chased
  further. Any other form (a remote URL, a nested JSON Pointer path, `$dynamicRef`, etc.) is left
  unresolved, which is not an error.
- A schema's own keywords (including `readOnly`) always apply alongside its `$ref`; if the `$ref`
  resolves, the target's keywords apply too, as if the two schemas were merged. So a field is
  stripped if `"readOnly": true` is either a sibling of the `$ref` (even an unresolvable one) or
  declared on the `$defs`/`definitions` target itself:

  ```json
  { "$ref": "#/$defs/Address", "readOnly": true }
  ```

### Other behavior

- `rawSchema` nil/empty → `instance` is returned unchanged.
- `instance` nil → treated as `{}`; the returned map is a **new** map, not the original `nil`.
- Non-nil `instance` → the returned map is the *same* underlying map, mutated in place. Copy
  `instance` first if you need the pre-strip data for anything else (logging, comparison, etc.).
- A `patternProperties` pattern that fails to compile as a Go regular expression (RE2) — e.g. one
  using an ECMA-262-only lookahead, lookbehind, or backreference — is skipped and logged as a
  warning, not treated as an error.
- A malformed (unparsable) `rawSchema` returns an error wrapped in `ErrSchemaLoad`.
