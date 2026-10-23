import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Save, Trash2, Undo2 } from 'lucide-react'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Card, CardHeader } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { EntityActionsMenu } from '@/components/ui/entity-actions-menu'
import { Checkbox, TextField } from '@/components/ui/field'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { api } from '@/lib/api'
import { cn } from '@/lib/cn'
import { showError } from '@/lib/form'
import type { MessageKey } from '@/i18n'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { FieldDialog } from './FieldDialog'
import { fieldDeletionImpacts, kebab, pluralize, typeDeletionImpacts, type Action, type Attr, type Impact, type SchemaFile } from './model'

type Schema = { contentTypes: SchemaFile[]; components: SchemaFile[]; develop: boolean }
type NewType = { kind: 'collectionType' | 'singleType' | 'component' }

function TypeDialog({ initial, kind, onClose, onDone }: { initial: SchemaFile | null; kind: NewType['kind']; onClose: () => void; onDone: (f: SchemaFile) => void }) {
  const { t } = useI18n()
  const component = kind === 'component'
  const [displayName, setDisplayName] = useState(initial?.info.displayName ?? '')
  const [singular, setSingular] = useState(initial?.info.singularName ?? '')
  const [plural, setPlural] = useState(initial?.info.pluralName ?? '')
  const [touched, setTouched] = useState(Boolean(initial))
  const [description, setDescription] = useState(initial?.info.description ?? '')
  const [category, setCategory] = useState(initial?.category ?? '')
  const [dp, setDp] = useState(initial?.options.draftAndPublish ?? true)
  const [i18n, setI18n] = useState(Boolean(initial?.pluginOptions.i18n?.localized))
  const auto = (name: string) => {
    setDisplayName(name)
    if (!touched) {
      const k = kebab(name)
      setSingular(k)
      setPlural(k ? pluralize(k) : '')
    }
  }
  const done = () => {
    const info = component ? { displayName, description: description || undefined } : { displayName, singularName: singular, pluralName: plural, description: description || undefined }
    const uid = initial?.uid ?? (component ? `${kebab(category)}.${kebab(displayName)}` : `api::${singular}.${singular}`)
    onDone({
      uid,
      kind: component ? undefined : (initial?.kind ?? kind),
      category: component ? kebab(category) : undefined,
      info: Object.fromEntries(Object.entries(info).filter(([, v]) => v !== undefined)),
      options: component ? (initial?.options ?? {}) : { ...initial?.options, draftAndPublish: dp },
      pluginOptions: component ? {} : i18n ? { ...initial?.pluginOptions, i18n: { localized: true } } : Object.fromEntries(Object.entries(initial?.pluginOptions ?? {}).filter(([k]) => k !== 'i18n')),
      attributes: initial?.attributes ?? [],
      editable: true,
    })
  }
  const title = initial ? (initial.info.displayName ?? initial.uid) : t(component ? 'ctb.newComponent' : kind === 'singleType' ? 'ctb.newSingle' : 'ctb.newCollection')
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
          <Button title={t('ctb.done')} icon={<Pencil className="h-4 w-4" />} disabled={!displayName || (component ? !category : !singular || !plural)} onClick={done} className="w-full md:w-auto">
            {t('ctb.done')}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <TextField label={t('ctb.displayName')} value={displayName} autoFocus onChange={(e) => auto(e.target.value)} />
        {component ? (
          <TextField label={t('ctb.category')} value={category} disabled={Boolean(initial)} onChange={(e) => setCategory(e.target.value)} />
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <TextField
              label={t('ctb.singularName')}
              value={singular}
              disabled={Boolean(initial)}
              onChange={(e) => {
                setTouched(true)
                setSingular(e.target.value)
              }}
            />
            <TextField
              label={t('ctb.pluralName')}
              value={plural}
              disabled={Boolean(initial)}
              onChange={(e) => {
                setTouched(true)
                setPlural(e.target.value)
              }}
            />
          </div>
        )}
        <TextField label={t('ctb.description')} value={description} onChange={(e) => setDescription(e.target.value)} />
        {!component && (
          <>
            <Checkbox label={t('ctb.draftAndPublish')} checked={dp} onChange={(e) => setDp(e.target.checked)} />
            <Checkbox label={t('ctb.localized')} checked={i18n} onChange={(e) => setI18n(e.target.checked)} />
          </>
        )}
      </div>
    </Dialog>
  )
}

export function ContentTypeBuilder() {
  const { t } = useI18n()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const schema = useQuery({ queryKey: ['ctb', 'schema'], queryFn: () => api.get<Schema>('/content-type-builder/schema') })
  const [cts, setCts] = useState<SchemaFile[]>([])
  const [comps, setComps] = useState<SchemaFile[]>([])
  const [changes, setChanges] = useState<Map<string, Action>>(new Map())
  const [selected, setSelected] = useState<string | null>(null)
  const [typeDialog, setTypeDialog] = useState<{ initial: SchemaFile | null; kind: NewType['kind'] } | null>(null)
  const [fieldDialog, setFieldDialog] = useState<{ initial: Attr | null } | null>(null)
  const [saving, setSaving] = useState(false)

  const reset = () => {
    if (!schema.data) return
    setCts(schema.data.contentTypes)
    setComps(schema.data.components)
    setChanges(new Map())
  }
  useEffect(reset, [schema.data])

  const all = useMemo(() => [...cts, ...comps], [cts, comps])
  const current = all.find((f) => f.uid === selected) ?? null
  const isComponent = (uid: string) => comps.some((c) => c.uid === uid)
  const develop = Boolean(schema.data?.develop)

  const mark = (uid: string, action: Action) =>
    setChanges((prev) => {
      const next = new Map(prev)
      const was = prev.get(uid)
      if (action === 'delete' && was === 'create') next.delete(uid)
      else next.set(uid, was === 'create' ? 'create' : action)
      return next
    })
  const put = (f: SchemaFile) => {
    const comp = f.category !== undefined && f.kind === undefined
    const setter = comp ? setComps : setCts
    setter((list) => (list.some((x) => x.uid === f.uid) ? list.map((x) => (x.uid === f.uid ? f : x)) : [...list, f]))
  }

  const save = async () => {
    const batch = { contentTypes: [] as unknown[], components: [] as unknown[] }
    for (const [uid, action] of changes) {
      const f = all.find((x) => x.uid === uid)
      const list = isComponent(uid) || (f?.category !== undefined && f?.kind === undefined) ? batch.components : batch.contentTypes
      list.push(action === 'delete' ? { action, uid } : { action, uid, data: f })
    }
    setSaving(true)
    try {
      const before = (await api.get<{ schemaHash: string }>('/content-type-builder/status')).schemaHash
      await api.post('/content-type-builder/update-schema', batch)
      const deadline = Date.now() + 20_000
      let reloaded = false
      while (Date.now() < deadline) {
        await new Promise((r) => setTimeout(r, 600))
        try {
          if ((await api.get<{ schemaHash: string }>('/content-type-builder/status')).schemaHash !== before) {
            reloaded = true
            break
          }
        } catch {
          // the server is swapping, try again
        }
      }
      if (reloaded) toast.success(t('ctb.saved'))
      else toast.warning(t('ctb.reloadTimeout'))
      await queryClient.invalidateQueries({ queryKey: ['ctb'] })
      await queryClient.invalidateQueries({ queryKey: ['cm'] })
    } catch (err) {
      showError(err, undefined, t('common.error'))
    } finally {
      setSaving(false)
    }
  }

  const list = (title: string, files: SchemaFile[]) =>
    files.length > 0 && (
      <div>
        <h3 className="mb-1 px-3 text-xs font-semibold uppercase tracking-wide text-muted">{title}</h3>
        <ul className="flex flex-col gap-0.5">
          {files.map((f) => (
            <li key={f.uid}>
              <button
                type="button"
                title={f.info.displayName ?? f.uid}
                onClick={() => setSelected(f.uid)}
                className={cn('flex w-full items-center justify-between rounded-md px-3 py-2 text-left text-sm', selected === f.uid ? 'bg-primary/10 font-medium text-primary' : 'hover:bg-background')}
              >
                <span className="truncate">{f.info.displayName ?? f.uid}</span>
                {changes.has(f.uid) && <span className="ml-2 h-2 w-2 shrink-0 rounded-full bg-amber-500" aria-hidden />}
              </button>
            </li>
          ))}
        </ul>
      </div>
    )

  const impactLabel = (i: Impact) => t(`ctb.impact.${i.kind}`, { owner: i.owner, field: i.field })
  const editable = develop && Boolean(current?.editable)
  const fieldActions = (a: Attr) =>
    editable
      ? [
          { label: t('ctb.editField'), icon: <Pencil className="h-4 w-4" />, onSelect: () => setFieldDialog({ initial: a }) },
          {
            label: t('ctb.deleteField'),
            icon: <Trash2 className="h-4 w-4" />,
            destructive: true,
            onSelect: async () => {
              if (!current) return
              const details = { title: t('ctb.impacts'), items: fieldDeletionImpacts(current.uid, a, all).map(impactLabel) }
              if (await confirm({ title: t('ctb.deleteField'), message: t('ctb.deleteFieldConfirm', { name: a.name }), details, confirmLabel: t('ctb.deleteField'), destructive: true })) {
                put({ ...current, attributes: current.attributes.filter((x) => x !== a) })
                mark(current.uid, 'update')
              }
            },
          },
        ]
      : []

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t('ctb.title')}
        subtitle={changes.size > 0 ? t('ctb.pending', { count: String(changes.size) }) : undefined}
        actions={
          develop && (
            <>
              {changes.size > 0 && (
                <Button variant="outline" title={t('ctb.discard')} icon={<Undo2 className="h-4 w-4" />} onClick={reset} className="w-full md:w-auto">
                  {t('ctb.discard')}
                </Button>
              )}
              <Button title={t('ctb.save')} icon={<Save className="h-4 w-4" />} loading={saving} disabled={changes.size === 0} onClick={() => void save()} className="w-full md:w-auto">
                {saving ? t('ctb.saving') : t('ctb.save')}
              </Button>
            </>
          )
        }
      />
      {schema.data && !develop && <Card className="text-sm text-muted">{t('ctb.readOnly')}</Card>}
      <div className="flex flex-col gap-6 lg:flex-row">
        <aside className="flex flex-col gap-4 lg:w-64 lg:shrink-0">
          {list(t('ctb.collectionTypes'), cts.filter((f) => f.kind !== 'singleType'))}
          {list(t('ctb.singleTypes'), cts.filter((f) => f.kind === 'singleType'))}
          {list(t('ctb.components'), comps)}
          {develop && (
            <div className="flex flex-col gap-2">
              <Button variant="outline" title={t('ctb.newCollection')} icon={<Plus className="h-4 w-4" />} onClick={() => setTypeDialog({ initial: null, kind: 'collectionType' })} className="w-full">
                {t('ctb.newCollection')}
              </Button>
              <Button variant="outline" title={t('ctb.newSingle')} icon={<Plus className="h-4 w-4" />} onClick={() => setTypeDialog({ initial: null, kind: 'singleType' })} className="w-full">
                {t('ctb.newSingle')}
              </Button>
              <Button variant="outline" title={t('ctb.newComponent')} icon={<Plus className="h-4 w-4" />} onClick={() => setTypeDialog({ initial: null, kind: 'component' })} className="w-full">
                {t('ctb.newComponent')}
              </Button>
            </div>
          )}
        </aside>
        <div className="min-w-0 flex-1">
          {!current ? (
            <Card className="text-sm text-muted">{t('ctb.select')}</Card>
          ) : (
            <Card>
              <CardHeader
                title={current.info.displayName ?? current.uid}
                subtitle={current.editable ? current.uid : `${current.uid} · ${t('ctb.plugin')}`}
                actions={
                  editable && (
                    <>
                      <Button
                        variant="outline"
                        title={t('ctb.editType')}
                        icon={<Pencil className="h-4 w-4" />}
                        onClick={() => setTypeDialog({ initial: current, kind: isComponent(current.uid) ? 'component' : (current.kind ?? 'collectionType') })}
                        className="w-full md:w-auto"
                      >
                        {t('ctb.editType')}
                      </Button>
                      <Button
                        variant="destructive-outline"
                        title={t('ctb.deleteType')}
                        icon={<Trash2 className="h-4 w-4" />}
                        className="w-full md:w-auto"
                        onClick={async () => {
                          const details = { title: t('ctb.impacts'), items: typeDeletionImpacts(current.uid, all).map(impactLabel) }
                          if (await confirm({ title: t('ctb.deleteType'), message: t('ctb.deleteTypeConfirm', { name: current.info.displayName ?? current.uid }), details, confirmLabel: t('ctb.deleteType'), destructive: true })) {
                            mark(current.uid, 'delete')
                            const keep = (l: SchemaFile[]) => l.filter((x) => x.uid !== current.uid)
                            setCts(keep)
                            setComps(keep)
                            setSelected(null)
                          }
                        }}
                      >
                        {t('ctb.deleteType')}
                      </Button>
                    </>
                  )
                }
              />
              {current.editable && (
                <>
                  <div className="hidden md:block">
                    <Table>
                      <thead>
                        <tr>
                          <Th>{t('ctb.fieldName')}</Th>
                          <Th>{t('ctb.fieldType')}</Th>
                          <Th className="w-16">
                            <span className="sr-only">{t('cm.actions')}</span>
                          </Th>
                        </tr>
                      </thead>
                      <tbody>
                        {current.attributes.map((a) => (
                          <Tr key={a.name}>
                            <Td className="font-medium">
                              {a.name}
                              {a.required ? ' *' : ''}
                            </Td>
                            <Td className="text-muted">
                              {t(`ctb.t.${a.type}` as MessageKey)}
                              {a.type === 'relation' ? ` · ${String(a.target ?? '')}` : a.type === 'component' ? ` · ${String(a.component ?? '')}` : ''}
                            </Td>
                            <Td className="text-right">{fieldActions(a).length > 0 && <EntityActionsMenu actions={fieldActions(a)} label={t('cm.actions')} />}</Td>
                          </Tr>
                        ))}
                      </tbody>
                    </Table>
                  </div>
                  <ul className="flex flex-col gap-2 md:hidden">
                    {current.attributes.map((a) => (
                      <li key={a.name} className="flex items-center justify-between gap-3 rounded-md border border-border p-3">
                        <div className="min-w-0">
                          <p className="font-medium">{a.name}</p>
                          <p className="text-xs text-muted">{t(`ctb.t.${a.type}` as MessageKey)}</p>
                        </div>
                        {fieldActions(a).length > 0 && <EntityActionsMenu actions={fieldActions(a)} label={t('cm.actions')} />}
                      </li>
                    ))}
                  </ul>
                  {editable && (
                    <div className="mt-4">
                      <Button variant="outline" title={t('ctb.addField')} icon={<Plus className="h-4 w-4" />} onClick={() => setFieldDialog({ initial: null })} className="w-full md:w-auto">
                        {t('ctb.addField')}
                      </Button>
                    </div>
                  )}
                </>
              )}
            </Card>
          )}
        </div>
      </div>
      {typeDialog && (
        <TypeDialog
          initial={typeDialog.initial}
          kind={typeDialog.kind}
          onClose={() => setTypeDialog(null)}
          onDone={(f) => {
            if (!typeDialog.initial && all.some((x) => x.uid === f.uid)) {
              toast.error(f.uid)
              return
            }
            if (typeDialog.kind === 'component' && !typeDialog.initial) setComps((l) => [...l, f])
            else put(f)
            mark(f.uid, typeDialog.initial ? 'update' : 'create')
            setSelected(f.uid)
            setTypeDialog(null)
          }}
        />
      )}
      {fieldDialog && current && (
        <FieldDialog
          initial={fieldDialog.initial}
          persisted={Boolean(fieldDialog.initial && [...(schema.data?.contentTypes ?? []), ...(schema.data?.components ?? [])].find((x) => x.uid === current.uid)?.attributes.some((a) => a.name === ((fieldDialog.initial!.renamedFrom as string | undefined) ?? fieldDialog.initial!.name)))}
          owner={current}
          contentTypes={cts}
          components={comps}
          onClose={() => setFieldDialog(null)}
          onDone={(a) => {
            const old = fieldDialog.initial?.name
            let attributes = fieldDialog.initial ? current.attributes.map((x) => (x === fieldDialog.initial ? a : x)) : [...current.attributes, a]
            if (old && old !== a.name) attributes = attributes.map((x) => (x.type === 'uid' && x.targetField === old ? { ...x, targetField: a.name } : x))
            const f = { ...current, attributes }
            if (isComponent(current.uid)) setComps((l) => l.map((x) => (x.uid === f.uid ? f : x)))
            else put(f)
            mark(current.uid, 'update')
            setFieldDialog(null)
          }}
        />
      )}
    </div>
  )
}
