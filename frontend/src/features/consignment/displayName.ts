const INVALID_NAMES = new Set([
  'n/a',
  'n.a.',
  'n.a',
  'na',
  'nil',
  'null',
  'undefined',
  'unknown',
  '-',
  '--',
  'none',
  'not applicable',
])

/** Returns a real officer-facing name, or undefined for blanks and placeholders. */
export function officerDisplayName(value?: string | null): string | undefined {
  const name = value?.trim()
  if (!name) return undefined
  if (INVALID_NAMES.has(name.toLowerCase())) return undefined
  return name
}
