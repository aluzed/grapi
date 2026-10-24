import { useEffect, useId, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, ChevronDown, ExternalLink, GripVertical, Plus, X } from 'lucide-react'
import { useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { requestWithMeta } from '@/lib/api'
import { cn } from '@/lib/cn'
import { appUrl, isSavedMessage, withEmbed } from '@/lib/embed'
import { reorder } from '@/lib/media'
import type { Attribute, Model, Pagination } from '@/lib/types'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { StatusBadge } from './cell'

export type RelationItem = { documentId: string; label: string; status?: string; locale?: string }

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

// As in Strapi, a long relation shows ten targets, then ten more on demand.
const step = 10

type Page = { data: RelationItem[]; meta: { pagination: Pagination } }

const cmPath = (kind: Model['kind'], uid: string, documentId?: string) =>
  `/content-manager/${kind === 'singleType' ? 'single-types' : 'collection-types'}/${encodeURIComponent(uid)}${documentId ? `/${encodeURIComponent(documentId)}` : ''}`

// EmbeddedDocument opens a related document, or a new one, in the admin
// itself, inside a dialog: the document being edited stays as it is.
function EmbeddedDocument({ title, path, onSaved, onClose }: { title: string; path: string; onSaved: (documentId: string, label: string) => void; onClose: () => void }) {
  const { t } = useI18n()
  const frame = useRef<HTMLIFrameElement>(null)
  useEffect(() => {
    const onMessage = (e: MessageEvent) => {
      if (isSavedMessage(e, frame.current?.contentWindow)) onSaved(e.data.documentId, e.data.label)
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [onSaved])
  return (
    <Dialog open wide title={title} onClose={onClose} closeLabel={t('users.close')}>
      <iframe ref={frame} title={title} src={appUrl(withEmbed(path))} className="block h-[70dvh] w-full rounded-md border border-border bg-background" />
    </Dialog>
  )
}

export function RelationField({
  uid,
  component,
  name,
  attr,
  label,
  value,
  onChange,
  locale,
  models,
  openMode = 'modal',
}: {
  uid: string
  component?: string
  name: string
  attr: Attribute
  label: string
  value: RelationItem[]
  onChange: (v: RelationItem[]) => void
  locale?: string
  models: Model[]
  openMode?: string
}) {
  const { t } = useI18n()
  const confirm = useConfirm()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const id = useId()
  const dragType = `application/x-grapi-relation-${id.replace(/[^a-z0-9]/gi, '')}`
  const multiple = isMultiple(attr)
  const target = models.find((m) => m.uid === attr.target)
  const [q, setQ] = useState('')
  const [debounced, setDebounced] = useState('')
  const [open, setOpen] = useState(false)
  const [visible, setVisible] = useState(step)
  const [dragFrom, setDragFrom] = useState<number | null>(null)
  const [embedded, setEmbedded] = useState<{ title: string; path: string; create: boolean } | null>(null)
  useEffect(() => {
    const h = setTimeout(() => setDebounced(q), 250)
    return () => clearTimeout(h)
  }, [q])

  const base = `/content-manager/relations/${encodeURIComponent(uid)}/${encodeURIComponent(name)}`
  const common = new URLSearchParams()
  if (locale) common.set('locale', locale)
  if (component) common.set('component', component)

  const chosen = value.map((v) => v.documentId)
  const targets = useInfiniteQuery({
    queryKey: ['cm', 'relations', uid, component, name, locale, debounced, chosen.join(',')],
    queryFn: ({ pageParam }) => {
      const p = new URLSearchParams(common)
      p.set('page', String(pageParam))
      p.set('pageSize', String(step))
      if (debounced) p.set('_q', debounced)
      if (chosen.length) p.set('idsToOmit', chosen.join(','))
      return requestWithMeta<RelationItem[], { pagination: Pagination }>(`${base}?${p}`) as Promise<Page>
    },
    initialPageParam: 1,
    getNextPageParam: (last) => (last.meta.pagination.page < last.meta.pagination.pageCount ? last.meta.pagination.page + 1 : undefined),
    enabled: open,
  })
  const options = targets.data?.pages.flatMap((p) => p.data) ?? []

  // The labels and statuses of the shown targets come from the server, so
  // that they follow what happens to the targets meanwhile.
  const shown = value.slice(0, visible)
  const shownIds = shown.map((v) => v.documentId).join(',')
  const fresh = useQuery({
    queryKey: ['cm', 'relation-status', uid, component, name, locale, shownIds],
    queryFn: () => {
      const p = new URLSearchParams(common)
      p.set('documentIds', shownIds)
      return requestWithMeta<RelationItem[], unknown>(`${base}?${p}`)
    },
    enabled: shownIds !== '',
  })
  const known = new Map((fresh.data?.data ?? []).map((r) => [r.documentId, r]))

  const add = (o: RelationItem) => {
    onChange(multiple ? [...value, o] : [o])
    setQ('')
    if (!multiple) setOpen(false)
  }
  const move = (from: number, to: number) => onChange(reorder(value, from, to))
  const openTarget = async (item: RelationItem) => {
    if (!target) return
    const path = `${cmPath(target.kind, target.uid, item.documentId)}${item.locale || locale ? `?locale=${item.locale || locale}` : ''}`
    if (openMode === 'newTab') return void window.open(appUrl(path), '_blank', 'noopener')
    if (openMode === 'page') {
      if (await confirm({ title: t('rel.leave'), message: t('rel.leaveConfirm'), confirmLabel: t('rel.leave') })) navigate(path)
      return
    }
    setEmbedded({ title: known.get(item.documentId)?.label || item.label || item.documentId, path, create: false })
  }

  return (
    <div className="@container flex flex-col gap-1.5 sm:col-span-2">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      {value.length === 0 ? (
        <p className="text-xs text-muted">{t('rel.none')}</p>
      ) : (
        <ol className="flex flex-col divide-y divide-border rounded-md border border-border" aria-label={label}>
          {shown.map((item, i) => {
            const info = known.get(item.documentId)
            const text = info?.label || item.label || item.documentId
            return (
              <li
                key={item.documentId}
                draggable={multiple}
                onDragStart={(e) => {
                  e.dataTransfer.setData(dragType, String(i))
                  e.dataTransfer.effectAllowed = 'move'
                  setDragFrom(i)
                }}
                onDragEnd={() => setDragFrom(null)}
                onDragOver={(e) => e.dataTransfer.types.includes(dragType) && e.preventDefault()}
                onDrop={(e) => {
                  if (!e.dataTransfer.types.includes(dragType)) return
                  e.preventDefault()
                  move(Number(e.dataTransfer.getData(dragType)), i)
                }}
                className={cn('flex items-center justify-between gap-2 px-3 py-2', dragFrom === i && 'opacity-50')}
              >
                <span className="flex min-w-0 items-center gap-2">
                  {multiple && <GripVertical className="h-4 w-4 shrink-0 cursor-grab text-muted" aria-hidden />}
                  {target?.permissions.read ? (
                    <button type="button" title={t('rel.open', { name: text })} onClick={() => void openTarget(item)} className="flex min-w-0 items-center gap-1 text-left text-sm font-medium text-primary hover:underline">
                      <span className="truncate">{text}</span>
                      <ExternalLink className="h-3.5 w-3.5 shrink-0" aria-hidden />
                    </button>
                  ) : (
                    <span className="min-w-0 truncate text-sm">{text}</span>
                  )}
                  <StatusBadge status={info?.status ?? item.status} />
                  {(info?.locale ?? item.locale) && <span className="text-xs text-muted uppercase">{info?.locale ?? item.locale}</span>}
                </span>
                <span className="flex shrink-0 gap-1">
                  {multiple && (
                    <>
                      <button type="button" title={t('rel.up')} disabled={i === 0} onClick={() => move(i, i - 1)} className={`${small} border-border bg-surface`}>
                        <ArrowUp className="h-3.5 w-3.5" />
                        <span className="hidden @md:inline">{t('rel.up')}</span>
                      </button>
                      <button type="button" title={t('rel.down')} disabled={i === value.length - 1} onClick={() => move(i, i + 1)} className={`${small} border-border bg-surface`}>
                        <ArrowDown className="h-3.5 w-3.5" />
                        <span className="hidden @md:inline">{t('rel.down')}</span>
                      </button>
                    </>
                  )}
                  <button
                    type="button"
                    title={t('rel.remove')}
                    onClick={async () => {
                      if (await confirm({ title: t('rel.remove'), message: t('rel.removeConfirm'), confirmLabel: t('rel.remove'), destructive: true })) onChange(value.filter((v) => v.documentId !== item.documentId))
                    }}
                    className={`${small} border-destructive/40 bg-surface text-destructive`}
                  >
                    <X className="h-3.5 w-3.5" />
                    <span className="hidden @md:inline">{t('rel.remove')}</span>
                  </button>
                </span>
              </li>
            )
          })}
        </ol>
      )}
      {value.length > visible && (
        <div>
          <Button variant="outline" title={t('rel.more')} icon={<ChevronDown className="h-4 w-4" />} onClick={() => setVisible(visible + step)} className="w-full @md:w-auto">
            {t('rel.more')} ({value.length - visible})
          </Button>
        </div>
      )}
      <div className="flex flex-col gap-2 @md:flex-row @md:items-start">
        <div className="relative flex-1">
          <input
            id={id}
            type="search"
            value={q}
            placeholder={t('rel.search')}
            role="combobox"
            aria-expanded={open}
            aria-controls={`${id}-list`}
            onFocus={() => setOpen(true)}
            onBlur={() => setTimeout(() => setOpen(false), 150)}
            onChange={(e) => setQ(e.target.value)}
            className="block w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-primary focus:outline-none"
          />
          {open && (
            <ul
              id={`${id}-list`}
              role="listbox"
              onScroll={(e) => {
                const el = e.currentTarget
                if (el.scrollTop + el.clientHeight >= el.scrollHeight - 24 && targets.hasNextPage && !targets.isFetchingNextPage) void targets.fetchNextPage()
              }}
              className="absolute z-20 mt-1 max-h-64 w-full overflow-y-auto rounded-md border border-border bg-surface py-1 shadow-lg"
            >
              {options.map((o) => (
                <li key={o.documentId}>
                  <button
                    type="button"
                    role="option"
                    aria-selected={false}
                    title={`${t('rel.add')} : ${o.label || o.documentId}`}
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => add(o)}
                    className="flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm hover:bg-background"
                  >
                    <span className="min-w-0 truncate">{o.label || o.documentId}</span>
                    <StatusBadge status={o.status} />
                  </button>
                </li>
              ))}
              {targets.isFetchingNextPage && <li className="px-3 py-2 text-sm text-muted">{t('common.loading')}</li>}
              {!targets.isPending && options.length === 0 && <li className="px-3 py-2 text-sm text-muted">{t('rel.noResult')}</li>}
            </ul>
          )}
        </div>
        {target?.permissions.create && target.kind === 'collectionType' && (multiple || value.length === 0) && (
          <Button
            variant="outline"
            title={t('rel.create', { name: target.info.displayName })}
            icon={<Plus className="h-4 w-4" />}
            onClick={() => setEmbedded({ title: t('rel.create', { name: target.info.displayName }), path: `${cmPath(target.kind, target.uid)}/create${locale ? `?locale=${locale}` : ''}`, create: true })}
            className="w-full @md:w-auto"
          >
            {t('rel.create', { name: target.info.displayName })}
          </Button>
        )}
      </div>
      {embedded && (
        <EmbeddedDocument
          title={embedded.title}
          path={embedded.path}
          onSaved={(documentId, savedLabel) => {
            // A created target is linked once, on its first save.
            if (embedded.create && !value.some((v) => v.documentId === documentId)) add({ documentId, label: savedLabel })
          }}
          onClose={() => {
            setEmbedded(null)
            void queryClient.invalidateQueries({ queryKey: ['cm', 'relation-status'] })
            void queryClient.invalidateQueries({ queryKey: ['cm', 'relations'] })
          }}
        />
      )}
    </div>
  )
}
