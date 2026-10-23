import { useEffect, useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, X } from 'lucide-react'
import { requestWithMeta } from '@/lib/api'
import type { Attribute, Model } from '@/lib/types'
import { useI18n } from '@/providers/I18nProvider'

export type RelationItem = { documentId: string; label: string }

const toMany = new Set(['oneToMany', 'manyToMany', 'manyWay'])
export const isMultiple = (attr: Attribute) => toMany.has(attr.relation ?? '')
export const isEditableRelation = (attr: Attribute) => attr.type === 'relation' && !(attr.relation ?? '').startsWith('morph') && Boolean(attr.target)

// relationItems reads a populated relation value.
export function relationItems(value: unknown, attr: Attribute, models: Model[]): RelationItem[] {
  if (!value) return []
  const target = models.find((m) => m.uid === attr.target)
  const main = (v: Record<string, unknown>) =>
    String((target ? v[target.mainField] : undefined) ?? v.name ?? v.title ?? v.documentId ?? '')
  const list = (Array.isArray(value) ? value : [value]) as Record<string, unknown>[]
  return list.map((v) => ({ documentId: String(v.documentId), label: main(v) }))
}

const small =
  'inline-flex min-h-9 items-center gap-1 rounded-md border px-2 text-xs font-medium disabled:cursor-not-allowed disabled:bg-zinc-200 disabled:text-zinc-500'

export function RelationField({ uid, component, name, attr, label, value, onChange, locale }: { uid: string; component?: string; name: string; attr: Attribute; label: string; value: RelationItem[]; onChange: (v: RelationItem[]) => void; locale?: string }) {
  const { t } = useI18n()
  const id = useId()
  const multiple = isMultiple(attr)
  const [q, setQ] = useState('')
  const [debounced, setDebounced] = useState('')
  const [open, setOpen] = useState(false)
  useEffect(() => {
    const h = setTimeout(() => setDebounced(q), 250)
    return () => clearTimeout(h)
  }, [q])
  const params = new URLSearchParams({ pageSize: '10' })
  if (debounced) params.set('_q', debounced)
  if (locale) params.set('locale', locale)
  if (component) params.set('component', component)
  const targets = useQuery({
    queryKey: ['cm', 'relations', uid, component, name, params.toString()],
    queryFn: () => requestWithMeta<RelationItem[], unknown>(`/content-manager/relations/${encodeURIComponent(uid)}/${encodeURIComponent(name)}?${params}`),
    enabled: open,
  })
  const chosen = new Set(value.map((v) => v.documentId))
  const options = (targets.data?.data ?? []).filter((o) => !chosen.has(o.documentId))
  const move = (i: number, d: number) => {
    const next = [...value]
    const [item] = next.splice(i, 1)
    if (item) next.splice(i + d, 0, item)
    onChange(next)
  }
  return (
    <div className="flex flex-col gap-1.5 sm:col-span-2">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      {value.length === 0 ? (
        <p className="text-xs text-muted">{t('rel.none')}</p>
      ) : (
        <ul className="flex flex-col divide-y divide-border rounded-md border border-border">
          {value.map((item, i) => (
            <li key={item.documentId} className="flex items-center justify-between gap-2 px-3 py-2">
              <span className="min-w-0 truncate text-sm">{item.label || item.documentId}</span>
              <span className="flex shrink-0 gap-1">
                {multiple && (
                  <>
                    <button type="button" title={t('rel.up')} disabled={i === 0} onClick={() => move(i, -1)} className={`${small} border-border bg-surface`}>
                      <ArrowUp className="h-3.5 w-3.5" />
                      <span className="hidden sm:inline">{t('rel.up')}</span>
                    </button>
                    <button type="button" title={t('rel.down')} disabled={i === value.length - 1} onClick={() => move(i, 1)} className={`${small} border-border bg-surface`}>
                      <ArrowDown className="h-3.5 w-3.5" />
                      <span className="hidden sm:inline">{t('rel.down')}</span>
                    </button>
                  </>
                )}
                <button type="button" title={t('rel.remove')} onClick={() => onChange(value.filter((v) => v.documentId !== item.documentId))} className={`${small} border-destructive/40 bg-surface text-destructive`}>
                  <X className="h-3.5 w-3.5" />
                  <span className="hidden sm:inline">{t('rel.remove')}</span>
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}
      <div className="relative">
        <input
          id={id}
          type="search"
          value={q}
          placeholder={t('rel.search')}
          onFocus={() => setOpen(true)}
          onBlur={() => setTimeout(() => setOpen(false), 150)}
          onChange={(e) => setQ(e.target.value)}
          className="block w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-primary focus:outline-none"
        />
        {open && (
          <ul role="listbox" className="absolute z-20 mt-1 max-h-64 w-full overflow-y-auto rounded-md border border-border bg-surface py-1 shadow-lg">
            {options.map((o) => (
              <li key={o.documentId}>
                <button
                  type="button"
                  role="option"
                  aria-selected={false}
                  title={`${t('rel.add')} : ${o.label || o.documentId}`}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => {
                    onChange(multiple ? [...value, o] : [o])
                    setQ('')
                    if (!multiple) setOpen(false)
                  }}
                  className="block w-full px-3 py-2 text-left text-sm hover:bg-background"
                >
                  {o.label || o.documentId}
                </button>
              </li>
            ))}
            {!targets.isPending && options.length === 0 && <li className="px-3 py-2 text-sm text-muted">{t('rel.noResult')}</li>}
          </ul>
        )}
      </div>
    </div>
  )
}
