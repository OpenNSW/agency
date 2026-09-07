type RuntimeConfigValue = string | undefined

type RuntimeConfigMap = Record<string, RuntimeConfigValue>

// Mirrors backend/internal/web/config.go's Branding/PartnerLogo — the shape
// served as window.__APP_CONFIG__.branding (see getBranding below). Kept as
// its own type rather than folded into RuntimeConfigMap since branding is a
// nested object with an array field, not a flat string map.
export interface BrandingPartnerLogo {
  url: string
  alt: string
}

export interface BrandingPayload {
  systemName: string
  appName: string
  logoUrl?: string
  systemLogoUrl?: string
  favicon?: string
  portalName?: string
  description?: string
  heroImageUrl?: string
  partnerLogos?: BrandingPartnerLogo[]
}

// window.__APP_CONFIG__'s shape, mirroring backend/internal/web/config.go's
// Config (Runtime/Branding, marshaled as-is — see Handler.ServeConfig): one
// object nesting runtime config and branding under "runtime"/"branding",
// matching config.yaml's own web.runtime/web.branding, rather than two
// separate window globals.
interface AppConfigPayload {
  runtime?: RuntimeConfigMap
  branding?: BrandingPayload
}

declare global {
  interface Window {
    __APP_CONFIG__?: AppConfigPayload
  }
}

function resolveRuntimeConfig(): RuntimeConfigMap {
  if (typeof window === 'undefined') {
    return {}
  }

  return window.__APP_CONFIG__?.runtime ?? {}
}

// Reads window.__APP_CONFIG__.branding, set by /config.js alongside runtime
// config (see backend/internal/web/handler.go's ServeConfig). Returns
// undefined if unset — config.ts falls back to its own hardcoded defaults in
// that case, same as when a branding fetch used to fail.
export function getBranding(): BrandingPayload | undefined {
  if (typeof window === 'undefined') {
    return undefined
  }

  return window.__APP_CONFIG__?.branding
}

export function getEnv(name: string, fallback?: string): string | undefined {
  const runtimeValue = resolveRuntimeConfig()[name]
  if (runtimeValue && runtimeValue.trim() !== '') {
    return runtimeValue
  }

  return fallback
}

export function getRequiredEnv(name: string): string {
  const value = getEnv(name)
  if (!value || value.trim() === '') {
    throw new Error(`Missing required environment variable: ${name}`)
  }

  return value
}

// Set by oidc-client-ts itself; overriding one corrupts the authorization request.
const RESERVED_AUTHORIZE_PARAMS = new Set([
  'client_id',
  'redirect_uri',
  'response_type',
  'scope',
  'state',
  'nonce',
  'code_challenge',
  'code_challenge_method',
])

/**
 * Reads a query-string-encoded variable into a parameter map, e.g.
 * `resource=https://api.example&prompt=consent`. Unset yields an empty map.
 */
export function getQueryParamsEnv(name: string): Record<string, string> {
  const raw = getEnv(name)
  if (!raw || raw.trim() === '') {
    return {}
  }

  const params: Record<string, string> = {}
  for (const [key, value] of new URLSearchParams(raw)) {
    if (RESERVED_AUTHORIZE_PARAMS.has(key)) {
      throw new Error(`${name}: "${key}" is set by the OIDC client and must not be overridden`)
    }
    params[key] = value
  }

  return params
}

export function getExpectedOuHandle(): string {
  return getRequiredEnv('VITE_IDP_EXPECTED_OU_HANDLE')
}
