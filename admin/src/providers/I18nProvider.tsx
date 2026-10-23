import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { format, isLanguage, messages, type Language, type MessageKey } from '@/i18n'

type I18n = {
  language: Language
  setLanguage: (l: Language) => void
  t: (key: MessageKey, params?: Record<string, string>) => string
}

const I18nContext = createContext<I18n | null>(null)

const storageKey = 'grapi.language'

function initialLanguage(): Language {
  try {
    const stored = localStorage.getItem(storageKey)
    if (isLanguage(stored)) return stored
  } catch {
    // storage may be unavailable
  }
  return navigator.language.toLowerCase().startsWith('fr') ? 'fr' : 'en'
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [language, setLang] = useState<Language>(initialLanguage)
  const setLanguage = useCallback((l: Language) => {
    setLang(l)
    document.documentElement.lang = l
    try {
      localStorage.setItem(storageKey, l)
    } catch {
      // storage may be unavailable
    }
  }, [])
  const value = useMemo<I18n>(
    () => ({ language, setLanguage, t: (key, params) => format(messages[language][key] ?? key, params) }),
    [language, setLanguage],
  )
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

export function useI18n(): I18n {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useI18n outside I18nProvider')
  return ctx
}
