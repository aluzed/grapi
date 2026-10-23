import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Save } from 'lucide-react'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Card, CardHeader } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { Checkbox, SelectField, TextField } from '@/components/ui/field'
import { api } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'

type Settings = Record<string, unknown>

function useSettings(path: string) {
  const queryClient = useQueryClient()
  const { t } = useI18n()
  const query = useQuery({ queryKey: ['up', path], queryFn: () => api.get<Settings>(`/users-permissions/${path}`) })
  const save = useMutation({
    mutationFn: (body: Settings) => api.put<Settings>(`/users-permissions/${path}`, body),
    onSuccess: () => {
      toast.success(t('up.saved'))
      void queryClient.invalidateQueries({ queryKey: ['up', path] })
    },
    onError: (e) => showError(e, undefined, t('common.error')),
  })
  return { query, save }
}

export function UPAdvanced() {
  const { t } = useI18n()
  const { permissions } = useAuth()
  const { query, save } = useSettings('advanced')
  const roles = useQuery({ queryKey: ['up', 'roles'], queryFn: () => api.get<{ type: string; name: string }[]>('/users-permissions/roles') })
  const [form, setForm] = useState<Settings>({})
  useEffect(() => {
    if (query.data) setForm(query.data)
  }, [query.data])
  const editable = can(permissions, 'plugin::users-permissions.advanced-settings.update')
  const bool = (k: 'unique_email' | 'allow_register' | 'email_confirmation') => (
    <Checkbox label={t(`up.adv.${k}`)} checked={Boolean(form[k])} onChange={(e) => setForm({ ...form, [k]: e.target.checked })} />
  )
  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t('up.advanced')}
        actions={
          editable && (
            <Button title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={save.isPending} onClick={() => save.mutate(form)} className="w-full md:w-auto">
              {t('common.save')}
            </Button>
          )
        }
      />
      <Card>
        <fieldset disabled={!editable} className="flex flex-col gap-4">
          <SelectField label={t('up.adv.default_role')} value={String(form.default_role ?? '')} onChange={(e) => setForm({ ...form, default_role: e.target.value })} className="md:w-80">
            {roles.data?.map((r) => (
              <option key={r.type} value={r.type}>
                {r.name}
              </option>
            ))}
          </SelectField>
          {bool('unique_email')}
          {bool('allow_register')}
          {bool('email_confirmation')}
          <TextField label={t('up.adv.email_confirmation_redirection')} type="url" value={String(form.email_confirmation_redirection ?? '')} onChange={(e) => setForm({ ...form, email_confirmation_redirection: e.target.value })} />
          <TextField label={t('up.adv.email_reset_password')} type="url" value={String(form.email_reset_password ?? '')} onChange={(e) => setForm({ ...form, email_reset_password: e.target.value })} />
        </fieldset>
      </Card>
    </div>
  )
}

type Template = { options?: { from?: { name?: string; email?: string }; response_email?: string; object?: string; message?: string } }

export function UPEmails() {
  const { t } = useI18n()
  const { permissions } = useAuth()
  const { query, save } = useSettings('email-templates')
  const [form, setForm] = useState<Record<string, Template>>({})
  useEffect(() => {
    if (query.data) setForm(query.data as Record<string, Template>)
  }, [query.data])
  const editable = can(permissions, 'plugin::users-permissions.email-templates.update')
  const set = (key: string, patch: NonNullable<Template['options']>) => setForm({ ...form, [key]: { ...form[key], options: { ...form[key]?.options, ...patch } } })
  return (
    <div className="flex flex-col gap-4">
      <PageHeader title={t('up.emails')} />
      {(['email_confirmation', 'reset_password'] as const).map((key) => {
        const o = form[key]?.options ?? {}
        return (
          <Card key={key}>
            <CardHeader
              title={t(`up.email.${key}`)}
              actions={
                editable && (
                  <Button title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={save.isPending} onClick={() => save.mutate({ [key]: form[key] })} className="w-full md:w-auto">
                    {t('common.save')}
                  </Button>
                )
              }
            />
            <fieldset disabled={!editable} className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <TextField label={t('up.email.fromName')} value={o.from?.name ?? ''} onChange={(e) => set(key, { from: { ...o.from, name: e.target.value } })} />
              <TextField label={t('up.email.fromEmail')} type="email" value={o.from?.email ?? ''} onChange={(e) => set(key, { from: { ...o.from, email: e.target.value } })} />
              <TextField label={t('up.email.replyTo')} type="email" value={o.response_email ?? ''} onChange={(e) => set(key, { response_email: e.target.value })} />
              <TextField label={t('up.email.object')} value={o.object ?? ''} onChange={(e) => set(key, { object: e.target.value })} />
              <label className="flex flex-col gap-1.5 text-sm font-medium sm:col-span-2">
                {t('up.email.message')}
                <textarea
                  rows={8}
                  value={o.message ?? ''}
                  onChange={(e) => set(key, { message: e.target.value })}
                  className="block w-full rounded-md border border-border bg-surface px-3 py-2 font-mono text-xs font-normal focus:border-primary focus:outline-none"
                />
              </label>
            </fieldset>
          </Card>
        )
      })}
    </div>
  )
}

type Provider = { enabled?: boolean; key?: string; secret?: string; callback?: string; callbackUrl?: string; scope?: string[] }

export function UPProviders() {
  const { t } = useI18n()
  const { permissions } = useAuth()
  const { query, save } = useSettings('providers')
  const [editing, setEditing] = useState<{ name: string; p: Provider } | null>(null)
  const editable = can(permissions, 'plugin::users-permissions.providers.update')
  const list = Object.entries((query.data ?? {}) as Record<string, Provider>).sort(([a], [b]) => a.localeCompare(b))
  return (
    <div className="flex flex-col gap-4">
      <PageHeader title={t('up.providers')} />
      <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {list.map(([name, p]) => (
          <li key={name} className="flex items-center justify-between gap-3 rounded-lg border border-border bg-surface p-4 shadow-sm">
            <div>
              <p className="font-medium capitalize">{name}</p>
              <span className={`mt-1 inline-flex rounded border px-1.5 py-0.5 text-xs font-medium ${p.enabled ? 'border-success/20 bg-success/10 text-success' : 'border-zinc-200 bg-zinc-100 text-zinc-700'}`}>
                {p.enabled ? t('up.provider.on') : t('up.provider.off')}
              </span>
            </div>
            {editable && name !== 'email' && (
              <Button variant="outline" title={`${t('up.provider.edit')} ${name}`} icon={<Pencil className="h-4 w-4" />} onClick={() => setEditing({ name, p: { ...p } })}>
                {t('up.provider.edit')}
              </Button>
            )}
            {editable && name === 'email' && <Checkbox label={t('up.provider.enabled')} checked={Boolean(p.enabled)} onChange={(e) => save.mutate({ email: { enabled: e.target.checked } })} />}
          </li>
        ))}
      </ul>
      {editing && (
        <Dialog
          open
          title={editing.name}
          onClose={() => setEditing(null)}
          closeLabel={t('common.cancel')}
          footer={
            <>
              <Button variant="outline" title={t('common.cancel')} onClick={() => setEditing(null)} className="w-full md:w-auto">
                {t('common.cancel')}
              </Button>
              <Button
                title={t('common.save')}
                icon={<Save className="h-4 w-4" />}
                loading={save.isPending}
                className="w-full md:w-auto"
                onClick={() => save.mutate({ [editing.name]: editing.p }, { onSuccess: () => setEditing(null) })}
              >
                {t('common.save')}
              </Button>
            </>
          }
        >
          <div className="flex flex-col gap-4">
            <Checkbox label={t('up.provider.enabled')} checked={Boolean(editing.p.enabled)} onChange={(e) => setEditing({ ...editing, p: { ...editing.p, enabled: e.target.checked } })} />
            <TextField label={t('up.provider.key')} value={editing.p.key ?? ''} autoComplete="off" onChange={(e) => setEditing({ ...editing, p: { ...editing.p, key: e.target.value } })} />
            <TextField label={t('up.provider.secret')} type="password" value={editing.p.secret ?? ''} autoComplete="off" onChange={(e) => setEditing({ ...editing, p: { ...editing.p, secret: e.target.value } })} />
            <TextField label={t('up.provider.callback')} type="url" value={editing.p.callback ?? ''} onChange={(e) => setEditing({ ...editing, p: { ...editing.p, callback: e.target.value } })} />
          </div>
        </Dialog>
      )}
    </div>
  )
}
