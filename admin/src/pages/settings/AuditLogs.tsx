import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Download, Eye } from 'lucide-react'
import { PageHeader } from '@/app/PageHeader'
import { Pager } from '@/components/Pager'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { SelectField, TextField } from '@/components/ui/field'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { api, download, requestWithMeta } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import type { Pagination } from '@/lib/types'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'

type Entry = { id: number; action: string; date: string; origin: string; user: { id: number; name: string; email: string } | null; payload?: Record<string, unknown> }

const actions = [
  'admin.auth.success', 'admin.auth.error', 'admin.logout', 'entry.create', 'entry.update', 'entry.delete', 'entry.publish', 'entry.unpublish',
  'media.create', 'media.update', 'media.delete', 'media-folder.create', 'media-folder.update', 'media-folder.delete', 'user.create', 'user.update',
  'user.delete', 'user.password.update', 'user.password-reset.create', 'user.password-reset.confirm', 'user.invite.accept', 'role.create', 'role.update',
  'role.delete', 'permission.update', 'token.create', 'token.update', 'token.regenerate', 'token.delete', 'webhook.create', 'webhook.update',
  'webhook.delete', 'locale.create', 'locale.update', 'locale.delete', 'locale.default.update', 'content-type.create', 'content-type.update',
  'content-type.delete', 'component.create', 'component.update', 'component.delete', 'release.create', 'release.update', 'release.delete',
  'release.publish', 'release.trigger', 'release-action.add', 'release-action.remove', 'release-settings.update', 'audit-log.export',
]

const origins = ['admin-panel', 'scheduler', 'cli']

function Detail({ id, onClose }: { id: number; onClose: () => void }) {
  const { t } = useI18n()
  const entry = useQuery({ queryKey: ['audit', id], queryFn: () => api.get<Entry>(`/audit-logs/${id}`) })
  return (
    <Dialog open wide title={t('audit.detail')} onClose={onClose} closeLabel={t('users.close')}>
      {entry.data && <pre className="overflow-x-auto rounded-md bg-background p-3 text-xs">{JSON.stringify(entry.data, null, 2)}</pre>}
    </Dialog>
  )
}

export function AuditLogs() {
  const { t, language } = useI18n()
  const { permissions } = useAuth()
  const [action, setAction] = useState('')
  const [origin, setOrigin] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [page, setPage] = useState(1)
  const [open, setOpen] = useState<number | null>(null)
  const params = new URLSearchParams({ page: String(page), pageSize: '25' })
  if (action) params.set('action', action)
  if (origin) params.set('origin', origin)
  if (from) params.set('from', new Date(from).toISOString())
  if (to) params.set('to', new Date(to).toISOString())
  const logs = useQuery({ queryKey: ['audit', 'list', params.toString()], queryFn: () => requestWithMeta<Entry[], { pagination: Pagination }>(`/audit-logs?${params}`), placeholderData: (p) => p })
  const list = logs.data?.data ?? []
  const pagination = logs.data?.meta.pagination
  const date = (iso: string) => new Date(iso).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'medium' })
  const exportParams = new URLSearchParams(params)
  exportParams.delete('page')
  exportParams.delete('pageSize')
  const who = (e: Entry) => (e.user ? e.user.name || e.user.email : t('audit.system'))
  const summary = (e: Entry) => [e.payload?.uid, e.payload?.documentId].filter(Boolean).join(' · ')
  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t('settings.audit')}
        actions={
          can(permissions, 'admin::audit-logs.export') && (
            <Button
              variant="outline"
              title={t('audit.export')}
              icon={<Download className="h-4 w-4" />}
              className="w-full md:w-auto"
              onClick={() => download(`/audit-logs/export.csv?${exportParams}`, 'audit-logs.csv').catch((e) => showError(e))}
            >
              {t('audit.export')}
            </Button>
          )
        }
      />
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <SelectField
          label={t('audit.action')}
          value={action}
          onChange={(e) => {
            setAction(e.target.value)
            setPage(1)
          }}
        >
          <option value="">{t('audit.allActions')}</option>
          {actions.map((a) => (
            <option key={a} value={a}>
              {a}
            </option>
          ))}
        </SelectField>
        <SelectField
          label={t('audit.origin')}
          value={origin}
          onChange={(e) => {
            setOrigin(e.target.value)
            setPage(1)
          }}
        >
          <option value="">{t('audit.allOrigins')}</option>
          {origins.map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </SelectField>
        <TextField label={t('audit.from')} type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} />
        <TextField label={t('audit.to')} type="datetime-local" value={to} onChange={(e) => setTo(e.target.value)} />
      </div>
      <div className="hidden md:block">
        <Table>
          <thead>
            <tr>
              <Th>{t('audit.action')}</Th>
              <Th>{t('audit.date')}</Th>
              <Th>{t('audit.user')}</Th>
              <Th>{t('audit.details')}</Th>
              <Th>
                <span className="sr-only">{t('audit.view')}</span>
              </Th>
            </tr>
          </thead>
          <tbody>
            {list.map((e) => (
              <Tr key={e.id}>
                <Td className="font-mono text-xs">{e.action}</Td>
                <Td>{date(e.date)}</Td>
                <Td>{who(e)}</Td>
                <Td className="max-w-xs truncate text-muted">{summary(e)}</Td>
                <Td className="text-right">
                  <Button variant="outline" title={t('audit.view')} icon={<Eye className="h-4 w-4" />} onClick={() => setOpen(e.id)}>
                    {t('audit.view')}
                  </Button>
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </div>
      <ul className="flex flex-col gap-3 md:hidden">
        {list.map((e) => (
          <li key={e.id} className="rounded-lg border border-border bg-surface p-4 shadow-sm">
            <p className="font-mono text-xs">{e.action}</p>
            <p className="text-sm">{date(e.date)}</p>
            <p className="text-sm text-muted">{who(e)}</p>
            <Button variant="outline" title={t('audit.view')} icon={<Eye className="h-4 w-4" />} onClick={() => setOpen(e.id)} className="mt-2 w-full">
              {t('audit.view')}
            </Button>
          </li>
        ))}
      </ul>
      {!logs.isPending && list.length === 0 && <Card className="text-center text-sm text-muted">{t('audit.empty')}</Card>}
      {pagination && pagination.pageCount > 1 && <Pager page={page} pageCount={pagination.pageCount} onPage={setPage} className="md:justify-end" />}
      {open !== null && <Detail id={open} onClose={() => setOpen(null)} />}
    </div>
  )
}
