import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api, onUnauthorized, publicRequest, refresh, setAccessToken } from '@/lib/api'
import { can as canDo } from '@/lib/permissions'
import type { AdminUser, Information, Permission } from '@/lib/types'
import { isLanguage } from '@/i18n'
import { useI18n } from './I18nProvider'

type Status = 'loading' | 'anonymous' | 'authenticated'

export type Logos = { menuLogo: string | null; authLogo: string | null }

type Auth = {
  status: Status
  hasAdmin: boolean
  logos: Logos
  // reloadLogos reads the project's logos again, after they changed.
  reloadLogos: () => Promise<void>
  user: AdminUser | null
  permissions: Permission[]
  information: Information | null
  can: (action: string, subject?: string) => boolean
  // signIn takes the {token, user} answer of a login-like route.
  signIn: (session: { token: string }) => Promise<void>
  logout: () => Promise<void>
  reload: () => Promise<void>
}

const AuthContext = createContext<Auth | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const { setLanguage } = useI18n()
  const [status, setStatus] = useState<Status>('loading')
  const [hasAdmin, setHasAdmin] = useState(true)
  const [logos, setLogos] = useState<Logos>({ menuLogo: null, authLogo: null })
  const reloadLogos = useCallback(async () => {
    const init = await publicRequest<Logos>('GET', '/auth/init')
    setLogos({ menuLogo: init.menuLogo ?? null, authLogo: init.authLogo ?? null })
  }, [])
  const [user, setUser] = useState<AdminUser | null>(null)
  const [permissions, setPermissions] = useState<Permission[]>([])
  const [information, setInformation] = useState<Information | null>(null)

  const anonymous = useCallback(() => {
    setAccessToken(null)
    setUser(null)
    setPermissions([])
    setStatus('anonymous')
    queryClient.clear()
  }, [queryClient])

  const load = useCallback(async () => {
    const [me, perms, info] = await Promise.all([
      api.get<AdminUser>('/users/me'),
      api.get<Permission[]>('/users/me/permissions'),
      api.get<Information>('/information'),
    ])
    setUser(me)
    setPermissions(perms)
    setInformation(info)
    if (isLanguage(me.preferedLanguage)) setLanguage(me.preferedLanguage)
    setStatus('authenticated')
  }, [setLanguage])

  useEffect(() => {
    onUnauthorized(anonymous)
    let cancelled = false
    ;(async () => {
      try {
        const init = await publicRequest<{ hasAdmin: boolean } & Logos>('GET', '/auth/init')
        if (cancelled) return
        setHasAdmin(init.hasAdmin)
        setLogos({ menuLogo: init.menuLogo ?? null, authLogo: init.authLogo ?? null })
        if (init.hasAdmin && (await refresh())) {
          await load()
          return
        }
      } catch {
        // the login page shows instead
      }
      if (!cancelled) setStatus('anonymous')
    })()
    return () => {
      cancelled = true
    }
  }, [anonymous, load])

  const signIn = useCallback(
    async ({ token }: { token: string }) => {
      setAccessToken(token)
      setHasAdmin(true)
      await load()
    },
    [load],
  )

  const logout = useCallback(async () => {
    try {
      await publicRequest('POST', '/auth/logout', {})
    } finally {
      anonymous()
    }
  }, [anonymous])

  const value = useMemo<Auth>(
    () => ({
      status,
      hasAdmin,
      logos,
      reloadLogos,
      user,
      permissions,
      information,
      can: (action, subject) => canDo(permissions, action, subject),
      signIn,
      logout,
      reload: load,
    }),
    [status, hasAdmin, logos, reloadLogos, user, permissions, information, signIn, logout, load],
  )
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): Auth {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth outside AuthProvider')
  return ctx
}
