import { NavLink, Outlet, useLocation, useNavigate } from 'react-router'
import { SelectField } from '@/components/ui/field'
import { cn } from '@/lib/cn'
import { can } from '@/lib/permissions'
import type { Permission } from '@/lib/types'
import type { MessageKey } from '@/i18n'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'

type Section = { to: string; label: MessageKey; allowed: (p: Permission[]) => boolean }

export const sections: Section[] = [
  { to: '/settings', label: 'settings.overview', allowed: () => true },
  { to: '/settings/users', label: 'settings.users', allowed: (p) => can(p, 'admin::users.read') },
  { to: '/settings/roles', label: 'settings.roles', allowed: (p) => can(p, 'admin::roles.read') },
  { to: '/settings/api-tokens', label: 'settings.apiTokens', allowed: (p) => can(p, 'admin::api-tokens.read') },
  { to: '/settings/webhooks', label: 'settings.webhooks', allowed: (p) => can(p, 'admin::webhooks.read') },
  { to: '/settings/users-permissions/roles', label: 'up.roles', allowed: (p) => can(p, 'plugin::users-permissions.roles.read') },
  { to: '/settings/users-permissions/advanced', label: 'up.advanced', allowed: (p) => can(p, 'plugin::users-permissions.advanced-settings.read') },
  { to: '/settings/users-permissions/email-templates', label: 'up.emails', allowed: (p) => can(p, 'plugin::users-permissions.email-templates.read') },
  { to: '/settings/users-permissions/providers', label: 'up.providers', allowed: (p) => can(p, 'plugin::users-permissions.providers.read') },
  { to: '/settings/email', label: 'settings.email', allowed: (p) => can(p, 'plugin::email.settings.read') },
  { to: '/settings/locales', label: 'settings.locales', allowed: (p) => can(p, 'plugin::i18n.locale.read') },
]

export function Settings() {
  const { t } = useI18n()
  const { permissions } = useAuth()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const visible = sections.filter((s) => s.allowed(permissions))
  const current = [...visible].reverse().find((s) => pathname === s.to || pathname.startsWith(`${s.to}/`))
  return (
    <div className="flex flex-col gap-6 lg:flex-row">
      <aside className="hidden w-60 shrink-0 lg:block">
        <h2 className="mb-3 text-lg font-semibold">{t('nav.settings')}</h2>
        <ul className="flex flex-col gap-0.5">
          {visible.map((s) => (
            <li key={s.to}>
              <NavLink to={s.to} end={s.to === '/settings'} className={({ isActive }) => cn('block rounded-md px-3 py-2 text-sm', isActive ? 'bg-primary/10 font-medium text-primary' : 'hover:bg-background')}>
                {t(s.label)}
              </NavLink>
            </li>
          ))}
        </ul>
      </aside>
      <div className="lg:hidden">
        <SelectField label={t('nav.settings')} value={current?.to ?? '/settings'} onChange={(e) => navigate(e.target.value)}>
          {visible.map((s) => (
            <option key={s.to} value={s.to}>
              {t(s.label)}
            </option>
          ))}
        </SelectField>
      </div>
      <div className="min-w-0 flex-1">
        <Outlet />
      </div>
    </div>
  )
}
