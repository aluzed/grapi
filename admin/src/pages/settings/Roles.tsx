import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Pencil, Plus, Trash2 } from 'lucide-react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { DialogActions } from '@/components/DialogActions'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import type { EntityAction } from '@/components/ui/entity-actions-menu'
import { TextField } from '@/components/ui/field'
import { api, ApiError } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import type { Permission, RoleWithCount } from '@/lib/types'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { RolesTable } from './RolesTable'

function CreateRole({ onClose }: { onClose: () => void }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState<string>()
  const create = useMutation({
    mutationFn: () => api.post<RoleWithCount>('/roles', { name, description }),
    onSuccess: (r) => {
      toast.success(t('roles.created'))
      void queryClient.invalidateQueries({ queryKey: ['settings', 'roles'] })
      navigate(`/settings/roles/${r.id}`)
    },
    onError: (err) => {
      if (err instanceof ApiError) setError(err.fieldErrors()[0]?.message)
      showError(err, undefined, t('common.error'))
    },
  })
  return (
    <Dialog
      open
      title={t('roles.create')}
      onClose={onClose}
      closeLabel={t('common.cancel')}
      footer={<DialogActions onCancel={onClose} onConfirm={() => create.mutate()} label={t('roles.create')} icon={<Plus className="h-4 w-4" />} loading={create.isPending} />}
    >
      <div className="flex flex-col gap-4">
        <TextField label={t('roles.name')} value={name} error={error} onChange={(e) => setName(e.target.value)} autoFocus />
        <TextField label={t('roles.description')} value={description} onChange={(e) => setDescription(e.target.value)} />
      </div>
    </Dialog>
  )
}

export function Roles() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const { permissions } = useAuth()
  const [creating, setCreating] = useState(false)
  const roles = useQuery({ queryKey: ['settings', 'roles'], queryFn: () => api.get<RoleWithCount[]>('/roles') })
  const remove = useMutation({
    mutationFn: (r: RoleWithCount) => api.del(`/roles/${r.id}`),
    onSuccess: () => {
      toast.success(t('roles.deleted'))
      void queryClient.invalidateQueries({ queryKey: ['settings', 'roles'] })
    },
    onError: (err) => showError(err, undefined, t('common.error')),
  })
  const duplicate = useMutation({
    mutationFn: async (r: RoleWithCount) => {
      const perms = await api.get<Permission[]>(`/roles/${r.id}/permissions`)
      const copy = await api.post<RoleWithCount>('/roles', { name: t('roles.copyName', { name: r.name }), description: r.description ?? '' })
      await api.put(`/roles/${copy.id}/permissions`, { permissions: perms.map(({ action, subject, properties, conditions }) => ({ action, subject, properties, conditions })) })
      return copy
    },
    onSuccess: (copy) => {
      toast.success(t('roles.duplicated', { name: copy.name }))
      void queryClient.invalidateQueries({ queryKey: ['settings', 'roles'] })
      navigate(`/settings/roles/${copy.id}`)
    },
    onError: (err) => showError(err, undefined, t('common.error')),
  })
  const actions = (r: RoleWithCount) => {
    const out: EntityAction[] = [{ label: t('roles.edit'), icon: <Pencil className="h-4 w-4" />, onSelect: () => navigate(`/settings/roles/${r.id}`) }]
    if (can(permissions, 'admin::roles.create') && r.code !== 'strapi-super-admin')
      out.push({ label: t('roles.duplicate'), icon: <Copy className="h-4 w-4" />, onSelect: () => duplicate.mutate(r) })
    if (can(permissions, 'admin::roles.delete') && r.code !== 'strapi-super-admin' && r.usersCount === 0)
      out.push({
        label: t('roles.delete'),
        icon: <Trash2 className="h-4 w-4" />,
        destructive: true,
        onSelect: async () => {
          if (await confirm({ title: t('roles.delete'), message: t('roles.deleteConfirm', { name: r.name }), confirmLabel: t('roles.delete'), destructive: true })) remove.mutate(r)
        },
      })
    return out
  }
  const list = roles.data ?? []
  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t('settings.roles')}
        actions={
          can(permissions, 'admin::roles.create') && (
            <Button title={t('roles.create')} icon={<Plus className="h-4 w-4" />} onClick={() => setCreating(true)} className="w-full md:w-auto">
              {t('roles.create')}
            </Button>
          )
        }
      />
      <RolesTable roles={list} actions={actions} usersLabel={(n) => t('roles.users', { count: String(n) })} />
      {creating && <CreateRole onClose={() => setCreating(false)} />}
    </div>
  )
}
