import { z } from 'zod'

import { getBranding } from './runtimeConfig'

export let appConfig: UIConfig

// Emergency fallback branding, used both when window.__APP_CONFIG__.branding
// is entirely missing/invalid, and (portalName/description only) to fill in
// for a deployment whose web.branding sets just the required
// systemName/appName — TopBar renders portalName and LoginScreen renders
// description unconditionally, with no fallback of their own, so a valid but
// partial payload must not reach them as blank.
const DEFAULT_BRANDING = {
  systemName: 'NSW',
  appName: 'NSW Agency Officer Portal',
  portalName: 'NSW Agency Portal',
  description: 'A unified digital platform enabling regulatory consignments.',
}

// Branding used to be fetched from a separate, per-agency
// /configs/<name>.branding.json static file; that mechanism never actually
// reached production (see backend/internal/web/config.go's Branding doc
// comment), so it now comes from window.__APP_CONFIG__.branding instead — the
// same /config.js response that already carries the rest of runtime config,
// loaded before the app bundle. That makes this synchronous now (no more
// fetch to await); a missing/invalid payload still degrades to the same
// hardcoded emergency fallback a failed fetch used to.
export function initAppConfig(): void {
  const branding = getBranding()
  if (branding) {
    const result = UIConfigSchema.safeParse({ branding })
    if (result.success) {
      appConfig = {
        ...result.data,
        branding: {
          ...result.data.branding,
          portalName: result.data.branding.portalName || DEFAULT_BRANDING.portalName,
          description: result.data.branding.description || DEFAULT_BRANDING.description,
        },
      }
      return
    }
    console.error(
      '[Config] Invalid branding from window.__APP_CONFIG__.branding:',
      result.error.issues.map((i) => `${i.path.join('.')}: ${i.message}`).join('\n'),
    )
  } else {
    console.warn('[Config] window.__APP_CONFIG__.branding is missing, falling back to hardcoded defaults...')
  }

  // Provide a hardcoded emergency config as a final safety fallback to keep the app working
  appConfig = { branding: DEFAULT_BRANDING }
}

const UIConfigSchema = z.object({
  branding: z.object({
    systemName: z.string().min(1),
    appName: z.string().min(1),
    logoUrl: z.string().optional(),
    systemLogoUrl: z.string().optional(),
    favicon: z.string().optional(),
    portalName: z.string().optional(),
    description: z.string().optional(),
    heroImageUrl: z.string().optional(),
    partnerLogos: z.array(z.object({ url: z.string(), alt: z.string() })).optional(),
    footerLinks: z.array(z.object({ key: z.string(), url: z.string() })).optional(),
  }),
  theme: z
    .object({
      fontFamily: z.string(),
      borderRadius: z.string(),
    })
    .optional(),
  features: z
    .object({
      preConsignment: z.boolean(),
      consignmentManagement: z.boolean(),
      reportingDashboard: z.boolean(),
    })
    .optional(),
})

export type UIConfig = z.infer<typeof UIConfigSchema>
