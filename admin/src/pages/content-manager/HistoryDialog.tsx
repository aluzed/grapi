import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { History, RotateCcw } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Tabs } from '@/components/ui/tabs'
import { api } from '@/lib/api'
import { cn } from '@/lib/cn'
import { showError } from '@/lib/form'
import type { Model } from '@/lib/types'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { Cell, StatusBadge } from './cell'
import { useModels } from './hooks'

type Version = {
  id: number
  status: string
  locale: string | null
  createdAt: string
  createdBy: { firstname?: string; lastname?: string; email?: string } | null
  data?: Record<string, unknown>
  unknownFields?: Record<string, unknown>
  missingFields?: string[]
  danglingReferences?: { path: string; target: string; documentId?: string; fileId?: number }[]
}

type Change = { path: string; before: unknown; after: unknown }

function short(v: unknown): string {
  if (v === null || v === undefined || v === '') return '-'
  if (typeof v === 'object') {
    const text = JSON.stringify(v)
    return text.length > 160 ? `${text.slice(0, 157)}...` : text
  }
  return String(v)
}

const who = (v: Version) => [v.createdBy?.firstname, v.createdBy?.lastname].filter(Boolean).join(' ') || v.createdBy?.email || ''

export function HistoryButton({ model, documentId, locale, title }: { model: Model; documentId: string; locale?: string; title: string }) {
  const { t, language } = useI18n()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const models = useModels().data ?? []
  const [open, setOpen] = useState(false)
  const [selected, setSelected] = useState<number | null>(null)
  const [view, setView] = useState<'content' | 'diff'>('content')
  const versions = useQuery({
    queryKey: ['cm', 'history', model.uid, documentId, locale],
    queryFn: () => api.get<Version[]>(`/content-manager/history/${encodeURIComponent(model.uid)}/${encodeURIComponent(documentId)}${locale ? `?locale=${locale}` : ''}`),
    enabled: open,
  })
  const current = selected ?? versions.data?.[0]?.id ?? null
  const version = useQuery({ queryKey: ['cm', 'history', 'version', current], queryFn: () => api.get<Version>(`/content-manager/history/versions/${current}`), enabled: open && current !== null })
  const diff = useQuery({
    queryKey: ['cm', 'history', 'diff', current],
    queryFn: () => api.get<{ changes: Change[] }>(`/content-manager/history/versions/${current}/diff`),
    enabled: open && current !== null && view === 'diff',
  })
  const restore = useMutation({
    mutationFn: (id: number) => api.post(`/content-manager/history/versions/${id}/restore`),
    onSuccess: () => {
      toast.success(t('history.restored'))
      void queryClient.invalidateQueries({ queryKey: ['cm'] })
      setOpen(false)
    },
    onError: (e) => showError(e, undefined, t('common.error')),
  })
  const date = (iso: string) => new Date(iso).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'short' })
  const v = version.data
  return (
    <>
      <Button variant="outline" title={t('history.open')} icon={<History className="h-4 w-4" />} onClick={() => setOpen(true)} className="w-full md:w-auto">
        {t('history.open')}
      </Button>
      {open && (
        <Dialog
          open
          wide
          title={t('history.title', { name: title })}
          onClose={() => setOpen(false)}
          closeLabel={t('users.close')}
          footer={
            model.permissions.update &&
            v && (
              <Button
                title={t('history.restore')}
                icon={<RotateCcw className="h-4 w-4" />}
                loading={restore.isPending}
                className="w-full md:w-auto"
                onClick={async () => {
                  const dangling = v.danglingReferences?.length ? ` ${t('history.danglingConfirm', { count: String(v.danglingReferences.length) })}` : ''
                  const message = (model.localized ? `${t('history.restoreConfirm')} ${t('history.restoreLocales')}` : t('history.restoreConfirm')) + dangling
                  if (await confirm({ title: t('history.restore'), message, confirmLabel: t('history.restore') })) restore.mutate(v.id)
                }}
              >
                {t('history.restore')}
              </Button>
            )
          }
        >
          {versions.data && versions.data.length === 0 ? (
            <p className="text-sm text-muted">{t('history.empty')}</p>
          ) : (
            <div className="flex flex-col gap-4 md:flex-row">
              <ul className="flex max-h-64 flex-col gap-1 overflow-y-auto md:max-h-[60vh] md:w-56 md:shrink-0">
                {versions.data?.map((x) => (
                  <li key={x.id}>
                    <button
                      type="button"
                      title={date(x.createdAt)}
                      onClick={() => setSelected(x.id)}
                      className={cn('w-full rounded-md border px-3 py-2 text-left text-sm', x.id === current ? 'border-primary bg-primary/5' : 'border-border hover:bg-background')}
                    >
                      <span className="block font-medium">{date(x.createdAt)}</span>
                      <span className="mt-1 flex items-center gap-2 text-xs text-muted">
                        <StatusBadge status={x.status} />
                        {who(x) && t('history.by', { name: who(x) })}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
              <div className="min-w-0 flex-1">
                {!v ? (
                  <p className="text-sm text-muted">{t('history.select')}</p>
                ) : (
                  <div className="flex flex-col gap-4">
                    <Tabs
                      tabs={[
                        { id: 'content', label: t('history.content') },
                        { id: 'diff', label: t('history.diff') },
                      ]}
                      value={view}
                      onChange={(id) => setView(id as 'content' | 'diff')}
                    />
                    {v.danglingReferences && v.danglingReferences.length > 0 && (
                      <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
                        <p className="font-medium">{t('history.dangling')}</p>
                        <ul className="mt-1 list-disc pl-5">
                          {v.danglingReferences.map((d) => (
                            <li key={`${d.path}-${d.documentId ?? d.fileId}`}>
                              {d.path}: {d.documentId ?? `#${d.fileId}`}
                            </li>
                          ))}
                        </ul>
                      </div>
                    )}
                    {view === 'diff' ? (
                      diff.data && diff.data.changes.length === 0 ? (
                        <p className="text-sm text-muted">{t('history.noDiff')}</p>
                      ) : (
                        <ul className="flex flex-col gap-2">
                          {diff.data?.changes.map((c) => (
                            <li key={c.path} className="rounded-md border border-border p-3 text-sm">
                              <p className="font-mono text-xs font-semibold text-muted">{c.path}</p>
                              <div className="mt-1 grid grid-cols-1 gap-2 sm:grid-cols-2">
                                <p className="break-words rounded bg-destructive/5 px-2 py-1">
                                  <span className="mr-1 text-xs text-muted">{t('history.thisVersion')}</span>
                                  {short(c.before)}
                                </p>
                                <p className="break-words rounded bg-success/5 px-2 py-1">
                                  <span className="mr-1 text-xs text-muted">{t('history.currentDraft')}</span>
                                  {short(c.after)}
                                </p>
                              </div>
                            </li>
                          ))}
                        </ul>
                      )
                    ) : (
                    <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                      {model.fields
                        .filter((f) => v.data && f in v.data && f !== 'documentId')
                        .map((f) => (
                          <div key={f} className="min-w-0">
                            <dt className="text-xs font-semibold text-muted">{model.editMeta?.[f]?.label || f}</dt>
                            <dd className="text-sm break-words">
                              <Cell value={v.data![f]} attr={model.attributes[f]} models={models} />
                            </dd>
                          </div>
                        ))}
                    </dl>
                    )}
                    {v.missingFields && v.missingFields.length > 0 && <p className="text-xs text-muted">{t('history.missing', { fields: v.missingFields.join(', ') })}</p>}
                    {v.unknownFields && Object.keys(v.unknownFields).length > 0 && (
                      <div className="rounded-md border border-dashed border-border p-3">
                        <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted">{t('history.unknown')}</p>
                        <dl className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                          {Object.entries(v.unknownFields).map(([k, val]) => (
                            <div key={k}>
                              <dt className="text-xs font-semibold text-muted">{k}</dt>
                              <dd className="text-sm break-words">
                                <Cell value={val} models={models} />
                              </dd>
                            </div>
                          ))}
                        </dl>
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>
          )}
        </Dialog>
      )}
    </>
  )
}
