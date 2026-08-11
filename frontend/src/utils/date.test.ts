import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { formatDateForTable } from './date'

vi.mock('@/i18n', () => ({
  default: {
    resolvedLanguage: 'en-US',
  },
}))

describe('formatDateForTable', () => {
  beforeEach(() => {
    vi.stubEnv('TZ', 'UTC')
  })

  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('returns "-" when date string is undefined or empty', () => {
    expect(formatDateForTable()).toBe('-')
    expect(formatDateForTable('')).toBe('-')
  })

  it('formats valid ISO date string correctly', () => {
    const formatted = formatDateForTable('2026-08-10T10:00:00Z')
    expect(formatted).toBe('Aug 10, 2026')
    expect(formatted).toContain('Aug')
    expect(formatted).toContain('10')
    expect(formatted).toContain('2026')
  })
})
