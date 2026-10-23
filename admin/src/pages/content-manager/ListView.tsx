import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, ChevronLeft, ChevronRight, Pencil, Plus, Send, Trash2, Undo2 } from 'lucide-react'
import { Link, useNavigate, useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { SelectField } from '@/components/ui/field'
import { EntityActionsMenu, type EntityAction } from '@/components/ui/entity-actions-menu'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { Tabs } from '@/components/ui/tabs'
import { api, requestWithMeta } from '@/lib/api'
import { showError } from '@/lib/form'
import type { BulkResult, Entry, Model, Pagination } from '@/lib/types'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { Cell, StatusBadge } from './cell'
import { useModel } from './ContentManager'
import { entryPath, label, useLocales, useModels } from './hooks'

type BulkOp = 'delete' | 'publish' | 'unpublish'

function useListParams(model: Model) {
  const [params, setParams] = useSearchParams()
  const page = Number(params.get('page') ?? '1') || 1
  const pageSize = Number(params.get('pageSize') ?? String(model.settings.pageSize)) || model.settings.pageSize
  const sort = params.get('sort') ?? `${model.settings.defaultSortBy}:${model.settings.defaultSortOrder.toLowerCase()}`
  const q = params.get('q') ?? ''
  const status = params.get('status') === 'published' ? 'published' : 'draft'
  const locale = params.get('locale') ?? ''
  const update = (patch: Record<string, string | number | null>, resetPage = true) => {
    const next = new URLSearchParams(params)
    for (const [k, v] of Object.entries(patch)) {
      if (v === null || v === '') next.delete(k)
      else next.set(k, String(v))
    }
    if (resetPage && !('page' in patch)) next.delete('page')
    setParams(next, { replace: true })
  }
  return { page, pageSize, sort, q, status, locale, update }
}

function SearchBox({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { t } = useI18n()
  const [draft, setDraft] = useState(value)
  useEffect(() => setDraft(value), [value])
  useEffect(() => {
    if (draft === value) return
    const id = setTimeout(() => onChange(draft), 300)
    return () => clearTimeout(id)
  }, [draft, value, onChange])
  return (
    <input
      type="search"
      value={draft}
      onChange={(e) => setDraft(e.target.value)}
      placeholder={t('cm.search')}
      aria-label={t('cm.search')}
      className="w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-primary focus:outline-none md:w-72"
    />
  )
}

export function ListView() {
  const model = useModel()
  if (!model) return null
  return <List key={model.uid} model={model} />
}

function List({ model }: { model: Model }) {
  const { t } = useI18n()
  const confirm = useConfirm()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const models = useModels().data ?? []
  const locales = useLocales(model.localized)
  const { page, pageSize, sort, q, status, locale, update } = useListParams(model)
  const [selected, setSelected] = useState<string[]>([])

  const query = new URLSearchParams({ page: String(page), pageSize: String(pageSize), sort })
  if (q) query.set('_q', q)
  if (model.draftAndPublish) query.set('status', status)
  if (model.localized && locale) query.set('locale', locale)
  const listKey = ['cm', 'list', model.uid, query.toString()]
  const list = useQuery({
    queryKey: listKey,
    queryFn: () => requestWithMeta<Entry[], { pagination: Pagination }>(`${entryPath(model.uid)}?${query}`),
    placeholderData: (prev) => prev,
  })
  useEffect(() => setSelected([]), [query.toString()])
  const entries = list.data?.data ?? []
  const pagination = list.data?.meta.pagination
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['cm', 'list', model.uid] })

  const bulk = useMutation({
    mutationFn: ({ op, ids }: { op: BulkOp; ids: string[] }) =>
      api.post<BulkResult>(`${entryPath(model.uid)}/actions/bulk-${op}`, { documentIds: ids, locale: locale || undefined }),
    onSuccess: (res) => {
      if (res.count > 0) toast.success(t('cm.done', { count: String(res.count) }))
      if (res.errors.length > 0) toast.error(`${t('cm.failed', { count: String(res.errors.length) })}: ${res.errors.map((e) => e.message).join(', ')}`)
      setSelected([])
      void refresh()
    },
    onError: (err) => showError(err, undefined, t('common.error')),
  })

  const runBulk = async (op: BulkOp, ids: string[]) => {
    const key = op === 'delete' ? 'cm.bulkDeleteConfirm' : op === 'publish' ? 'cm.bulkPublishConfirm' : 'cm.bulkUnpublishConfirm'
    const message = op === 'delete' && ids.length === 1 ? t('cm.deleteConfirm') : t(key, { count: String(ids.length) })
    const title = t(op === 'delete' ? 'cm.delete' : op === 'publish' ? 'cm.publish' : 'cm.unpublish')
    if (await confirm({ title, message, confirmLabel: title, destructive: op === 'delete' })) bulk.mutate({ op, ids })
  }

  const columns = useMemo(() => model.listColumns, [model])
  const [sortField, sortDir] = sort.split(':')
  const toggleSort = (field: string) => update({ sort: `${field}:${sortField === field && sortDir === 'asc' ? 'desc' : 'asc'}` })
  const editPath = (e: Entry) => `/content-manager/collection-types/${encodeURIComponent(model.uid)}/${encodeURIComponent(e.documentId)}${locale ? `?locale=${locale}` : ''}`

  const rowActions = (e: Entry): EntityAction[] => {
    const out: EntityAction[] = [{ label: t('cm.edit'), icon: <Pencil className="h-4 w-4" />, onSelect: () => navigate(editPath(e)) }]
    if (model.draftAndPublish && model.permissions.publish) {
      out.push({ label: t('cm.publish'), icon: <Send className="h-4 w-4" />, onSelect: () => void runBulk('publish', [e.documentId]) })
      if (e.status !== 'draft') out.push({ label: t('cm.unpublish'), icon: <Undo2 className="h-4 w-4" />, onSelect: () => void runBulk('unpublish', [e.documentId]) })
    }
    if (model.permissions.delete) out.push({ label: t('cm.delete'), icon: <Trash2 className="h-4 w-4" />, destructive: true, onSelect: () => void runBulk('delete', [e.documentId]) })
    return out
  }

  const allChecked = entries.length > 0 && entries.every((e) => selected.includes(e.documentId))
  const toggle = (id: string) => setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]))
  const bulkable = model.settings.bulkable && (model.permissions.delete || (model.draftAndPublish && model.permissions.publish))

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={model.info.displayName}
        subtitle={pagination ? t('cm.entries', { count: String(pagination.total) }) : undefined}
        actions={
          model.permissions.create && (
            <Button title={t('cm.create')} icon={<Plus className="h-4 w-4" />} className="w-full md:w-auto" onClick={() => navigate(`/content-manager/collection-types/${encodeURIComponent(model.uid)}/create${locale ? `?locale=${locale}` : ''}`)}>
              {t('cm.create')}
            </Button>
          )
        }
      />

      <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        {model.settings.searchable && <SearchBox value={q} onChange={(v) => update({ q: v })} />}
        <div className="flex flex-col gap-3 md:flex-row md:items-end">
          {model.draftAndPublish && (
            <Tabs
              tabs={[
                { id: 'draft', label: t('cm.view.draft') },
                { id: 'published', label: t('cm.view.published') },
              ]}
              value={status}
              onChange={(v) => update({ status: v === 'draft' ? null : v })}
            />
          )}
          {model.localized && (locales.data?.length ?? 0) > 0 && (
            <SelectField label={t('cm.locale')} value={locale || (locales.data?.find((l) => l.isDefault)?.code ?? '')} onChange={(e) => update({ locale: e.target.value })}>
              {locales.data?.map((l) => (
                <option key={l.code} value={l.code}>
                  {l.name}
                </option>
              ))}
            </SelectField>
          )}
        </div>
      </div>

      {bulkable && selected.length > 0 && (
        <div className="flex flex-col gap-2 rounded-lg border border-primary/30 bg-primary/5 p-3 md:flex-row md:items-center md:justify-between">
          <span className="text-sm font-medium">{t('cm.selected', { count: String(selected.length) })}</span>
          <div className="flex flex-col gap-2 md:flex-row">
            {model.draftAndPublish && model.permissions.publish && (
              <>
                <Button variant="outline" title={t('cm.publish')} icon={<Send className="h-4 w-4" />} className="w-full md:w-auto" onClick={() => void runBulk('publish', selected)}>
                  {t('cm.publish')}
                </Button>
                <Button variant="outline" title={t('cm.unpublish')} icon={<Undo2 className="h-4 w-4" />} className="w-full md:w-auto" onClick={() => void runBulk('unpublish', selected)}>
                  {t('cm.unpublish')}
                </Button>
              </>
            )}
            {model.permissions.delete && (
              <Button variant="destructive-outline" title={t('cm.delete')} icon={<Trash2 className="h-4 w-4" />} className="w-full md:w-auto" onClick={() => void runBulk('delete', selected)}>
                {t('cm.delete')}
              </Button>
            )}
          </div>
        </div>
      )}

      {list.isError && <Card className="text-sm text-destructive">{list.error.message}</Card>}

      <div className="hidden md:block">
        <Table>
          <thead>
            <tr>
              {bulkable && (
                <Th className="w-10">
                  <input
                    type="checkbox"
                    title={t('cm.selectAll')}
                    aria-label={t('cm.selectAll')}
                    checked={allChecked}
                    onChange={() => setSelected(allChecked ? [] : entries.map((e) => e.documentId))}
                    className="h-4 w-4 accent-primary"
                  />
                </Th>
              )}
              {columns.map((c) => (
                <Th key={c}>
                  <button type="button" title={t('cm.sortBy', { field: label(model, c) })} onClick={() => toggleSort(c)} className="inline-flex items-center gap-1 uppercase hover:text-foreground">
                    {label(model, c)}
                    {sortField === c && (sortDir === 'asc' ? <ArrowUp className="h-3 w-3" /> : <ArrowDown className="h-3 w-3" />)}
                  </button>
                </Th>
              ))}
              {model.draftAndPublish && <Th>{t('cm.statusColumn')}</Th>}
              <Th className="w-16 text-right">
                <span className="sr-only">{t('cm.actions')}</span>
              </Th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e) => (
              <Tr key={e.documentId}>
                {bulkable && (
                  <Td>
                    <input type="checkbox" title={t('cm.select')} aria-label={t('cm.select')} checked={selected.includes(e.documentId)} onChange={() => toggle(e.documentId)} className="h-4 w-4 accent-primary" />
                  </Td>
                )}
                {columns.map((c, i) => (
                  <Td key={c} className="max-w-xs">
                    {i === 0 || c === model.mainField ? (
                      <Link to={editPath(e)} className="font-medium hover:text-primary">
                        <Cell value={e[c]} attr={model.attributes[c]} models={models} />
                      </Link>
                    ) : (
                      <Cell value={e[c]} attr={model.attributes[c]} models={models} />
                    )}
                  </Td>
                ))}
                {model.draftAndPublish && (
                  <Td>
                    <StatusBadge status={e.status} />
                  </Td>
                )}
                <Td className="text-right">
                  <EntityActionsMenu actions={rowActions(e)} label={t('cm.actions')} />
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </div>

      <ul className="flex flex-col gap-3 md:hidden">
        {entries.map((e) => (
          <li key={e.documentId} className="rounded-lg border border-border bg-surface p-4 shadow-sm">
            <div className="flex items-start gap-3">
              {bulkable && (
                <input type="checkbox" title={t('cm.select')} aria-label={t('cm.select')} checked={selected.includes(e.documentId)} onChange={() => toggle(e.documentId)} className="mt-1 h-4 w-4 accent-primary" />
              )}
              <div className="min-w-0 flex-1">
                <Link to={editPath(e)} className="block font-medium">
                  <Cell value={e[model.mainField] ?? e.id} attr={model.attributes[model.mainField]} models={models} />
                </Link>
                <dl className="mt-2 flex flex-col gap-1 text-sm">
                  {columns
                    .filter((c) => c !== model.mainField && c !== 'id')
                    .map((c) => (
                      <div key={c} className="flex flex-col">
                        <dt className="text-xs text-muted">{label(model, c)}</dt>
                        <dd className="min-w-0">
                          <Cell value={e[c]} attr={model.attributes[c]} models={models} />
                        </dd>
                      </div>
                    ))}
                </dl>
                {model.draftAndPublish && (
                  <div className="mt-2">
                    <StatusBadge status={e.status} />
                  </div>
                )}
              </div>
              <EntityActionsMenu actions={rowActions(e)} label={t('cm.actions')} />
            </div>
          </li>
        ))}
      </ul>

      {!list.isPending && entries.length === 0 && <Card className="text-center text-sm text-muted">{t('cm.empty')}</Card>}

      {pagination && pagination.pageCount > 0 && (
        <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
          <SelectField label={t('cm.pageSize')} value={String(pageSize)} onChange={(e) => update({ pageSize: e.target.value })} className="md:w-40">
            {[10, 20, 50, 100].map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </SelectField>
          <div className="flex flex-col gap-2 md:flex-row md:items-center">
            <span className="text-center text-sm text-muted">{t('cm.page', { page: String(page), pages: String(pagination.pageCount) })}</span>
            <Button variant="outline" title={t('cm.previous')} icon={<ChevronLeft className="h-4 w-4" />} disabled={page <= 1} onClick={() => update({ page: page - 1 }, false)} className="w-full md:w-auto">
              {t('cm.previous')}
            </Button>
            <Button variant="outline" title={t('cm.next')} icon={<ChevronRight className="h-4 w-4" />} disabled={page >= pagination.pageCount} onClick={() => update({ page: page + 1 }, false)} className="w-full md:w-auto">
              {t('cm.next')}
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
