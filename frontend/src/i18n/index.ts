import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import en from './locales/en'
import si from './locales/si'
import { getI18nConfig } from '@/runtimeConfig'

const resources = {
  en: { translation: en },
  si: { translation: si },
}

// This app isn't specific to any one country, so which languages a
// deployment offers isn't hardcoded here — add a new language by creating
// <lang>.ts, registering it in `resources` and `languageLabels` above, then
// listing its code in web.i18n.supportedLanguages (see
// backend/internal/web/config.go's I18nConfig).
export const languageLabels: Record<string, string> = {
  en: 'English',
  si: 'සිංහල',
}

const bundledLanguages = Object.keys(resources)

// Config values are read directly off window.__APP_CONFIG__ (not through
// config.ts's appConfig) because this must run before initAppConfig() does —
// see main.tsx's import order. Anything configured that isn't actually
// bundled above is dropped rather than trusted, since a bad config value
// here would otherwise silently break the whole app before React mounts.
const configuredI18n = getI18nConfig()
const configuredSupported = (configuredI18n?.supportedLanguages ?? []).filter((lang) => bundledLanguages.includes(lang))
export const supportedLanguages = configuredSupported.length > 0 ? configuredSupported : bundledLanguages

const fallbackLng =
  configuredI18n?.defaultLanguage && supportedLanguages.includes(configuredI18n.defaultLanguage)
    ? configuredI18n.defaultLanguage
    : 'en'

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng,
    supportedLngs: supportedLanguages,
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
    },
    interpolation: {
      escapeValue: false,
    },
  })

export default i18n
