export const locales = ['en', 'ru', 'de', 'es', 'fr', 'zh-Hans'] as const

export type Locale = (typeof locales)[number]

export const defaultLocale: Locale = 'en'

export const localeLabels: Record<Locale, { code: string; native: string }> = {
  en: { code: 'EN', native: 'English' },
  ru: { code: 'RU', native: 'Русский' },
  de: { code: 'DE', native: 'Deutsch' },
  es: { code: 'ES', native: 'Español' },
  fr: { code: 'FR', native: 'Français' },
  'zh-Hans': { code: '中文', native: '简体中文' },
}

export const openGraphLocales: Record<Locale, string> = {
  en: 'en_US',
  ru: 'ru_RU',
  de: 'de_DE',
  es: 'es_ES',
  fr: 'fr_FR',
  'zh-Hans': 'zh_CN',
}

export function localePath(locale: Locale): string {
  return locale === defaultLocale ? '/' : `/${locale}/`
}
