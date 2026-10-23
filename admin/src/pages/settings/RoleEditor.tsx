import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Save, SlidersHorizontal } from 'lucide-react'
import { useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardHeader } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { Checkbox, TextField } from '@/components/ui/field'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { Tabs } from '@/components/ui/tabs'
import { api } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import type { Catalog, CatalogAction, Permission, RoleWithCount } from '@/lib/types'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'
import { useLocales, useModels } from '../content-manager/hooks'

const verbs = ['create', 'read', 'update', 'delete', 'publish'] as const
const fieldVerbs = ['create', 'read', 'update'] as const
const systemFields = new Set(['id', 'documentId', 'createdAt', 'updatedAt', 'publishedAt', 'createdBy', 'updatedBy', 'locale', 'localizations'])
const explorer = 'plugin::content-manager.explorer.'
const isCreator = 'admin::is-creator'

const key = (action: string, subject: string | null) => `${action}|${subject ?? ''}`

export function RoleEditor() {
  const { id } = useParams()
  const { t } = useI18n()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { permissions: mine } = useAuth()
  const models = useModels().data ?? []
  const role = useQuery({ queryKey: ['settings', 'roles', id], queryFn: () => api.get<RoleWithCount>(`/roles/${id}`) })
  const rolePerms = useQuery({ queryKey: ['settings', 'roles', id, 'permissions'], queryFn: () => api.get<Permission[]>(`/roles/${id}/permissions`) })
  const catalog = useQuery({ queryKey: ['settings', 'catalog'], queryFn: () => api.get<Catalog>('/permissions') })
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [perms, setPerms] = useState<Map<string, Permission>>(new Map())
  const [tab, setTab] = useState('contentTypes')
  const isSuper = role.data?.code === 'strapi-super-admin'
  const editable = !isSuper && can(mine, 'admin::roles.update')

  useEffect(() => {
    if (role.data) {
      setName(role.data.name)
      setDescription(role.data.description ?? '')
    }
  }, [role.data])
  useEffect(() => {
    if (rolePerms.data) setPerms(new Map(rolePerms.data.map((p) => [key(p.action, p.subject), p])))
  }, [rolePerms.data])

  const subjects = useMemo(() => {
    const set = new Set<string>()
    for (const a of catalog.data?.sections.contentTypes ?? []) for (const s of a.subjects ?? []) set.add(s)
    return [...set].sort((a, b) => displayName(a).localeCompare(displayName(b)))
  }, [catalog.data, models])
  function displayName(uid: string) {
    return models.find((m) => m.uid === uid)?.info.displayName ?? uid
  }

  const has = (action: string, subject: string | null) => perms.has(key(action, subject))
  const set = (action: string, subject: string | null, on: boolean, conditions?: string[]) =>
    setPerms((prev) => {
      const next = new Map(prev)
      const k = key(action, subject)
      if (on) next.set(k, prev.get(k) ?? { action, subject, properties: {}, conditions: conditions ?? [] })
      else next.delete(k)
      return next
    })
  const ownOnly = (subject: string) => verbs.some((v) => perms.get(key(explorer + v, subject))?.conditions.includes(isCreator))
  const setOwnOnly = (subject: string, on: boolean) =>
    setPerms((prev) => {
      const next = new Map(prev)
      for (const v of verbs) {
        const p = next.get(key(explorer + v, subject))
        if (p) next.set(key(explorer + v, subject), { ...p, conditions: on ? [isCreator] : [] })
      }
      return next
    })

  const [restricting, setRestricting] = useState<string | null>(null)
  const locales = useLocales(true).data ?? []
  const modelOf = (uid: string) => models.find((m) => m.uid === uid)
  const withProps = (action: string, subject: string, change: (p: Permission['properties']) => Permission['properties']) =>
    setPerms((prev) => {
      const k = key(action, subject)
      const p = prev.get(k)
      if (!p) return prev
      const next = new Map(prev)
      next.set(k, { ...p, properties: change(p.properties ?? {}) })
      return next
    })
  // An absent list means everything: a full selection goes back to absent.
  const toggleIn = (list: string[] | null | undefined, all: string[], item: string, on: boolean) => {
    const current = list ?? all
    const out = on ? [...new Set([...current, item])] : current.filter((x) => x !== item)
    return all.every((x) => out.includes(x)) ? undefined : all.filter((x) => out.includes(x))
  }
  const restrictionSummary = (s: string) => {
    const parts: string[] = []
    const fields = fieldVerbs.some((v) => perms.get(key(explorer + v, s))?.properties?.fields)
    const locs = verbs.some((v) => perms.get(key(explorer + v, s))?.properties?.locales)
    if (fields) parts.push(t('roles.someFields'))
    if (locs) parts.push(t('roles.someLocales'))
    return parts.join(' · ')
  }

  const save = useMutation({
    mutationFn: async () => {
      await api.put(`/roles/${id}`, { name, description })
      return api.put(`/roles/${id}/permissions`, { permissions: [...perms.values()] })
    },
    onSuccess: () => {
      toast.success(t('roles.saved'))
      void queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
    onError: (err) => showError(err, undefined, t('common.error')),
  })

  const grouped = (list: CatalogAction[]) => {
    const groups = new Map<string, CatalogAction[]>()
    for (const a of list) {
      const g = `${a.plugin}${a.category ? ` · ${a.category}` : ''}`
      groups.set(g, [...(groups.get(g) ?? []), a])
    }
    return [...groups.entries()].sort(([a], [b]) => a.localeCompare(b))
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="space-y-4">
        <div>
          <Button variant="outline" title={t('roles.back')} icon={<ArrowLeft className="h-4 w-4" />} onClick={() => navigate('/settings/roles')} className="w-full md:w-auto">
            {t('roles.back')}
          </Button>
        </div>
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <h1 className="text-2xl font-semibold tracking-tight">{role.data?.name ?? '-'}</h1>
          {editable && (
            <Button title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={save.isPending} onClick={() => save.mutate()} className="w-full md:w-auto">
              {t('common.save')}
            </Button>
          )}
        </div>
      </div>
      {isSuper && <Card className="text-sm text-muted">{t('roles.superAdmin')}</Card>}
      <Card>
        <CardHeader title={t('roles.details')} />
        <fieldset disabled={!editable} className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <TextField label={t('roles.name')} value={name} onChange={(e) => setName(e.target.value)} />
          <TextField label={t('roles.description')} value={description} onChange={(e) => setDescription(e.target.value)} />
        </fieldset>
      </Card>
      {!isSuper && (
        <Card>
          <CardHeader title={t('roles.permissions')} />
          <div className="mb-4">
            <Tabs
              tabs={[
                { id: 'contentTypes', label: t('roles.tab.contentTypes') },
                { id: 'plugins', label: t('roles.tab.plugins') },
                { id: 'settings', label: t('roles.tab.settings') },
              ]}
              value={tab}
              onChange={setTab}
            />
          </div>
          <fieldset disabled={!editable}>
            {tab === 'contentTypes' && (
              <>
                <div className="hidden md:block">
                  <Table>
                    <thead>
                      <tr>
                        <Th>{t('roles.contentType')}</Th>
                        {verbs.map((v) => (
                          <Th key={v} className="text-center">
                            {t(`roles.action.${v}`)}
                          </Th>
                        ))}
                        <Th>{t('roles.ownOnly')}</Th>
                      </tr>
                    </thead>
                    <tbody>
                      {subjects.map((s) => (
                        <Tr key={s}>
                          <Td>
                            <div className="flex flex-col items-start gap-1">
                              <span className="font-medium">{displayName(s)}</span>
                              <Button variant="outline" title={t('roles.restrict')} icon={<SlidersHorizontal className="h-4 w-4" />} disabled={!verbs.some((v) => has(explorer + v, s))} onClick={() => setRestricting(s)} className="h-8 px-2 text-xs">
                                {t('roles.restrict')}
                              </Button>
                              {restrictionSummary(s) && <span className="text-xs text-muted">{restrictionSummary(s)}</span>}
                            </div>
                          </Td>
                          {verbs.map((v) => (
                            <Td key={v} className="text-center">
                              <input
                                type="checkbox"
                                title={`${t(`roles.action.${v}`)} · ${displayName(s)}`}
                                aria-label={`${t(`roles.action.${v}`)} · ${displayName(s)}`}
                                checked={has(explorer + v, s)}
                                onChange={(e) => set(explorer + v, s, e.target.checked, ownOnly(s) ? [isCreator] : [])}
                                className="h-4 w-4 accent-primary"
                              />
                            </Td>
                          ))}
                          <Td>
                            <input
                              type="checkbox"
                              title={t('roles.ownOnlyHelp')}
                              aria-label={`${t('roles.ownOnly')} · ${displayName(s)}`}
                              checked={ownOnly(s)}
                              onChange={(e) => setOwnOnly(s, e.target.checked)}
                              className="h-4 w-4 accent-primary"
                            />
                          </Td>
                        </Tr>
                      ))}
                    </tbody>
                  </Table>
                </div>
                <ul className="flex flex-col gap-3 md:hidden">
                  {subjects.map((s) => (
                    <li key={s} className="rounded-lg border border-border p-3">
                      <p className="mb-2 font-medium">{displayName(s)}</p>
                      {verbs.map((v) => (
                        <Checkbox key={v} label={t(`roles.action.${v}`)} checked={has(explorer + v, s)} onChange={(e) => set(explorer + v, s, e.target.checked, ownOnly(s) ? [isCreator] : [])} />
                      ))}
                      <Checkbox label={t('roles.ownOnly')} checked={ownOnly(s)} onChange={(e) => setOwnOnly(s, e.target.checked)} />
                      <Button variant="outline" title={t('roles.restrict')} icon={<SlidersHorizontal className="h-4 w-4" />} disabled={!verbs.some((v) => has(explorer + v, s))} onClick={() => setRestricting(s)} className="mt-2 w-full">
                        {t('roles.restrict')}
                      </Button>
                      {restrictionSummary(s) && <p className="mt-1 text-xs text-muted">{restrictionSummary(s)}</p>}
                    </li>
                  ))}
                </ul>
              </>
            )}
            {(tab === 'plugins' || tab === 'settings') && (
              <div className="flex flex-col gap-5">
                {grouped(catalog.data?.sections[tab] ?? []).map(([group, actions]) => (
                  <section key={group}>
                    <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted">{group}</h3>
                    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
                      {actions.map((a) => (
                        <Checkbox key={a.actionId} label={a.displayName} checked={has(a.actionId, null)} onChange={(e) => set(a.actionId, null, e.target.checked)} />
                      ))}
                    </div>
                  </section>
                ))}
              </div>
            )}
          </fieldset>
        </Card>
      )}
      {restricting && (
        <Dialog open wide title={t('roles.restrictTitle', { name: displayName(restricting) })} onClose={() => setRestricting(null)} closeLabel={t('users.close')}>
          <fieldset disabled={!editable} className="flex flex-col gap-5">
            {fieldVerbs
              .filter((v) => has(explorer + v, restricting))
              .map((v) => {
                const all = Object.keys(modelOf(restricting)?.attributes ?? {}).filter((f) => !systemFields.has(f))
                const chosen = perms.get(key(explorer + v, restricting))?.properties?.fields
                return (
                  <section key={v}>
                    <h3 className="mb-1 text-sm font-semibold">{t('roles.fieldsFor', { action: t(`roles.action.${v}`) })}</h3>
                    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
                      {all.map((f) => (
                        <Checkbox
                          key={f}
                          label={modelOf(restricting)?.editMeta?.[f]?.label || f}
                          checked={!chosen || chosen.includes(f)}
                          onChange={(e) => withProps(explorer + v, restricting, (p) => ({ ...p, fields: toggleIn(p.fields, all, f, e.target.checked) }))}
                        />
                      ))}
                    </div>
                  </section>
                )
              })}
            {modelOf(restricting)?.localized && locales.length > 0 && (
              <section>
                <h3 className="mb-1 text-sm font-semibold">{t('roles.locales')}</h3>
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
                  {locales.map((l) => {
                    const codes = locales.map((x) => x.code)
                    const current = verbs.map((v) => perms.get(key(explorer + v, restricting))?.properties?.locales).find((x) => x)
                    return (
                      <Checkbox
                        key={l.code}
                        label={l.name}
                        checked={!current || current.includes(l.code)}
                        onChange={(e) => {
                          const next = toggleIn(current, codes, l.code, e.target.checked)
                          for (const v of verbs) withProps(explorer + v, restricting, (p) => ({ ...p, locales: next }))
                        }}
                      />
                    )
                  })}
                </div>
              </section>
            )}
            <p className="text-xs text-muted">{t('roles.restrictHelp')}</p>
          </fieldset>
        </Dialog>
      )}
    </div>
  )
}
