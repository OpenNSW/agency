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

  it('returns "-" when date string is missing', () => {
    expect(formatDateForTable()).toBe('-')
    expect(formatDateForTable('')).toBe('-')
  })

  it('formats a valid ISO date for table display', () => {
    expect(formatDateForTable('2026-08-10T10:00:00Z')).toBe('Aug 10, 2026')
  })
})
