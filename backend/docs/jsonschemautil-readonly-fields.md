# Making a field readOnly

`StripReadOnly` (`pkg/jsonschemautil`) deletes any instance field its schema marks
`"readOnly": true` before the data is written back — the standard way to protect a
server-authoritative field (e.g. a generated ID) from being overwritten by a client
resubmitting the form. It implements a deliberately narrow subset of JSON Schema; the rules
below are written around its actual limitations (see
[jsonschemautil-strip-readonly-limitations.md](./jsonschemautil-strip-readonly-limitations.md)
for the "why").

## The rule that always works

Put `"readOnly": true` directly on the schema entry for that exact field, inside the
`"properties"` (or `"patternProperties"`) of the object that directly contains it:

```jsonc
{
  "type": "object",
  "properties": {
    "refId": { "type": "string", "readOnly": true }
  }
}
```

This works at any nesting depth — just repeat it inside each nested object's own
`"properties"`.

## Do

- **Nested objects**: mark the field readOnly inside the nested object's own `"properties"`,
  the same as at the top level.
- **Whole nested object is server-owned**: put `"readOnly": true` directly on *that property's*
  schema entry (the one the parent object's `"properties"` points at), not on some other schema
  reached indirectly.
- **Shared/reusable schemas via `$ref`**: point `"$ref"` straight at a `$defs`/`definitions`
  entry that is the *full* schema — not at another `$ref`. One hop only.
- **Extending a shared `$ref` inline**: add extra, form-specific fields via a sibling
  `"properties"` next to `"$ref"` — both apply:
  ```jsonc
  "foo": {
    "$ref": "#/$defs/Shared",
    "properties": { "secret": { "type": "string", "readOnly": true } }
  }
  ```
- **Lock every top-level field** (since the schema root itself can't be marked readOnly — see
  Don't): use a catch-all `patternProperties` instead:
  ```jsonc
  { "type": "object", "patternProperties": { ".*": { "readOnly": true } } }
  ```

## Don't — confirmed not stripped

- **The root schema.** `{"type": "object", "readOnly": true, ...}` has no effect; the whole
  instance is returned unchanged. Use the catch-all `patternProperties` trick above instead.
- **An array's `items` schema**, to drop whole elements. `"items": {"readOnly": true}` is a
  no-op whether items are scalars or objects — array elements are never deleted. Put
  `"readOnly": true` on a named field *inside* the item object instead.
- **A `$ref` chain two levels deep**, where an intermediate `$defs` entry is itself just
  another `"$ref"`. A single hop (`property -> $ref -> full schema`) works fully; a second hop
  still recurses into nested fields but misses `readOnly` declared on that far target itself;
  a third hop loses the subtree entirely. Always point `"$ref"` straight at the complete schema.
- **`allOf` / `anyOf` / `oneOf`.** These are never evaluated, so a `readOnly` field declared only
  inside one is invisible. Declare it directly under the object's own `"properties"` /
  `"patternProperties"` instead. (Using `oneOf` purely for a scalar `const` enum is unaffected —
  there's no nested `readOnly` there to miss.)
- **A remote ref or a nested JSON Pointer** (e.g. `"#/$defs/Foo/properties/bar"`). Only a direct
  `"#/$defs/<name>"` or `"#/definitions/<name>"` pointer is followed.
