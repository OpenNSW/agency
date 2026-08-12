import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import en from './locales/en'
import si from './locales/si'

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      si: { translation: si },
      en: { translation: en },
    },
    fallbackLng: 'en',
    supportedLngs: ['en', 'si'],
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
    },
    interpolation: {
      escapeValue: false,
    },
  })

export default i18n
