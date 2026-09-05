import { z } from 'zod'

import { getBranding } from './runtimeConfig'

export let appConfig: UIConfig

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
      appConfig = result.data
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
  appConfig = {
    branding: {
      systemName: 'NSW',
      appName: 'NSW Agency Officer Portal',
      portalName: 'NSW Agency Portal',
      description: 'A unified digital platform enabling regulatory consignments.',
    },
  }
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
