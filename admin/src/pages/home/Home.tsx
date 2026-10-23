import { ArrowRight } from 'lucide-react'
import { Link } from 'react-router'
import { navItems } from '@/app/nav'
import { PageHeader } from '@/app/PageHeader'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'

export function Home() {
  const { t } = useI18n()
  const { user, permissions, information } = useAuth()
  const shortcuts = navItems.filter((i) => i.to !== '/' && i.allowed(permissions, information))
  return (
    <div className="mx-auto max-w-5xl">
      <PageHeader title={t('home.title', { name: user?.firstname ?? '' })} subtitle={t('home.subtitle')} />
      {shortcuts.length > 0 && (
        <section>
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted">{t('home.shortcuts')}</h2>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {shortcuts.map((s) => (
              <Link
                key={s.to}
                to={s.to}
                className="flex items-center justify-between gap-3 rounded-lg border border-border bg-surface p-4 shadow-sm hover:border-primary/40"
              >
                <span className="flex items-center gap-3 font-medium">
                  {s.icon}
                  {t(s.label)}
                </span>
                <ArrowRight className="h-4 w-4 text-muted" aria-hidden />
              </Link>
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
