import { type SchemaProperty } from './types'

function capitalizeOptions(prop: SchemaProperty) {
  if (prop.oneOf) {
    prop.oneOf = prop.oneOf.map((opt) => {
      const titleVal = opt.title || String(opt.const)
      const formattedTitle = titleVal
        .split(/[_\s]+/)
        .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
        .join(' ')
      return { ...opt, title: formattedTitle }
    })
  } else if (prop.enum) {
    prop.oneOf = prop.enum.map((val: string) => {
      const title = val
        .split(/[_\s]+/)
        .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
        .join(' ')
      return { const: val, title }
    })
    delete prop.enum
  }
}

// Recursively capitalizes oneOf/enum option titles throughout a JSON Schema
// properties map, mutating in place. Descends into both array item schemas
// (properties.items.properties) and plain nested object schemas
// (properties.properties) — a schema can nest either shape arbitrarily
// deep, so both branches need to recurse, not just one.
export function capitalizeSchemaOptions(props: Record<string, unknown>) {
  Object.values(props).forEach((prop) => {
    capitalizeOptions(prop as SchemaProperty)
    const { items, properties } = prop as {
      items?: { properties?: Record<string, unknown> }
      properties?: Record<string, unknown>
    }
    if (items?.properties) {
      capitalizeSchemaOptions(items.properties)
    }
    if (properties) {
      capitalizeSchemaOptions(properties)
    }
  })
}
