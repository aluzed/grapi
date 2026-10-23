import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { EntityActionsMenu } from '@/components/ui/entity-actions-menu'
import { TextField } from '@/components/ui/field'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { api, ApiError } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import type { RoleWithCount } from '@/lib/types'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'

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
      footer={
        <>
          <Button variant="outline" title={t('common.cancel')} onClick={onClose} className="w-full md:w-auto">
            {t('common.cancel')}
          </Button>
          <Button title={t('roles.create')} icon={<Plus className="h-4 w-4" />} loading={create.isPending} onClick={() => create.mutate()} className="w-full md:w-auto">
            {t('roles.create')}
          </Button>
        </>
      }
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
  const actions = (r: RoleWithCount) => {
    const out = [{ label: t('roles.edit'), icon: <Pencil className="h-4 w-4" />, onSelect: () => navigate(`/settings/roles/${r.id}`) }]
    if (can(permissions, 'admin::roles.delete') && r.code !== 'strapi-super-admin' && r.usersCount === 0)
      out.push({
        label: t('roles.delete'),
        icon: <Trash2 className="h-4 w-4" />,
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
      <div className="hidden md:block">
        <Table>
          <thead>
            <tr>
              <Th>{t('roles.name')}</Th>
              <Th>{t('roles.description')}</Th>
              <Th>{t('settings.users')}</Th>
              <Th className="w-16">
                <span className="sr-only">{t('cm.actions')}</span>
              </Th>
            </tr>
          </thead>
          <tbody>
            {list.map((r) => (
              <Tr key={r.id}>
                <Td className="font-medium">{r.name}</Td>
                <Td className="text-muted">{r.description}</Td>
                <Td>{r.usersCount}</Td>
                <Td className="text-right">
                  <EntityActionsMenu actions={actions(r)} label={t('cm.actions')} />
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </div>
      <ul className="flex flex-col gap-3 md:hidden">
        {list.map((r) => (
          <li key={r.id} className="flex items-start justify-between gap-3 rounded-lg border border-border bg-surface p-4 shadow-sm">
            <div className="min-w-0">
              <p className="font-medium">{r.name}</p>
              <p className="text-sm text-muted">{r.description}</p>
              <p className="mt-1 text-xs text-muted">{t('roles.users', { count: String(r.usersCount) })}</p>
            </div>
            <EntityActionsMenu actions={actions(r)} label={t('cm.actions')} />
          </li>
        ))}
      </ul>
      {creating && <CreateRole onClose={() => setCreating(false)} />}
    </div>
  )
}
