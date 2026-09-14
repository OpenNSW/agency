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

// One footer link — key resolves to a translated label (see
// frontend/src/components/Layout/Footer.tsx), url is where it routes.
export interface BrandingFooterLink {
  key: string
  url: string
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
  footerLinks?: BrandingFooterLink[]
  copyrightNotice?: string
}

// Mirrors backend/internal/web/config.go's I18nConfig — the shape served as
// window.__APP_CONFIG__.i18n (see getI18nConfig below).
export interface I18nPayload {
  supportedLanguages?: string[]
  defaultLanguage?: string
}

// window.__APP_CONFIG__'s shape, mirroring backend/internal/web/config.go's
// Config (Runtime/Branding/I18n, marshaled as-is — see Handler.ServeConfig):
// one object nesting runtime config, branding, and i18n under
// "runtime"/"branding"/"i18n", matching config.yaml's own
// web.runtime/web.branding/web.i18n, rather than separate window globals.
interface AppConfigPayload {
  runtime?: RuntimeConfigMap
  branding?: BrandingPayload
  i18n?: I18nPayload
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

// Reads window.__APP_CONFIG__.i18n, set by /config.js alongside runtime
// config and branding. Read directly (not through config.ts's appConfig)
// because src/i18n/index.ts must configure i18next before initAppConfig()
// runs — see main.tsx's import order. Returns undefined if unset, in which
// case src/i18n/index.ts falls back to every bundled language.
export function getI18nConfig(): I18nPayload | undefined {
  if (typeof window === 'undefined') {
    return undefined
  }

  return window.__APP_CONFIG__?.i18n
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
