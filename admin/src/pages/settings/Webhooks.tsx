import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Send, Trash2, X } from 'lucide-react'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { DialogActions } from '@/components/DialogActions'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { EntityActionsMenu } from '@/components/ui/entity-actions-menu'
import { Checkbox, TextField } from '@/components/ui/field'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { api } from '@/lib/api'
import { fieldErrorMap, showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'

type Webhook = { id: number; name: string; url: string; headers: Record<string, string>; events: string[]; isEnabled: boolean }

function WebhookDialog({ hook, onClose }: { hook: Webhook | null; onClose: () => void }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const events = useQuery({ queryKey: ['webhooks', 'events'], queryFn: () => api.get<string[]>('/webhooks/events'), staleTime: Infinity })
  const [name, setName] = useState(hook?.name ?? '')
  const [url, setUrl] = useState(hook?.url ?? '')
  const [enabled, setEnabled] = useState(hook?.isEnabled ?? true)
  const [chosen, setChosen] = useState<string[]>(hook?.events ?? [])
  const [headers, setHeaders] = useState<[string, string][]>(Object.entries(hook?.headers ?? {}))
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useMutation({
    mutationFn: () => {
      const body = { name, url, isEnabled: enabled, events: chosen, headers: Object.fromEntries(headers.filter(([k]) => k.trim() !== '')) }
      return hook ? api.put(`/webhooks/${hook.id}`, body) : api.post('/webhooks', body)
    },
    onSuccess: () => {
      toast.success(t('webhooks.saved'))
      void queryClient.invalidateQueries({ queryKey: ['webhooks'] })
      onClose()
    },
    onError: (err) => {
      setErrors(fieldErrorMap(err))
      showError(err, undefined, t('common.error'))
    },
  })
  const groups = new Map<string, string[]>()
  for (const e of events.data ?? []) {
    const g = e.split('.')[0]!
    groups.set(g, [...(groups.get(g) ?? []), e])
  }
  return (
    <Dialog
      open
      wide
      title={hook ? hook.name : t('webhooks.create')}
      onClose={onClose}
      closeLabel={t('common.cancel')}
      footer={<DialogActions onCancel={onClose} onConfirm={() => save.mutate()} loading={save.isPending} />}
    >
      <div className="flex flex-col gap-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <TextField label={t('webhooks.name')} value={name} error={errors.name} onChange={(e) => setName(e.target.value)} />
          <TextField label={t('webhooks.url')} type="url" value={url} error={errors.url} placeholder="https://" onChange={(e) => setUrl(e.target.value)} />
        </div>
        <Checkbox label={t('webhooks.enabled')} checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        <fieldset className="flex flex-col gap-2">
          <legend className="mb-1 text-sm font-medium">{t('webhooks.headers')}</legend>
          {headers.map(([k, v], i) => (
            <div key={i} className="flex flex-col gap-2 sm:flex-row sm:items-end">
              <TextField label={t('webhooks.headerName')} value={k} onChange={(e) => setHeaders(headers.map((h, j) => (j === i ? [e.target.value, h[1]] : h)))} className="flex-1" />
              <TextField label={t('webhooks.headerValue')} value={v} onChange={(e) => setHeaders(headers.map((h, j) => (j === i ? [h[0], e.target.value] : h)))} className="flex-1" />
              <Button variant="destructive-outline" title={t('webhooks.removeHeader')} icon={<X className="h-4 w-4" />} onClick={() => setHeaders(headers.filter((_, j) => j !== i))} className="w-full sm:w-auto">
                {t('webhooks.removeHeader')}
              </Button>
            </div>
          ))}
          <div>
            <Button variant="outline" title={t('webhooks.addHeader')} icon={<Plus className="h-4 w-4" />} onClick={() => setHeaders([...headers, ['', '']])} className="w-full md:w-auto">
              {t('webhooks.addHeader')}
            </Button>
          </div>
        </fieldset>
        <fieldset>
          <legend className="mb-1 text-sm font-medium">{t('webhooks.events')}</legend>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {[...groups.entries()].map(([g, list]) => (
              <div key={g} className="rounded-md border border-border p-3">
                <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted">{g}</p>
                {list.map((e) => (
                  <Checkbox key={e} label={e} checked={chosen.includes(e)} onChange={(ev) => setChosen(ev.target.checked ? [...chosen, e] : chosen.filter((x) => x !== e))} />
                ))}
              </div>
            ))}
          </div>
        </fieldset>
      </div>
    </Dialog>
  )
}

export function Webhooks() {
  const { t } = useI18n()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const { permissions } = useAuth()
  const [editing, setEditing] = useState<Webhook | null | undefined>(undefined)
  const hooks = useQuery({ queryKey: ['webhooks', 'list'], queryFn: () => api.get<Webhook[]>('/webhooks') })
  const trigger = useMutation({
    mutationFn: (h: Webhook) => api.post<{ statusCode: number; message?: string }>(`/webhooks/${h.id}/trigger`),
    onSuccess: (r) =>
      r.message ? toast.error(t('webhooks.triggerFailed', { message: r.message })) : (r.statusCode >= 200 && r.statusCode < 300 ? toast.success : toast.error)(t('webhooks.triggered', { status: String(r.statusCode) })),
    onError: (e) => showError(e),
  })
  const remove = useMutation({
    mutationFn: (h: Webhook) => api.del(`/webhooks/${h.id}`),
    onSuccess: () => {
      toast.success(t('webhooks.deleted'))
      void queryClient.invalidateQueries({ queryKey: ['webhooks'] })
    },
    onError: (e) => showError(e),
  })
  const actions = (h: Webhook) => {
    const out = []
    if (can(permissions, 'admin::webhooks.update')) {
      out.push({ label: t('webhooks.edit'), icon: <Pencil className="h-4 w-4" />, onSelect: () => setEditing(h) })
      out.push({ label: t('webhooks.trigger'), icon: <Send className="h-4 w-4" />, onSelect: () => trigger.mutate(h) })
    }
    if (can(permissions, 'admin::webhooks.delete'))
      out.push({
        label: t('webhooks.delete'),
        icon: <Trash2 className="h-4 w-4" />,
        destructive: true,
        onSelect: async () => {
          if (await confirm({ title: t('webhooks.delete'), message: t('webhooks.deleteConfirm', { name: h.name }), confirmLabel: t('webhooks.delete'), destructive: true })) remove.mutate(h)
        },
      })
    return out
  }
  const status = (h: Webhook) => (
    <span className={`inline-flex rounded border px-1.5 py-0.5 text-xs font-medium ${h.isEnabled ? 'border-success/20 bg-success/10 text-success' : 'border-zinc-200 bg-zinc-100 text-zinc-700'}`}>
      {h.isEnabled ? t('webhooks.enabled') : t('webhooks.disabled')}
    </span>
  )
  const list = hooks.data ?? []
  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t('settings.webhooks')}
        actions={
          can(permissions, 'admin::webhooks.create') && (
            <Button title={t('webhooks.create')} icon={<Plus className="h-4 w-4" />} onClick={() => setEditing(null)} className="w-full md:w-auto">
              {t('webhooks.create')}
            </Button>
          )
        }
      />
      <div className="hidden md:block">
        <Table>
          <thead>
            <tr>
              <Th>{t('webhooks.name')}</Th>
              <Th>{t('webhooks.url')}</Th>
              <Th>{t('webhooks.events')}</Th>
              <Th>{t('users.status')}</Th>
              <Th className="w-16">
                <span className="sr-only">{t('cm.actions')}</span>
              </Th>
            </tr>
          </thead>
          <tbody>
            {list.map((h) => (
              <Tr key={h.id}>
                <Td className="font-medium">{h.name}</Td>
                <Td className="max-w-xs truncate text-muted">{h.url}</Td>
                <Td>{h.events.length}</Td>
                <Td>{status(h)}</Td>
                <Td className="text-right">{actions(h).length > 0 && <EntityActionsMenu actions={actions(h)} label={t('cm.actions')} />}</Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </div>
      <ul className="flex flex-col gap-3 md:hidden">
        {list.map((h) => (
          <li key={h.id} className="flex items-start justify-between gap-3 rounded-lg border border-border bg-surface p-4 shadow-sm">
            <div className="min-w-0">
              <p className="font-medium">{h.name}</p>
              <p className="truncate text-sm text-muted">{h.url}</p>
              <div className="mt-2">{status(h)}</div>
            </div>
            {actions(h).length > 0 && <EntityActionsMenu actions={actions(h)} label={t('cm.actions')} />}
          </li>
        ))}
      </ul>
      {!hooks.isPending && list.length === 0 && <Card className="text-center text-sm text-muted">{t('webhooks.empty')}</Card>}
      {editing !== undefined && <WebhookDialog hook={editing} onClose={() => setEditing(undefined)} />}
    </div>
  )
}
