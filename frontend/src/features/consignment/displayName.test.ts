import { describe, expect, it } from 'vitest'
import { officerDisplayName } from './displayName'

describe('officerDisplayName', () => {
  it('keeps real names', () => {
    expect(officerDisplayName('Stay Naturals Private Limited')).toBe('Stay Naturals Private Limited')
    expect(officerDisplayName('  Acme Imports Ltd  ')).toBe('Acme Imports Ltd')
  })

  it('drops blanks and placeholders', () => {
    expect(officerDisplayName(undefined)).toBeUndefined()
    expect(officerDisplayName('')).toBeUndefined()
    expect(officerDisplayName('N/A')).toBeUndefined()
    expect(officerDisplayName('n/a')).toBeUndefined()
    expect(officerDisplayName('-')).toBeUndefined()
  })
})
