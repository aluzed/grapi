import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Pencil, Save, Trash2, UserPlus } from 'lucide-react'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { EntityActionsMenu } from '@/components/ui/entity-actions-menu'
import { Checkbox, TextField } from '@/components/ui/field'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { api, ApiError, requestWithMeta } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import type { AdminUser, Pagination, RoleWithCount } from '@/lib/types'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'

const fullName = (u: AdminUser) => [u.firstname, u.lastname].filter(Boolean).join(' ') || u.email

function StatusTag({ u }: { u: AdminUser }) {
  const { t } = useI18n()
  const [text, style] = u.blocked
    ? [t('users.blocked'), 'bg-destructive/10 text-destructive border-destructive/20']
    : u.isActive
      ? [t('users.active'), 'bg-success/10 text-success border-success/20']
      : [t('users.pending'), 'bg-amber-50 text-amber-800 border-amber-200']
  return <span className={`inline-flex rounded border px-1.5 py-0.5 text-xs font-medium ${style}`}>{text}</span>
}

type FormState = { firstname: string; lastname: string; email: string; isActive: boolean; roles: number[] }

function UserDialog({ user, roles, onClose }: { user: AdminUser | null; roles: RoleWithCount[]; onClose: () => void }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const creating = user === null
  const [form, setForm] = useState<FormState>({
    firstname: user?.firstname ?? '',
    lastname: user?.lastname ?? '',
    email: user?.email ?? '',
    isActive: user?.isActive ?? true,
    roles: user?.roles.map((r) => r.id) ?? [],
  })
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [link, setLink] = useState<string | null>(null)
  const save = useMutation({
    mutationFn: () => {
      const body = { firstname: form.firstname, lastname: form.lastname, email: form.email, roles: form.roles }
      return creating ? api.post<{ registrationUrl: string }>('/users', body) : api.put(`/users/${user.id}`, { ...body, isActive: form.isActive })
    },
    onSuccess: (res) => {
      void queryClient.invalidateQueries({ queryKey: ['settings', 'users'] })
      if (creating) {
        toast.success(t('users.invited'))
        setLink((res as { registrationUrl: string }).registrationUrl)
      } else {
        toast.success(t('users.saved'))
        onClose()
      }
    },
    onError: (err) => {
      if (err instanceof ApiError) setErrors(Object.fromEntries(err.fieldErrors().map((e) => [e.path[e.path.length - 1] ?? '', e.message])))
      showError(err, undefined, t('common.error'))
    },
  })
  const submit = () => {
    if (form.roles.length === 0) {
      setErrors({ roles: t('users.rolesRequired') })
      return
    }
    save.mutate()
  }
  const title = creating ? t('users.invite') : fullName(user)
  if (link) {
    return (
      <Dialog
        open
        title={t('users.inviteLink')}
        onClose={onClose}
        closeLabel={t('users.close')}
        footer={
          <>
            <Button variant="outline" title={t('users.close')} onClick={onClose} className="w-full md:w-auto">
              {t('users.close')}
            </Button>
            <Button
              title={t('users.copy')}
              icon={<Copy className="h-4 w-4" />}
              className="w-full md:w-auto"
              onClick={() => {
                void navigator.clipboard?.writeText(link).then(() => toast.success(t('users.copied')))
              }}
            >
              {t('users.copy')}
            </Button>
          </>
        }
      >
        <p className="mb-3 text-sm text-muted">{t('users.inviteLinkHelp')}</p>
        <code className="block break-all rounded-md bg-background p-3 text-xs">{link}</code>
      </Dialog>
    )
  }
  return (
    <Dialog
      open
      title={title}
      onClose={onClose}
      closeLabel={t('common.cancel')}
      footer={
        <>
          <Button variant="outline" title={t('common.cancel')} onClick={onClose} className="w-full md:w-auto">
            {t('common.cancel')}
          </Button>
          <Button title={creating ? t('users.invite') : t('common.save')} icon={creating ? <UserPlus className="h-4 w-4" /> : <Save className="h-4 w-4" />} loading={save.isPending} onClick={submit} className="w-full md:w-auto">
            {creating ? t('users.invite') : t('common.save')}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <TextField label={t('auth.firstname')} value={form.firstname} error={errors.firstname} onChange={(e) => setForm({ ...form, firstname: e.target.value })} />
          <TextField label={t('auth.lastname')} value={form.lastname} onChange={(e) => setForm({ ...form, lastname: e.target.value })} />
        </div>
        <TextField label={t('users.email')} type="email" value={form.email} error={errors.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
        {!creating && <Checkbox label={t('users.isActive')} checked={form.isActive} onChange={(e) => setForm({ ...form, isActive: e.target.checked })} />}
        <fieldset>
          <legend className="mb-1 text-sm font-medium">{t('users.roles')}</legend>
          {roles.map((r) => (
            <Checkbox
              key={r.id}
              label={r.name}
              checked={form.roles.includes(r.id)}
              onChange={(e) => setForm({ ...form, roles: e.target.checked ? [...form.roles, r.id] : form.roles.filter((x) => x !== r.id) })}
            />
          ))}
          {errors.roles && <p className="text-xs text-destructive">{errors.roles}</p>}
        </fieldset>
      </div>
    </Dialog>
  )
}

export function Users() {
  const { t } = useI18n()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const { permissions, user: me } = useAuth()
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<AdminUser | null | undefined>(undefined)
  const q = new URLSearchParams({ page: String(page), pageSize: '20', sort: 'firstname:asc' })
  if (search) q.set('search', search)
  const users = useQuery({
    queryKey: ['settings', 'users', q.toString()],
    queryFn: () => requestWithMeta<AdminUser[], { pagination: Pagination }>(`/users?${q}`),
    placeholderData: (prev) => prev,
  })
  const roles = useQuery({ queryKey: ['settings', 'roles'], queryFn: () => api.get<RoleWithCount[]>('/roles'), enabled: can(permissions, 'admin::roles.read') })
  const remove = useMutation({
    mutationFn: (u: AdminUser) => api.del(`/users/${u.id}`),
    onSuccess: () => {
      toast.success(t('users.deleted'))
      void queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
    onError: (err) => showError(err, undefined, t('common.error')),
  })
  const list = users.data?.data ?? []
  const pagination = users.data?.meta.pagination
  const actions = (u: AdminUser) => {
    const out = []
    if (can(permissions, 'admin::users.update')) out.push({ label: t('users.edit'), icon: <Pencil className="h-4 w-4" />, onSelect: () => setEditing(u) })
    if (can(permissions, 'admin::users.delete') && u.id !== me?.id)
      out.push({
        label: t('users.delete'),
        icon: <Trash2 className="h-4 w-4" />,
        destructive: true,
        onSelect: async () => {
          if (await confirm({ title: t('users.delete'), message: t('users.deleteConfirm', { name: fullName(u) }), confirmLabel: t('users.delete'), destructive: true })) remove.mutate(u)
        },
      })
    return out
  }
  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t('settings.users')}
        subtitle={pagination ? String(pagination.total) : undefined}
        actions={
          can(permissions, 'admin::users.create') && (
            <Button title={t('users.invite')} icon={<UserPlus className="h-4 w-4" />} onClick={() => setEditing(null)} className="w-full md:w-auto">
              {t('users.invite')}
            </Button>
          )
        }
      />
      <input
        type="search"
        value={search}
        onChange={(e) => {
          setSearch(e.target.value)
          setPage(1)
        }}
        placeholder={t('users.search')}
        aria-label={t('users.search')}
        className="w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-primary focus:outline-none md:w-72"
      />
      <div className="hidden md:block">
        <Table>
          <thead>
            <tr>
              <Th>{t('users.name')}</Th>
              <Th>{t('users.email')}</Th>
              <Th>{t('users.roles')}</Th>
              <Th>{t('users.status')}</Th>
              <Th className="w-16">
                <span className="sr-only">{t('cm.actions')}</span>
              </Th>
            </tr>
          </thead>
          <tbody>
            {list.map((u) => (
              <Tr key={u.id}>
                <Td className="font-medium">{fullName(u)}</Td>
                <Td>{u.email}</Td>
                <Td>{u.roles.map((r) => r.name).join(', ')}</Td>
                <Td>
                  <StatusTag u={u} />
                </Td>
                <Td className="text-right">{actions(u).length > 0 && <EntityActionsMenu actions={actions(u)} label={t('cm.actions')} />}</Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </div>
      <ul className="flex flex-col gap-3 md:hidden">
        {list.map((u) => (
          <li key={u.id} className="flex items-start justify-between gap-3 rounded-lg border border-border bg-surface p-4 shadow-sm">
            <div className="min-w-0">
              <p className="font-medium">{fullName(u)}</p>
              <p className="truncate text-sm text-muted">{u.email}</p>
              <p className="mt-1 text-sm">{u.roles.map((r) => r.name).join(', ')}</p>
              <div className="mt-2">
                <StatusTag u={u} />
              </div>
            </div>
            {actions(u).length > 0 && <EntityActionsMenu actions={actions(u)} label={t('cm.actions')} />}
          </li>
        ))}
      </ul>
      {!users.isPending && list.length === 0 && <Card className="text-center text-sm text-muted">{t('users.empty')}</Card>}
      {pagination && pagination.pageCount > 1 && (
        <div className="flex flex-col gap-2 md:flex-row md:justify-end">
          <Button variant="outline" title={t('cm.previous')} disabled={page <= 1} onClick={() => setPage(page - 1)} className="w-full md:w-auto">
            {t('cm.previous')}
          </Button>
          <Button variant="outline" title={t('cm.next')} disabled={page >= pagination.pageCount} onClick={() => setPage(page + 1)} className="w-full md:w-auto">
            {t('cm.next')}
          </Button>
        </div>
      )}
      {editing !== undefined && <UserDialog user={editing} roles={roles.data ?? []} onClose={() => setEditing(undefined)} />}
    </div>
  )
}
