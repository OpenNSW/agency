import { describe, it, expect } from 'vitest'
import en from './locales/en'
import si from './locales/si'

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function collectKeyPaths(obj: Record<string, unknown>, prefix = ''): string[] {
  return Object.entries(obj).flatMap(([key, value]) => {
    const path = prefix ? `${prefix}.${key}` : key
    return isPlainObject(value) ? collectKeyPaths(value, path) : [path]
  })
}

describe('locale key parity', () => {
  it('has the same translation keys in en and si', () => {
    const enKeys = new Set(collectKeyPaths(en))
    const siKeys = new Set(collectKeyPaths(si))

    const missingFromSi = [...enKeys].filter((key) => !siKeys.has(key))
    const missingFromEn = [...siKeys].filter((key) => !enKeys.has(key))

    expect(missingFromSi, 'keys present in en.ts but missing from si.ts').toEqual([])
    expect(missingFromEn, 'keys present in si.ts but missing from en.ts').toEqual([])
  })
})
