import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, ExternalLink, Pencil, Rocket, Trash2 } from 'lucide-react'
import { useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { EntityActionsMenu, type EntityAction } from '@/components/ui/entity-actions-menu'
import { SelectField } from '@/components/ui/field'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { api } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import type { MessageKey } from '@/i18n'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import type { Release, ReleaseAction } from './model'
import { ReleaseDialog, StatusPill, useReleaseDate } from './Releases'

function entryURL(a: ReleaseAction) {
  return `/content-manager/collection-types/${encodeURIComponent(a.uid)}/${encodeURIComponent(a.documentId)}${a.locale ? `?locale=${a.locale}` : ''}`
}

export function ReleaseDetail() {
  const { t } = useI18n()
  const { id } = useParams()
  const navigate = useNavigate()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const { permissions } = useAuth()
  const date = useReleaseDate()
  const [editing, setEditing] = useState(false)
  const [groupBy, setGroupBy] = useState<'contentType' | 'locale'>('contentType')
  const release = useQuery({ queryKey: ['releases', 'detail', id], queryFn: () => api.get<Release>(`/releases/${id}`) })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: ['releases'] })
  const publish = useMutation({
    mutationFn: () => api.post<Release>(`/releases/${id}/publish`),
    onSuccess: () => {
      toast.success(t('releases.published'))
      refresh()
      void queryClient.invalidateQueries({ queryKey: ['cm'] })
    },
    onError: (e) => {
      refresh()
      showError(e, undefined, t('releases.blockedError'))
    },
  })
  const remove = useMutation({
    mutationFn: () => api.del(`/releases/${id}`),
    onSuccess: () => {
      toast.success(t('releases.deleted'))
      refresh()
      navigate('/releases')
    },
    onError: (e) => showError(e),
  })
  const removeAction = useMutation({
    mutationFn: (a: ReleaseAction) => api.del(`/releases/${id}/actions/${a.id}`),
    onSuccess: refresh,
    onError: (e) => showError(e),
  })
  const r = release.data
  const done = Boolean(r?.releasedAt)
  const actions = r?.actions ?? []
  const rowActions = (a: ReleaseAction) => {
    const out: EntityAction[] = [{ label: t('releases.openEntry'), icon: <ExternalLink className="h-4 w-4" />, onSelect: () => navigate(entryURL(a)) }]
    if (!done && can(permissions, 'admin::releases.actions.delete'))
      out.push({
        label: t('releases.removeEntry'),
        icon: <Trash2 className="h-4 w-4" />,
        destructive: true,
        onSelect: async () => {
          if (await confirm({ title: t('releases.removeEntry'), message: t('releases.removeEntryConfirm', { name: a.title || a.documentId }), confirmLabel: t('releases.removeEntry'), destructive: true }))
            removeAction.mutate(a)
        },
      })
    return out
  }
  const state = (a: ReleaseAction) =>
    a.state && (
      <div className="flex flex-col gap-1">
        <span>
          <StatusPill status={a.state} />
        </span>
        {a.errors?.map((e) => (
          <span key={e} className="text-xs text-destructive">
            {e}
          </span>
        ))}
      </div>
    )
  const groupKey = (a: ReleaseAction) => (groupBy === 'locale' ? (a.locale ?? t('releases.noLocale')) : (a.contentType ?? a.uid))
  const groups = [...actions.reduce((m, a) => m.set(groupKey(a), [...(m.get(groupKey(a)) ?? []), a]), new Map<string, ReleaseAction[]>())].sort(([x], [y]) => x.localeCompare(y))
  const typeLabel = (a: ReleaseAction) => t(`releases.type.${a.type}` as MessageKey)
  return (
    <div className="flex flex-col gap-6">
      <div className="space-y-4">
        <div>
          <Button variant="outline" title={t('releases.back')} icon={<ArrowLeft className="h-4 w-4" />} onClick={() => navigate('/releases')} className="w-full md:w-auto">
            {t('releases.back')}
          </Button>
        </div>
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div className="flex flex-col gap-1">
            <h1 className="text-2xl font-semibold tracking-tight">{r?.name ?? '-'}</h1>
            {r && (
              <p className="text-sm text-muted">
                {r.releasedAt
                  ? t('releases.releasedOn', { date: date(r.releasedAt) })
                  : r.scheduledAt
                    ? t('releases.scheduledOn', { date: date(r.scheduledAt, r.timezone), timezone: r.timezone ?? 'UTC' })
                    : t('releases.notScheduled')}
              </p>
            )}
            {r && (
              <span>
                <StatusPill status={r.status} />
              </span>
            )}
          </div>
          {r && !done && (
            <div className="flex flex-col gap-2 md:flex-row">
              {can(permissions, 'admin::releases.update') && (
                <Button variant="outline" title={t('releases.edit')} icon={<Pencil className="h-4 w-4" />} onClick={() => setEditing(true)} className="w-full md:w-auto">
                  {t('releases.edit')}
                </Button>
              )}
              {can(permissions, 'admin::releases.delete') && (
                <Button
                  variant="destructive-outline"
                  title={t('releases.delete')}
                  icon={<Trash2 className="h-4 w-4" />}
                  onClick={async () => {
                    if (await confirm({ title: t('releases.delete'), message: t('releases.deleteConfirm', { name: r.name }), confirmLabel: t('releases.delete'), destructive: true })) remove.mutate()
                  }}
                  className="w-full md:w-auto"
                >
                  {t('releases.delete')}
                </Button>
              )}
              {can(permissions, 'admin::releases.publish') && (
                <Button
                  title={t('releases.publish')}
                  icon={<Rocket className="h-4 w-4" />}
                  loading={publish.isPending}
                  disabled={r.status !== 'ready'}
                  onClick={async () => {
                    if (await confirm({ title: t('releases.publish'), message: t('releases.publishConfirm', { count: String(actions.length) }), confirmLabel: t('releases.publish') })) publish.mutate()
                  }}
                  className="w-full md:w-auto"
                >
                  {t('releases.publish')}
                </Button>
              )}
            </div>
          )}
        </div>
      </div>
      {actions.length > 1 && actions.some((a) => a.locale) && (
        <div className="sm:w-64">
          <SelectField label={t('releases.groupBy')} value={groupBy} onChange={(e) => setGroupBy(e.target.value as 'contentType' | 'locale')}>
            <option value="contentType">{t('releases.contentType')}</option>
            <option value="locale">{t('releases.locale')}</option>
          </SelectField>
        </div>
      )}
      {r?.error && <Card className="border-destructive/40 text-sm text-destructive">{t('releases.failedWith', { error: r.error })}</Card>}
      {r?.status === 'blocked' && <Card className="border-amber-200 bg-amber-50 text-sm text-amber-800">{t('releases.blockedHint')}</Card>}
      {groups.map(([group, list]) => (
        <section key={group} className="flex flex-col gap-2">
          {groups.length > 1 && <h2 className="text-sm font-semibold uppercase tracking-wide text-muted">{group}</h2>}
        <div className="hidden md:block">
          <Table>
            <thead>
              <tr>
                <Th>{t('releases.entry')}</Th>
                <Th>{t('releases.contentType')}</Th>
                <Th>{t('releases.action')}</Th>
                {!done && <Th>{t('releases.state')}</Th>}
                <Th className="w-16">
                  <span className="sr-only">{t('cm.actions')}</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {list.map((a) => (
                <Tr key={a.id}>
                  <Td className="font-medium">
                    {a.title || a.documentId}
                    {a.locale && <span className="ml-2 text-xs text-muted">{a.locale}</span>}
                  </Td>
                  <Td className="text-muted">{a.contentType ?? a.uid}</Td>
                  <Td>{typeLabel(a)}</Td>
                  {!done && <Td>{state(a)}</Td>}
                  <Td className="text-right">
                    <EntityActionsMenu actions={rowActions(a)} label={t('cm.actions')} />
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        </div>
        <ul className="flex flex-col gap-3 md:hidden">
          {list.map((a) => (
            <li key={a.id} className="flex items-start justify-between gap-3 rounded-lg border border-border bg-surface p-4 shadow-sm">
              <div className="flex min-w-0 flex-col gap-1">
                <p className="font-medium">{a.title || a.documentId}</p>
                <p className="text-sm text-muted">
                  {a.contentType ?? a.uid}
                  {a.locale ? ` · ${a.locale}` : ''}
                </p>
                <p className="text-sm">{typeLabel(a)}</p>
                {!done && state(a)}
              </div>
              <EntityActionsMenu actions={rowActions(a)} label={t('cm.actions')} />
            </li>
          ))}
        </ul>
        </section>
      ))}
      {r && actions.length === 0 && <Card className="text-center text-sm text-muted">{t('releases.noEntries')}</Card>}
      {editing && r && <ReleaseDialog release={r} onClose={() => setEditing(false)} />}
    </div>
  )
}
