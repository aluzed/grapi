import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, KeyRound, Pencil, Plus, RefreshCw, Save, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { EntityActionsMenu } from '@/components/ui/entity-actions-menu'
import { Checkbox, SelectField, TextField } from '@/components/ui/field'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { api, ApiError } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'

type Token = {
  id: number
  name: string
  description: string
  type: 'read-only' | 'full-access' | 'custom'
  expiresAt: string | null
  lastUsedAt: string | null
  createdAt: string
  permissions: string[]
  accessKey?: string
}

function KeyDialog({ value, onClose }: { value: string; onClose: () => void }) {
  const { t } = useI18n()
  return (
    <Dialog
      open
      title={t('tokens.yourToken')}
      onClose={onClose}
      closeLabel={t('users.close')}
      footer={
        <>
          <Button variant="outline" title={t('users.close')} onClick={onClose} className="w-full md:w-auto">
            {t('users.close')}
          </Button>
          <Button title={t('tokens.copy')} icon={<Copy className="h-4 w-4" />} onClick={() => void navigator.clipboard?.writeText(value).then(() => toast.success(t('tokens.copied')))} className="w-full md:w-auto">
            {t('tokens.copy')}
          </Button>
        </>
      }
    >
      <p className="mb-3 text-sm text-muted">{t('tokens.copyNow')}</p>
      <code className="block break-all rounded-md bg-background p-3 text-xs">{value}</code>
    </Dialog>
  )
}

// Actions grouped by their content type: api::article.article.find goes
// under api::article.article.
function groupActions(actions: string[]) {
  const groups = new Map<string, string[]>()
  for (const a of actions) {
    const i = a.lastIndexOf('.')
    const g = a.slice(0, i)
    groups.set(g, [...(groups.get(g) ?? []), a])
  }
  return [...groups.entries()]
}

function TokenDialog({ token, onClose, onKey }: { token: Token | null; onClose: () => void; onKey: (k: string) => void }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const actions = useQuery({ queryKey: ['tokens', 'actions'], queryFn: () => api.get<string[]>('/api-tokens/actions'), staleTime: 60_000 })
  const full = useQuery({ queryKey: ['tokens', token?.id], queryFn: () => api.get<Token>(`/api-tokens/${token!.id}`), enabled: token !== null })
  const [form, setForm] = useState({ name: token?.name ?? '', description: token?.description ?? '', type: token?.type ?? 'read-only', lifespan: '30' })
  const [perms, setPerms] = useState<Set<string> | null>(null)
  const current = perms ?? new Set(full.data?.permissions ?? [])
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useMutation({
    mutationFn: () => {
      const body = { name: form.name, description: form.description, type: form.type, permissions: form.type === 'custom' ? [...current] : [] }
      return token ? api.put<Token>(`/api-tokens/${token.id}`, body) : api.post<Token>('/api-tokens', { ...body, lifespan: Number(form.lifespan) })
    },
    onSuccess: (res) => {
      void queryClient.invalidateQueries({ queryKey: ['tokens'] })
      toast.success(t('tokens.saved'))
      onClose()
      if (!token && res.accessKey) onKey(res.accessKey)
    },
    onError: (err) => {
      if (err instanceof ApiError) setErrors(Object.fromEntries(err.fieldErrors().map((e) => [e.path[0] ?? '', e.message])))
      showError(err, undefined, t('common.error'))
    },
  })
  const toggle = (list: string[], on: boolean) => {
    const next = new Set(current)
    for (const a of list) {
      if (on) next.add(a)
      else next.delete(a)
    }
    setPerms(next)
  }
  return (
    <Dialog
      open
      wide
      title={token ? token.name : t('tokens.create')}
      onClose={onClose}
      closeLabel={t('common.cancel')}
      footer={
        <>
          <Button variant="outline" title={t('common.cancel')} onClick={onClose} className="w-full md:w-auto">
            {t('common.cancel')}
          </Button>
          <Button title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={save.isPending} onClick={() => save.mutate()} className="w-full md:w-auto">
            {t('common.save')}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <TextField label={t('tokens.name')} value={form.name} error={errors.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <TextField label={t('tokens.description')} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          {!token && (
            <SelectField label={t('tokens.lifespan')} value={form.lifespan} onChange={(e) => setForm({ ...form, lifespan: e.target.value })}>
              {(['7', '30', '90', '0'] as const).map((d) => (
                <option key={d} value={d}>
                  {t(`tokens.lifespan.${d}`)}
                </option>
              ))}
            </SelectField>
          )}
          <SelectField label={t('tokens.type')} value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value as Token['type'] })}>
            {(['read-only', 'full-access', 'custom'] as const).map((k) => (
              <option key={k} value={k}>
                {t(`tokens.type.${k}`)}
              </option>
            ))}
          </SelectField>
        </div>
        {form.type === 'custom' && (
          <fieldset>
            <legend className="mb-2 text-sm font-medium">{t('tokens.permissions')}</legend>
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              {groupActions(actions.data ?? []).map(([group, list]) => (
                <div key={group} className="rounded-md border border-border p-3">
                  <Checkbox label={group} checked={list.every((a) => current.has(a))} onChange={(e) => toggle(list, e.target.checked)} className="font-medium" />
                  <div className="grid grid-cols-2 pl-6">
                    {list.map((a) => (
                      <Checkbox key={a} label={a.slice(a.lastIndexOf('.') + 1)} checked={current.has(a)} onChange={(e) => toggle([a], e.target.checked)} />
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </fieldset>
        )}
      </div>
    </Dialog>
  )
}

export function ApiTokens() {
  const { t, language } = useI18n()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const { permissions } = useAuth()
  const [editing, setEditing] = useState<Token | null | undefined>(undefined)
  const [key, setKey] = useState<string | null>(null)
  const tokens = useQuery({ queryKey: ['tokens', 'list'], queryFn: () => api.get<Token[]>('/api-tokens') })
  const date = (v: string | null) => (v ? new Date(v).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'short' }) : t('tokens.never'))
  const regenerate = useMutation({
    mutationFn: (tk: Token) => api.post<{ accessKey: string }>(`/api-tokens/${tk.id}/regenerate`),
    onSuccess: (r) => setKey(r.accessKey),
    onError: (e) => showError(e),
  })
  const remove = useMutation({
    mutationFn: (tk: Token) => api.del(`/api-tokens/${tk.id}`),
    onSuccess: () => {
      toast.success(t('tokens.deleted'))
      void queryClient.invalidateQueries({ queryKey: ['tokens'] })
    },
    onError: (e) => showError(e),
  })
  const actions = (tk: Token) => {
    const out = []
    if (can(permissions, 'admin::api-tokens.update')) out.push({ label: t('tokens.edit'), icon: <Pencil className="h-4 w-4" />, onSelect: () => setEditing(tk) })
    if (can(permissions, 'admin::api-tokens.regenerate'))
      out.push({
        label: t('tokens.regenerate'),
        icon: <RefreshCw className="h-4 w-4" />,
        onSelect: async () => {
          if (await confirm({ title: t('tokens.regenerate'), message: t('tokens.regenerateConfirm'), confirmLabel: t('tokens.regenerate'), destructive: true })) regenerate.mutate(tk)
        },
      })
    if (can(permissions, 'admin::api-tokens.delete'))
      out.push({
        label: t('tokens.delete'),
        icon: <Trash2 className="h-4 w-4" />,
        destructive: true,
        onSelect: async () => {
          if (await confirm({ title: t('tokens.delete'), message: t('tokens.deleteConfirm', { name: tk.name }), confirmLabel: t('tokens.delete'), destructive: true })) remove.mutate(tk)
        },
      })
    return out
  }
  const list = tokens.data ?? []
  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t('settings.apiTokens')}
        actions={
          can(permissions, 'admin::api-tokens.create') && (
            <Button title={t('tokens.create')} icon={<Plus className="h-4 w-4" />} onClick={() => setEditing(null)} className="w-full md:w-auto">
              {t('tokens.create')}
            </Button>
          )
        }
      />
      <div className="hidden md:block">
        <Table>
          <thead>
            <tr>
              <Th>{t('tokens.name')}</Th>
              <Th>{t('tokens.type')}</Th>
              <Th>{t('tokens.expires')}</Th>
              <Th>{t('tokens.lastUsed')}</Th>
              <Th className="w-16">
                <span className="sr-only">{t('cm.actions')}</span>
              </Th>
            </tr>
          </thead>
          <tbody>
            {list.map((tk) => (
              <Tr key={tk.id}>
                <Td>
                  <span className="flex items-center gap-2 font-medium">
                    <KeyRound className="h-4 w-4 text-muted" aria-hidden />
                    {tk.name}
                  </span>
                  {tk.description && <span className="block text-xs text-muted">{tk.description}</span>}
                </Td>
                <Td>{t(`tokens.type.${tk.type}`)}</Td>
                <Td>{date(tk.expiresAt)}</Td>
                <Td>{date(tk.lastUsedAt)}</Td>
                <Td className="text-right">{actions(tk).length > 0 && <EntityActionsMenu actions={actions(tk)} label={t('cm.actions')} />}</Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </div>
      <ul className="flex flex-col gap-3 md:hidden">
        {list.map((tk) => (
          <li key={tk.id} className="flex items-start justify-between gap-3 rounded-lg border border-border bg-surface p-4 shadow-sm">
            <div className="min-w-0">
              <p className="font-medium">{tk.name}</p>
              <p className="text-sm text-muted">{t(`tokens.type.${tk.type}`)}</p>
              <p className="text-xs text-muted">
                {t('tokens.expires')} : {date(tk.expiresAt)}
              </p>
            </div>
            {actions(tk).length > 0 && <EntityActionsMenu actions={actions(tk)} label={t('cm.actions')} />}
          </li>
        ))}
      </ul>
      {!tokens.isPending && list.length === 0 && <Card className="text-center text-sm text-muted">{t('tokens.empty')}</Card>}
      {editing !== undefined && <TokenDialog token={editing} onClose={() => setEditing(undefined)} onKey={setKey} />}
      {key && <KeyDialog value={key} onClose={() => setKey(null)} />}
    </div>
  )
}
