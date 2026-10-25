import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Save, Trash2 } from 'lucide-react'
import { useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { BackButton } from '@/components/BackButton'
import { Button } from '@/components/ui/button'
import { Card, CardHeader } from '@/components/ui/card'
import type { EntityAction } from '@/components/ui/entity-actions-menu'
import { Checkbox, TextField } from '@/components/ui/field'
import { api } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { RolesTable } from '../RolesTable'

type UPRole = { id: number; name: string; description: string; type: string; usersCount: number; permissions?: string[] }

const base = '/settings/users-permissions/roles'

export function UPRoles() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const { permissions } = useAuth()
  const roles = useQuery({ queryKey: ['up', 'roles'], queryFn: () => api.get<UPRole[]>('/users-permissions/roles') })
  const remove = useMutation({
    mutationFn: (r: UPRole) => api.del(`/users-permissions/roles/${r.id}`),
    onSuccess: () => {
      toast.success(t('up.role.deleted'))
      void queryClient.invalidateQueries({ queryKey: ['up'] })
    },
    onError: (e) => showError(e),
  })
  const actions = (r: UPRole) => {
    const out: EntityAction[] = [{ label: t('roles.edit'), icon: <Pencil className="h-4 w-4" />, onSelect: () => navigate(`${base}/${r.id}`) }]
    if (can(permissions, 'plugin::users-permissions.roles.delete') && r.type !== 'public' && r.type !== 'authenticated')
      out.push({
        label: t('roles.delete'),
        icon: <Trash2 className="h-4 w-4" />,
        destructive: true,
        onSelect: async () => {
          if (await confirm({ title: t('roles.delete'), message: t('up.role.deleteConfirm', { name: r.name }), confirmLabel: t('roles.delete'), destructive: true })) remove.mutate(r)
        },
      })
    return out
  }
  const list = roles.data ?? []
  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t('up.roles')}
        actions={
          can(permissions, 'plugin::users-permissions.roles.create') && (
            <Button title={t('up.role.create')} icon={<Plus className="h-4 w-4" />} onClick={() => navigate(`${base}/new`)} className="w-full md:w-auto">
              {t('up.role.create')}
            </Button>
          )
        }
      />
      <RolesTable roles={list} actions={actions} usersLabel={(n) => t('up.role.users', { count: String(n) })} />
    </div>
  )
}

function groups(actions: string[]) {
  const out = new Map<string, string[]>()
  for (const a of actions) {
    const i = a.lastIndexOf('.')
    const g = a.slice(0, i)
    out.set(g, [...(out.get(g) ?? []), a])
  }
  return [...out.entries()].sort(([a], [b]) => (a.startsWith('api::') === b.startsWith('api::') ? a.localeCompare(b) : a.startsWith('api::') ? -1 : 1))
}

export function UPRoleEditor() {
  const { id } = useParams()
  const creating = id === 'new'
  const { t } = useI18n()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { permissions } = useAuth()
  const role = useQuery({ queryKey: ['up', 'roles', id], queryFn: () => api.get<UPRole>(`/users-permissions/roles/${id}`), enabled: !creating })
  const actions = useQuery({ queryKey: ['up', 'actions'], queryFn: () => api.get<string[]>('/users-permissions/actions'), staleTime: 60_000 })
  const routes = useQuery({ queryKey: ['up', 'routes'], queryFn: () => api.get<Record<string, { method: string; path: string }[]>>('/users-permissions/routes'), staleTime: 60_000 })
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [chosen, setChosen] = useState<Set<string>>(new Set())
  useEffect(() => {
    if (role.data) {
      setName(role.data.name)
      setDescription(role.data.description ?? '')
      setChosen(new Set(role.data.permissions ?? []))
    }
  }, [role.data])
  const editable = creating ? can(permissions, 'plugin::users-permissions.roles.create') : can(permissions, 'plugin::users-permissions.roles.update')
  const save = useMutation({
    mutationFn: () => {
      const body = { name, description, permissions: [...chosen] }
      return creating ? api.post<UPRole>('/users-permissions/roles', body) : api.put<UPRole>(`/users-permissions/roles/${id}`, body)
    },
    onSuccess: (r) => {
      toast.success(t('up.role.saved'))
      void queryClient.invalidateQueries({ queryKey: ['up'] })
      if (creating) navigate(`${base}/${r.id}`, { replace: true })
    },
    onError: (e) => showError(e, undefined, t('common.error')),
  })
  const toggle = (list: string[], on: boolean) =>
    setChosen((prev) => {
      const next = new Set(prev)
      for (const a of list) {
        if (on) next.add(a)
        else next.delete(a)
      }
      return next
    })
  return (
    <div className="flex flex-col gap-6">
      <div className="space-y-4">
        <BackButton label={t('up.role.back')} onClick={() => navigate(base)} />
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <h1 className="text-2xl font-semibold tracking-tight">{creating ? t('up.role.create') : (role.data?.name ?? '-')}</h1>
          {editable && (
            <Button title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={save.isPending} onClick={() => save.mutate()} className="w-full md:w-auto">
              {t('common.save')}
            </Button>
          )}
        </div>
      </div>
      <Card>
        <CardHeader title={t('roles.details')} />
        <fieldset disabled={!editable} className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <TextField label={t('roles.name')} value={name} onChange={(e) => setName(e.target.value)} />
          <TextField label={t('roles.description')} value={description} onChange={(e) => setDescription(e.target.value)} />
        </fieldset>
      </Card>
      <Card>
        <CardHeader title={t('up.actions')} />
        <fieldset disabled={!editable} className="grid grid-cols-1 gap-3 md:grid-cols-2">
          {groups(actions.data ?? []).map(([g, list]) => (
            <div key={g} className="rounded-md border border-border p-3">
              <Checkbox label={g} title={t('up.selectGroup', { name: g })} checked={list.every((a) => chosen.has(a))} onChange={(e) => toggle(list, e.target.checked)} className="font-medium" />
              <div className="grid grid-cols-1 pl-6 sm:grid-cols-2">
                {list.map((a) => (
                  <div key={a} className="flex flex-col">
                    <Checkbox label={a.slice(a.lastIndexOf('.') + 1)} checked={chosen.has(a)} onChange={(e) => toggle([a], e.target.checked)} />
                    {routes.data?.[a]?.map((r) => (
                      <span key={`${r.method} ${r.path}`} className="-mt-2 mb-1 pl-6 font-mono text-[11px] break-all text-muted">
                        {r.method} {r.path}
                      </span>
                    ))}
                  </div>
                ))}
              </div>
            </div>
          ))}
        </fieldset>
      </Card>
    </div>
  )
}
