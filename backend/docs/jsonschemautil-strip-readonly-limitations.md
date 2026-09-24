# `StripReadOnly` limitations

`StripReadOnly` (`pkg/jsonschemautil/strip.go`) is a deliberately narrow, non-recursive-spec
implementation of "delete every `readOnly` field from an instance" — it does not implement
general JSON Schema evaluation. This documents the concrete gaps between what it does and what
the [JSON Schema](https://json-schema.org/) spec allows, each confirmed by exercising the
current code rather than assumed from reading it.

Everything below is about the current, real behavior of `strip.go`. Fields of
`jsonschema.Schema` other than `Properties`, `PatternProperties`, `AdditionalProperties`,
`Items`, `PrefixItems`, `ItemsArray`, `AdditionalItems`, `Ref`, `ReadOnly`, `Defs`, and
`Definitions` are never read by this package — anything expressed only through another keyword is
invisible to it.

A `"$ref"` alongside sibling keywords (e.g. a `"properties"` entry declared next to `"$ref"`) is
handled correctly: JSON Schema 2020-12 applies `"$ref"` like any other keyword, so both the
sibling schema and the resolved target are walked, and `readOnly` in either is honored.

## `$ref` chains longer than two hops silently lose the rest of the subtree

`resolveRef` follows exactly one `"#/$defs/<name>"` / `"#/definitions/<name>"` hop and returns
whatever it finds — even if that's itself another bare `$ref`. Each *instance*
property/array-item traversal performs at most two such hops (one to resolve the property's own
schema before recursing; one more at the top of `stripValue`, before descending into that value).
A `$defs` chain of `A -> B` (two hops) still gets fully resolved this way, but a chain of three or
more (`A -> B -> C`) is not: the third hop is never taken, so recursion uses a schema object that
is still just `{"$ref": "C"}` — which has no `properties`/`items` of its own — and the entire
nested subtree passes through unexamined, `readOnly` fields included:

```jsonc
{
  "properties": { "foo": { "$ref": "#/$defs/A" } },
  "$defs": {
    "A": { "$ref": "#/$defs/B" },
    "B": { "$ref": "#/$defs/C" },
    "C": { "type": "object", "properties": {
      "secret": { "type": "string", "readOnly": true }
    }}
  }
}
```

Confirmed by running it: with `foo: {"secret": "x"}`, `secret` survives. A two-hop version of the
same schema (only `A -> B`) strips `secret` correctly, so the break specifically happens at the
third hop, not "any indirection."

Separately, even within the two-hop budget, `readOnly` is only ever checked on the immediate
schema and its one-hop-resolved target — never on whatever a *second* hop resolves to. So
`readOnly: true` sitting on the final target of a two-hop chain is missed even though recursion
into that target's own nested properties (a third, independent concern) works fine:

```jsonc
{
  "properties": { "foo": { "$ref": "#/$defs/A" } },
  "$defs": {
    "A": { "$ref": "#/$defs/B" },
    "B": { "type": "object", "readOnly": true, "properties": { "note": { "type": "string" } } }
  }
}
```

Confirmed by running it: `foo` is not deleted, even though `B` (two hops from `foo`) is marked
`readOnly`.

## Non-`$defs`/`definitions` refs are never followed

Only a direct `"#/$defs/<name>"` or `"#/definitions/<name>"` pointer is resolved. Anything else —
a nested JSON Pointer path (`"#/$defs/Foo/properties/bar"`), a remote ref, or a ref by `$id`/
anchor — resolves to `nil` and the subtree underneath it is left completely untouched, exactly as
if no schema were configured for it at all (`TestStripReadOnly_UnsupportedRefIsSkipped` covers the
remote-ref case).

## `allOf`/`anyOf`/`oneOf` are never evaluated

These composition keywords aren't inspected at all, not even partially. A `readOnly` field
declared only inside one of them is completely invisible to `StripReadOnly`:

```jsonc
{
  "type": "object",
  "allOf": [ { "properties": { "secret": { "type": "string", "readOnly": true } } } ]
}
```

Confirmed by running it (and the `anyOf` equivalent): `{"secret": "spoofed"}` passes through
unchanged. This codebase's own forms only use `oneOf` for scalar `const` enumerations (e.g.
`{"oneOf": [{"const": "A"}, {"const": "B"}]}`), which have no nested `properties`/`readOnly` to
miss and so aren't affected — but any schema that uses these keywords to compose object shapes
would silently leak readOnly data through this gap.

## `readOnly` not attached to a named object property is a no-op

Stripping only ever does `delete(obj, name)` on a map key it reached by walking `properties` /
`patternProperties` / `additionalProperties`. It has no mechanism to redact a bare value or drop
an array element, so `readOnly` declared anywhere else is silently ignored:

- On an `items` schema (every element of an array, not a field within each element), whether the
  items are scalars or objects:
  ```jsonc
  { "properties": { "tags": { "type": "array", "items": { "type": "string", "readOnly": true } } } }
  ```
  Confirmed by running it: `{"tags": ["a", "b", "c"]}` comes back unchanged, and the same holds
  when `items` describes an object with `readOnly: true` on the object itself rather than one of
  its fields — there is no containing key to delete an array element under.

- On the root schema itself (the whole instance is meant to be server-authoritative):
  ```jsonc
  { "type": "object", "readOnly": true, "properties": { "note": { "type": "string" } } }
  ```
  Confirmed by running it: the instance is returned unchanged. `StripReadOnly` enters recursion
  via `stripValue(&sch, instance)` directly, with no parent key to delete `instance` under, so a
  root-level `readOnly` can never take effect. (A catch-all `"patternProperties": {".*": {"readOnly": true}}`
  is a working substitute — confirmed by running it — since it still deletes each matched
  top-level key individually.)

`readOnly` on a property whose *value* happens to be an object still works normally (the object's
own nested properties are walked as usual) — the gap is specifically "readOnly with no enclosing
named property to delete."

## `properties`/`patternProperties` union is exact only up to `$ref` depth

The readOnly check itself correctly unions every applicable schema for a property name — an
explicit `properties` entry and every matching `patternProperties` regexp are all consulted, and
the property is stripped if *any* of them (or their one-hop `$ref` target) says `readOnly: true`.
This is not order-dependent even when several `patternProperties` patterns match the same name.
This part matches spec semantics (verified against `jsonschema-go`'s own validator, which applies
`properties` and `patternProperties` the same way). The residual gap is purely the `$ref`-depth
limit described above: the union is only as deep as the two hops `resolveRef` performs per
instance-tree step.

## Not coupled to `ValidateInstance`

`StripReadOnly` parses `rawSchema` itself via `json.Unmarshal` and lazily compiles
`patternProperties` regexps itself; it does not call `ValidateInstance` or reuse
`jsonschema.Schema.Resolve()` even when both are handed the same raw schema. Nothing enforces
that `StripReadOnly` only ever runs after validation has already succeeded on the identical
schema — that's a convention callers must uphold themselves.
