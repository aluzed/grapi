import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Save } from 'lucide-react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { Checkbox, SelectField, TextField } from '@/components/ui/field'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { Tabs } from '@/components/ui/tabs'
import { api, ApiError } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import { isoToZoned, localTimeZone, timeZones, zonedToISO } from '@/lib/tz'
import { useReleaseSettings } from '@/pages/settings/ReleaseSettings'
import { useAuth } from '@/providers/AuthProvider'
import type { MessageKey } from '@/i18n'
import { useI18n } from '@/providers/I18nProvider'
import { statusTone, type Release } from './model'

export function StatusPill({ status }: { status: string }) {
  const { t } = useI18n()
  return (
    <span className={`inline-flex rounded border px-1.5 py-0.5 text-xs font-medium ${statusTone[status] ?? statusTone.empty}`}>
      {t(`releases.status.${status}` as MessageKey)}
    </span>
  )
}

export function useReleaseDate() {
  const { language } = useI18n()
  return (iso: string, timeZone?: string | null) => new Date(iso).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'short', timeZone: timeZone ?? undefined })
}

export function ReleaseDialog({ release, onClose, onSaved }: { release: Release | null; onClose: () => void; onSaved?: (r: Release) => void }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [name, setName] = useState(release?.name ?? '')
  const [scheduled, setScheduled] = useState(Boolean(release?.scheduledAt))
  const { permissions } = useAuth()
  const defaults = useReleaseSettings(!release && can(permissions, 'admin::releases.settings.read'))
  const [timezone, setTimezone] = useState(release?.timezone ?? localTimeZone())
  useEffect(() => {
    if (!release && defaults.data?.defaultTimezone) setTimezone(defaults.data.defaultTimezone)
  }, [release, defaults.data])
  const [at, setAt] = useState(release?.scheduledAt ? isoToZoned(release.scheduledAt, release.timezone ?? 'UTC') : '')
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useMutation({
    mutationFn: () => {
      const body = { name, scheduledAt: scheduled && at ? zonedToISO(at, timezone) : null, timezone: scheduled ? timezone : null }
      return release ? api.put<Release>(`/releases/${release.id}`, body) : api.post<Release>('/releases', body)
    },
    onSuccess: (r) => {
      toast.success(t('releases.saved'))
      void queryClient.invalidateQueries({ queryKey: ['releases'] })
      onClose()
      onSaved?.(r)
    },
    onError: (err) => {
      if (err instanceof ApiError) setErrors(Object.fromEntries(err.fieldErrors().map((e) => [e.path[0] ?? '', e.message])))
      showError(err, undefined, t('common.error'))
    },
  })
  return (
    <Dialog
      open
      title={release ? release.name : t('releases.create')}
      onClose={onClose}
      closeLabel={t('common.cancel')}
      footer={
        <>
          <Button variant="outline" title={t('common.cancel')} onClick={onClose} className="w-full md:w-auto">
            {t('common.cancel')}
          </Button>
          <Button title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={save.isPending} disabled={scheduled && !at} onClick={() => save.mutate()} className="w-full md:w-auto">
            {t('common.save')}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <TextField label={t('releases.name')} value={name} error={errors.name} onChange={(e) => setName(e.target.value)} />
        <Checkbox label={t('releases.schedule')} checked={scheduled} onChange={(e) => setScheduled(e.target.checked)} />
        {scheduled && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <TextField label={t('releases.date')} type="datetime-local" value={at} error={errors.scheduledAt} onChange={(e) => setAt(e.target.value)} />
            <SelectField label={t('releases.timezone')} value={timezone} error={errors.timezone} onChange={(e) => setTimezone(e.target.value)}>
              {timeZones().map((z) => (
                <option key={z} value={z}>
                  {z}
                </option>
              ))}
            </SelectField>
          </div>
        )}
      </div>
    </Dialog>
  )
}

export function Releases() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { permissions } = useAuth()
  const date = useReleaseDate()
  const [tab, setTab] = useState<'pending' | 'done'>('pending')
  const [creating, setCreating] = useState(false)
  const releases = useQuery({ queryKey: ['releases', tab], queryFn: () => api.get<Release[]>(`/releases${tab === 'done' ? '?status=done' : ''}`) })
  const list = releases.data ?? []
  const when = (r: Release) => (r.releasedAt ? date(r.releasedAt) : r.scheduledAt ? date(r.scheduledAt, r.timezone) : t('releases.notScheduled'))
  const open = (r: Release) => navigate(`/releases/${r.id}`)
  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t('nav.releases')}
        subtitle={t('releases.subtitle')}
        actions={
          can(permissions, 'admin::releases.create') && (
            <Button title={t('releases.create')} icon={<Plus className="h-4 w-4" />} onClick={() => setCreating(true)} className="w-full md:w-auto">
              {t('releases.create')}
            </Button>
          )
        }
      />
      <Tabs
        tabs={[
          { id: 'pending', label: t('releases.pending') },
          { id: 'done', label: t('releases.done') },
        ]}
        value={tab}
        onChange={(id) => setTab(id as 'pending' | 'done')}
      />
      <div className="hidden md:block">
        <Table>
          <thead>
            <tr>
              <Th>{t('releases.name')}</Th>
              <Th>{tab === 'done' ? t('releases.releasedAt') : t('releases.scheduledAt')}</Th>
              <Th>{t('releases.entries')}</Th>
              <Th>{t('users.status')}</Th>
            </tr>
          </thead>
          <tbody>
            {list.map((r) => (
              <Tr key={r.id} className="cursor-pointer" onClick={() => open(r)}>
                <Td className="font-medium">{r.name}</Td>
                <Td className="text-muted">{when(r)}</Td>
                <Td>{r.count}</Td>
                <Td>
                  <StatusPill status={r.status} />
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </div>
      <ul className="flex flex-col gap-3 md:hidden">
        {list.map((r) => (
          <li key={r.id}>
            <button type="button" title={r.name} onClick={() => open(r)} className="flex w-full flex-col gap-1 rounded-lg border border-border bg-surface p-4 text-left shadow-sm">
              <span className="font-medium">{r.name}</span>
              <span className="text-sm text-muted">{when(r)}</span>
              <span className="text-sm text-muted">{t('releases.entryCount', { count: String(r.count) })}</span>
              <span className="mt-1">
                <StatusPill status={r.status} />
              </span>
            </button>
          </li>
        ))}
      </ul>
      {!releases.isPending && list.length === 0 && <Card className="text-center text-sm text-muted">{tab === 'done' ? t('releases.emptyDone') : t('releases.empty')}</Card>}
      {creating && <ReleaseDialog release={null} onClose={() => setCreating(false)} onSaved={open} />}
    </div>
  )
}
