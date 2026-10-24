import { useEffect, useState } from 'react'
import { LogOut, Menu, UserRound, X } from 'lucide-react'
import { NavLink, Outlet, useLocation } from 'react-router'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/cn'
import { isEmbedded } from '@/lib/embed'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'
import { Logo } from './Logo'
import { navItems } from './nav'

function NavLinks({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useI18n()
  const { permissions, information } = useAuth()
  const link = ({ isActive }: { isActive: boolean }) =>
    cn('flex min-h-10 items-center gap-3 rounded-md px-3 text-sm font-medium', isActive ? 'bg-primary/10 text-primary' : 'text-foreground hover:bg-background')
  return (
    <nav className="flex flex-col gap-1">
      {navItems
        .filter((i) => i.allowed(permissions, information))
        .map((i) => (
          <NavLink key={i.to} to={i.to} end={i.to === '/'} className={link} onClick={onNavigate}>
            {i.icon}
            {t(i.label)}
          </NavLink>
        ))}
    </nav>
  )
}

function UserBlock({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useI18n()
  const { user, logout } = useAuth()
  return (
    <div className="flex flex-col gap-2 border-t border-border pt-4">
      <NavLink
        to="/me"
        onClick={onNavigate}
        className={({ isActive }) => cn('flex min-h-10 items-center gap-3 rounded-md px-3 text-sm', isActive ? 'bg-primary/10 text-primary' : 'hover:bg-background')}
      >
        <UserRound className="h-4 w-4 shrink-0" />
        <span className="min-w-0">
          <span className="block truncate font-medium">{[user?.firstname, user?.lastname].filter(Boolean).join(' ')}</span>
          <span className="block truncate text-xs text-muted">{user?.email}</span>
        </span>
      </NavLink>
      <Button variant="outline" title={t('nav.logout')} icon={<LogOut className="h-4 w-4" />} onClick={() => void logout()} className="w-full">
        {t('nav.logout')}
      </Button>
    </div>
  )
}

export function Layout() {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const location = useLocation()
  useEffect(() => setOpen(false), [location.pathname])
  if (isEmbedded(location.search)) {
    return (
      <main className="min-w-0 px-4 py-4">
        <Outlet />
      </main>
    )
  }
  return (
    <div className="min-h-full md:flex">
      <aside className="hidden w-64 shrink-0 flex-col justify-between border-r border-border bg-surface p-4 md:sticky md:top-0 md:flex md:h-screen">
        <div className="flex flex-col gap-6">
          <Logo />
          <NavLinks />
        </div>
        <UserBlock />
      </aside>

      <header className="sticky top-0 z-30 flex items-center justify-between border-b border-border bg-surface px-4 py-3 md:hidden">
        <Logo />
        <Button variant="outline" title={t('nav.menu')} icon={<Menu className="h-4 w-4" />} onClick={() => setOpen(true)} aria-expanded={open}>
          {t('nav.menu')}
        </Button>
      </header>
      {open && (
        <div className="fixed inset-0 z-40 md:hidden" role="dialog" aria-modal="true" aria-label={t('nav.menu')}>
          <div className="absolute inset-0 bg-black/40" onClick={() => setOpen(false)} />
          <div className="absolute inset-y-0 left-0 flex w-[85%] max-w-xs flex-col justify-between overflow-y-auto bg-surface p-4 shadow-xl">
            <div className="flex flex-col gap-6">
              <div className="flex items-center justify-between">
                <Logo />
                <button type="button" onClick={() => setOpen(false)} title={t('nav.close')} aria-label={t('nav.close')} className="rounded p-1 text-muted hover:text-foreground">
                  <X className="h-5 w-5" />
                </button>
              </div>
              <NavLinks onNavigate={() => setOpen(false)} />
            </div>
            <UserBlock onNavigate={() => setOpen(false)} />
          </div>
        </div>
      )}

      <main className="min-w-0 flex-1 px-4 py-6 md:px-8 md:py-8">
        <Outlet />
      </main>
    </div>
  )
}
