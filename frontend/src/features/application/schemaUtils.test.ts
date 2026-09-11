import { describe, expect, it } from 'vitest'
import { capitalizeSchemaOptions } from './schemaUtils'

describe('capitalizeSchemaOptions', () => {
  it('capitalizes a top-level enum into a title-cased oneOf', () => {
    const props: Record<string, unknown> = {
      status: { enum: ['needs_more_info', 'approved'] },
    }

    capitalizeSchemaOptions(props)

    expect(props.status).toEqual({
      oneOf: [
        { const: 'needs_more_info', title: 'Needs More Info' },
        { const: 'approved', title: 'Approved' },
      ],
    })
  })

  it('recurses into array item properties (items.properties)', () => {
    const props: Record<string, unknown> = {
      commodities: {
        items: {
          properties: {
            commodity_condition: { enum: ['fresh_cut'] },
          },
        },
      },
    }

    capitalizeSchemaOptions(props)

    const items = (props.commodities as { items: { properties: Record<string, unknown> } }).items
    expect(items.properties.commodity_condition).toEqual({
      oneOf: [{ const: 'fresh_cut', title: 'Fresh Cut' }],
    })
  })

  // Regression: a plain nested object property (not wrapped in an array's
  // `items`) was previously skipped entirely, so an enum inside it never
  // received capitalized oneOf labels — see the x-search {value, label}
  // objects introduced for commodity_common_name/importing_country.
  it('recurses into plain nested object properties (properties.properties)', () => {
    const props: Record<string, unknown> = {
      commodity_common_name: {
        type: 'object',
        properties: {
          value: { type: 'string' },
          source: { enum: ['manual_entry'] },
        },
      },
    }

    capitalizeSchemaOptions(props)

    const nested = (props.commodity_common_name as { properties: Record<string, unknown> }).properties
    expect(nested.source).toEqual({
      oneOf: [{ const: 'manual_entry', title: 'Manual Entry' }],
    })
  })
})
