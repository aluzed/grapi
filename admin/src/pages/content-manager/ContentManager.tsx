import { useState } from 'react'
import { Navigate, NavLink, Outlet, useLocation, useMatch, useNavigate, useParams } from 'react-router'
import { SelectField } from '@/components/ui/field'
import { cn } from '@/lib/cn'
import type { Model } from '@/lib/types'
import { isEmbedded } from '@/lib/embed'
import { useI18n } from '@/providers/I18nProvider'
import { useModels } from './hooks'

const pathOf = (m: Model) => `/content-manager/${m.kind === 'singleType' ? 'single-types' : 'collection-types'}/${encodeURIComponent(m.uid)}`

function TypeMenu({ models }: { models: Model[] }) {
  const { t } = useI18n()
  const [search, setSearch] = useState('')
  const shown = models.filter((m) => m.info.displayName.toLowerCase().includes(search.toLowerCase()))
  const group = (kind: Model['kind'], title: string) => {
    const list = shown.filter((m) => m.kind === kind)
    if (list.length === 0) return null
    return (
      <div>
        <h3 className="mb-1 px-3 text-xs font-semibold uppercase tracking-wide text-muted">{title}</h3>
        <ul className="flex flex-col gap-0.5">
          {list.map((m) => (
            <li key={m.uid}>
              <NavLink
                to={pathOf(m)}
                className={({ isActive }) => cn('block rounded-md px-3 py-2 text-sm', isActive ? 'bg-primary/10 font-medium text-primary' : 'hover:bg-background')}
              >
                {m.info.displayName}
              </NavLink>
            </li>
          ))}
        </ul>
      </div>
    )
  }
  return (
    <div className="flex flex-col gap-4">
      <input
        type="search"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        placeholder={t('cm.searchTypes')}
        aria-label={t('cm.searchTypes')}
        className="w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-primary focus:outline-none"
      />
      {group('collectionType', t('cm.collectionTypes'))}
      {group('singleType', t('cm.singleTypes'))}
    </div>
  )
}

export function ContentManager() {
  const { t } = useI18n()
  const models = useModels()
  const navigate = useNavigate()
  const uid = useMatch('/content-manager/:kind/:uid/*')?.params.uid
  const location = useLocation()
  if (isEmbedded(location.search)) return <Outlet />
  if (models.isPending) return <p className="text-sm text-muted">{t('common.loading')}</p>
  const list = models.data ?? []
  if (list.length === 0) return <p className="text-sm text-muted">{t('cm.none')}</p>
  return (
    <div className="flex flex-col gap-6 lg:flex-row">
      <aside className="hidden w-60 shrink-0 lg:block">
        <h2 className="mb-3 text-lg font-semibold">{t('nav.contentManager')}</h2>
        <TypeMenu models={list} />
      </aside>
      <div className="lg:hidden">
        <SelectField
          label={t('cm.types')}
          value={uid ?? ''}
          onChange={(e) => {
            const m = list.find((x) => x.uid === e.target.value)
            if (m) navigate(pathOf(m))
          }}
        >
          {!uid && <option value="">-</option>}
          {list.map((m) => (
            <option key={m.uid} value={m.uid}>
              {m.info.displayName}
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

export function FirstModel() {
  const models = useModels()
  const first = models.data?.[0]
  return first ? <Navigate to={pathOf(first)} replace /> : null
}

export function useModel(): Model | undefined {
  const { uid } = useParams()
  return useModels().data?.find((m) => m.uid === uid)
}
