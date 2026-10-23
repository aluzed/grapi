import { useQuery } from '@tanstack/react-query'
import { ArrowRight } from 'lucide-react'
import { Link } from 'react-router'
import { navItems } from '@/app/nav'
import { PageHeader } from '@/app/PageHeader'
import { Card, CardHeader } from '@/components/ui/card'
import { api } from '@/lib/api'
import { can } from '@/lib/permissions'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'

type Recent = { uid: string; kind: string; displayName: string; documentId: string; title: string; locale: string | null; at: string }
type Homepage = { counts: { uid: string; kind: string; displayName: string; total: number }[]; lastEdited: Recent[]; lastPublished: Recent[] }

const editPath = (r: Recent) =>
  r.kind === 'singleType'
    ? `/content-manager/single-types/${encodeURIComponent(r.uid)}`
    : `/content-manager/collection-types/${encodeURIComponent(r.uid)}/${encodeURIComponent(r.documentId)}${r.locale ? `?locale=${r.locale}` : ''}`

function RecentList({ title, items }: { title: string; items: Recent[] }) {
  const { t, language } = useI18n()
  return (
    <Card>
      <CardHeader title={title} />
      {items.length === 0 ? (
        <p className="text-sm text-muted">{t('home.nothing')}</p>
      ) : (
        <ul className="flex flex-col divide-y divide-border">
          {items.map((r) => (
            <li key={`${r.uid}/${r.documentId}/${r.locale}`} className="py-2">
              <Link to={editPath(r)} className="block truncate text-sm font-medium hover:text-primary">
                {r.title || r.documentId}
              </Link>
              <span className="block text-xs text-muted">
                {r.displayName}
                {r.locale ? ` · ${r.locale}` : ''} · {r.at ? new Date(r.at).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'short' }) : '-'}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}

export function Home() {
  const { t } = useI18n()
  const { user, permissions, information } = useAuth()
  const shortcuts = navItems.filter((i) => i.to !== '/' && i.allowed(permissions, information))
  const home = useQuery({ queryKey: ['homepage'], queryFn: () => api.get<Homepage>('/homepage'), enabled: can(permissions, 'plugin::content-manager.explorer.read') })
  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6">
      <PageHeader title={t('home.title', { name: user?.firstname ?? '' })} subtitle={t('home.subtitle')} />
      {home.data && home.data.counts.length > 0 && (
        <section>
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted">{t('home.counts')}</h2>
          <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            {home.data.counts.map((c) => (
              <li key={c.uid}>
                <Link
                  to={`/content-manager/${c.kind === 'singleType' ? 'single-types' : 'collection-types'}/${encodeURIComponent(c.uid)}`}
                  className="block rounded-lg border border-border bg-surface p-4 shadow-sm hover:border-primary/40"
                >
                  <span className="block text-2xl font-semibold">{c.total}</span>
                  <span className="block truncate text-sm text-muted">{c.displayName}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}
      {home.data && (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <RecentList title={t('home.lastEdited')} items={home.data.lastEdited} />
          <RecentList title={t('home.lastPublished')} items={home.data.lastPublished} />
        </div>
      )}
      {shortcuts.length > 0 && (
        <section>
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted">{t('home.shortcuts')}</h2>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {shortcuts.map((s) => (
              <Link key={s.to} to={s.to} className="flex items-center justify-between gap-3 rounded-lg border border-border bg-surface p-4 shadow-sm hover:border-primary/40">
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
