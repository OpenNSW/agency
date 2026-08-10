import { describe, it, expect } from 'vitest'
import { formatDateForTable } from './date'

describe('formatDateForTable', () => {
  it('returns "-" when date string is undefined or empty', () => {
    expect(formatDateForTable()).toBe('-')
    expect(formatDateForTable('')).toBe('-')
  })

  it('formats valid ISO date string correctly', () => {
    const formatted = formatDateForTable('2026-08-10T10:00:00Z')
    expect(formatted).not.toBe('-')
    expect(formatted).toContain('2026')
  })
})
