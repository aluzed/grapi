import en from './en.json'
import fr from './fr.json'

export type MessageKey = keyof typeof en
export type Language = 'en' | 'fr'

export const messages: Record<Language, Record<MessageKey, string>> = { en, fr }
export const languages: Language[] = ['en', 'fr']

export function isLanguage(v: unknown): v is Language {
  return typeof v === 'string' && (languages as string[]).includes(v)
}

export function format(template: string, params?: Record<string, string>): string {
  if (!params) return template
  return template.replace(/\{(\w+)\}/g, (m, k: string) => params[k] ?? m)
}
