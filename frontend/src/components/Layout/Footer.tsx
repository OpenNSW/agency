import { useTranslation } from 'react-i18next'
import { appConfig } from '@/config'

// Deliberately tiny — a thin white bar, not a full government footer. Fixed
// full-width across the bottom of the viewport (z-30: above Sidebar's z-20,
// below TopBar's z-50 — see those components), so it's the same one instance
// regardless of what's rendered above it — see Layout.tsx and LoginScreen.tsx.
export function Footer() {
  const { t } = useTranslation()
  const footerLinks = appConfig.branding.footerLinks ?? []
  const version = import.meta.env.VITE_APP_VERSION || 'dev'

  // Only a footerLinks entry whose key matches one of this fixed, known set
  // is rendered, with its label translated here rather than carried in
  // config (see backend/internal/web/config.go's FooterLink). url is always
  // an absolute URL — these are not routes in this app.
  function footerLinkLabel(key: string): string | null {
    switch (key) {
      case 'policy':
        return t('footer.links.policy')
      case 'accessibility':
        return t('footer.links.accessibility')
      case 'support':
        return t('footer.links.support')
      default:
        return null
    }
  }

  return (
    <footer className="fixed inset-x-0 bottom-0 z-30 flex flex-col items-center justify-center gap-1 border-t border-gray-200 bg-white px-4 py-1.5 text-xs sm:h-8 sm:flex-row sm:justify-between sm:gap-4 sm:px-6 sm:py-0">
      <div className="flex flex-wrap items-center justify-center gap-x-4 gap-y-1">
        {footerLinks.map((link) => {
          const label = footerLinkLabel(link.key)
          if (!label) return null

          return (
            <a
              key={link.key}
              href={link.url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-gray-500 hover:text-gray-700 hover:underline"
            >
              {label}
            </a>
          )
        })}
      </div>
      <div className="flex items-center gap-4 text-gray-700">
        <a
          href="https://github.com/OpenNSW"
          target="_blank"
          rel="noopener noreferrer"
          className="hover:text-gray-900 hover:underline"
        >
          {t('footer.poweredBy')}
        </a>
        <span>{version}</span>
      </div>
    </footer>
  )
}
