import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowLeft, ArrowUp, Plus, Save, X } from 'lucide-react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardHeader } from '@/components/ui/card'
import { Checkbox, SelectField, TextField } from '@/components/ui/field'
import { api } from '@/lib/api'
import { showError } from '@/lib/form'
import type { Model } from '@/lib/types'
import { useI18n } from '@/providers/I18nProvider'
import { useModel } from './ContentManager'

type Meta = { edit: { label: string; description: string; placeholder: string; visible: boolean; editable: boolean; mainField?: string }; list: { label: string; searchable: boolean; sortable: boolean } }
type Config = {
  uid: string
  settings: { bulkable: boolean; filterable: boolean; searchable: boolean; pageSize: number; mainField: string; defaultSortBy: string; defaultSortOrder: 'ASC' | 'DESC'; relationOpenMode?: string }
  metadatas: Record<string, Meta>
  layouts: { list: string[]; edit: { name: string; size: number }[][] }
}

const fullWidth = new Set(['json', 'richtext', 'blocks', 'component', 'dynamiczone'])
const notListable = new Set(['json', 'password', 'richtext', 'blocks', 'dynamiczone', 'component'])
const narrow = new Set(['boolean', 'date', 'time', 'biginteger', 'decimal', 'float', 'integer'])
const systemFields = new Set(['createdAt', 'updatedAt', 'publishedAt', 'createdBy', 'updatedBy', 'locale', 'localizations'])

// The server's default sizes (field-sizes): full width, 4 or 6 columns.
const defaultSize = (type: string) => (fullWidth.has(type) ? 12 : narrow.has(type) ? 4 : 6)
const small =
  'inline-flex min-h-9 items-center gap-1 rounded-md border px-2 text-xs font-medium disabled:cursor-not-allowed disabled:bg-zinc-200 disabled:text-zinc-500'

// pack fills rows of 12 in order, as Strapi's appendToEditLayout does.
export function pack(fields: { name: string; size: number }[]) {
  const rows: { name: string; size: number }[][] = []
  for (const f of fields) {
    const row = rows[rows.length - 1]
    const used = row?.reduce((s, x) => s + x.size, 0) ?? 12
    if (!row || used + f.size > 12) rows.push([f])
    else row.push(f)
  }
  return rows
}

function Tools({ i, n, move, remove }: { i: number; n: number; move: (d: number) => void; remove?: () => void }) {
  const { t } = useI18n()
  return (
    <span className="flex shrink-0 gap-1">
      <button type="button" title={t('rel.up')} disabled={i === 0} onClick={() => move(-1)} className={`${small} border-border bg-surface`}>
        <ArrowUp className="h-3.5 w-3.5" />
        <span className="hidden sm:inline">{t('rel.up')}</span>
      </button>
      <button type="button" title={t('rel.down')} disabled={i === n - 1} onClick={() => move(1)} className={`${small} border-border bg-surface`}>
        <ArrowDown className="h-3.5 w-3.5" />
        <span className="hidden sm:inline">{t('rel.down')}</span>
      </button>
      {remove && (
        <button type="button" title={t('comp.remove')} onClick={remove} className={`${small} border-destructive/40 bg-surface text-destructive`}>
          <X className="h-3.5 w-3.5" />
          <span className="hidden sm:inline">{t('comp.remove')}</span>
        </button>
      )}
    </span>
  )
}

function moved<T>(list: T[], i: number, d: number) {
  const next = [...list]
  const [x] = next.splice(i, 1)
  if (x !== undefined) next.splice(i + d, 0, x)
  return next
}

export function ConfigureView() {
  const model = useModel()
  if (!model) return null
  return <Configure key={model.uid} model={model} />
}

function Configure({ model }: { model: Model }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const path = `/content-manager/configuration/${encodeURIComponent(model.uid)}`
  const cfg = useQuery({ queryKey: ['cm', 'configuration', model.uid], queryFn: () => api.get<Config>(path) })
  const [c, setC] = useState<Config | null>(null)
  const [edit, setEdit] = useState<{ name: string; size: number }[]>([])
  const [adding, setAdding] = useState('')
  const [addingField, setAddingField] = useState('')
  useEffect(() => {
    if (cfg.data) {
      setC(cfg.data)
      setEdit(cfg.data.layouts.edit.flat())
    }
  }, [cfg.data])
  const save = useMutation({
    mutationFn: () => api.put<Config>(path, { ...c, layouts: { list: c!.layouts.list, edit: pack(edit) } }),
    onSuccess: () => {
      toast.success(t('cfg.saved'))
      void queryClient.invalidateQueries({ queryKey: ['cm'] })
    },
    onError: (e) => showError(e, undefined, t('common.error')),
  })
  if (!c) return <p className="text-sm text-muted">{t('common.loading')}</p>
  const attrs = model.attributes
  const listPath = `/content-manager/${model.kind === 'singleType' ? 'single-types' : 'collection-types'}/${encodeURIComponent(model.uid)}`
  const setSettings = (patch: Partial<Config['settings']>) => setC({ ...c, settings: { ...c.settings, ...patch } })
  const setMeta = (name: string, side: 'edit' | 'list', patch: Record<string, unknown>) =>
    setC({ ...c, metadatas: { ...c.metadatas, [name]: { ...c.metadatas[name]!, [side]: { ...c.metadatas[name]![side], ...patch } } } })
  const listable = ['id', ...model.fields.filter((f) => attrs[f] && !notListable.has(attrs[f]!.type) && f !== 'documentId')]
  const sortable = ['id', ...model.fields.filter((f) => attrs[f] && !notListable.has(attrs[f]!.type) && attrs[f]!.type !== 'relation' && attrs[f]!.type !== 'media')]
  const remaining = listable.filter((f) => !c.layouts.list.includes(f))
  const unplaced = model.fields.filter((f) => attrs[f] && !systemFields.has(f) && !edit.some((x) => x.name === f))
  return (
    <div className="flex flex-col gap-6">
      <div className="space-y-4">
        <div>
          <Button variant="outline" title={t('cfg.back')} icon={<ArrowLeft className="h-4 w-4" />} onClick={() => navigate(listPath)} className="w-full md:w-auto">
            {t('cfg.back')}
          </Button>
        </div>
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">{t('cfg.title')}</h1>
            <p className="mt-1 text-sm text-muted">{model.info.displayName}</p>
          </div>
          <Button title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={save.isPending} onClick={() => save.mutate()} className="w-full md:w-auto">
            {t('common.save')}
          </Button>
        </div>
      </div>
      <Card>
        <CardHeader title={t('cfg.settings')} />
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Checkbox label={t('cfg.searchable')} checked={c.settings.searchable} onChange={(e) => setSettings({ searchable: e.target.checked })} />
          <Checkbox label={t('cfg.filterable')} checked={c.settings.filterable} onChange={(e) => setSettings({ filterable: e.target.checked })} />
          <Checkbox label={t('cfg.bulkable')} checked={c.settings.bulkable} onChange={(e) => setSettings({ bulkable: e.target.checked })} />
          <SelectField label={t('cfg.pageSize')} value={String(c.settings.pageSize)} onChange={(e) => setSettings({ pageSize: Number(e.target.value) })}>
            {[10, 20, 50, 100].map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </SelectField>
          <SelectField label={t('cfg.mainField')} value={c.settings.mainField} onChange={(e) => setSettings({ mainField: e.target.value })}>
            {sortable.map((f) => (
              <option key={f} value={f}>
                {f}
              </option>
            ))}
          </SelectField>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <SelectField label={t('cfg.sortBy')} value={c.settings.defaultSortBy} onChange={(e) => setSettings({ defaultSortBy: e.target.value })}>
              {sortable.map((f) => (
                <option key={f} value={f}>
                  {f}
                </option>
              ))}
            </SelectField>
            <SelectField label={t('cfg.sortOrder')} value={c.settings.defaultSortOrder} onChange={(e) => setSettings({ defaultSortOrder: e.target.value as 'ASC' | 'DESC' })}>
              <option value="ASC">{t('cfg.asc')}</option>
              <option value="DESC">{t('cfg.desc')}</option>
            </SelectField>
          </div>
          <SelectField label={t('cfg.relationOpenMode')} hint={t('cfg.relationOpenModeHelp')} value={c.settings.relationOpenMode || 'modal'} onChange={(e) => setSettings({ relationOpenMode: e.target.value })}>
            <option value="modal">{t('cfg.relationOpenMode.modal')}</option>
            <option value="newTab">{t('cfg.relationOpenMode.newTab')}</option>
            <option value="page">{t('cfg.relationOpenMode.page')}</option>
          </SelectField>
        </div>
      </Card>
      {model.kind !== 'singleType' && (
        <Card>
          <CardHeader title={t('cfg.list')} />
          <ul className="flex flex-col gap-2">
            {c.layouts.list.map((name, i) => (
              <li key={name} className="flex flex-col gap-2 rounded-md border border-border p-3 md:flex-row md:items-end md:justify-between">
                {name === 'id' ? (
                  <span className="text-sm font-medium">id</span>
                ) : (
                  <TextField label={`${t('cfg.label')} · ${name}`} value={c.metadatas[name]?.list.label ?? name} onChange={(e) => setMeta(name, 'list', { label: e.target.value })} className="md:w-80" />
                )}
                <Tools
                  i={i}
                  n={c.layouts.list.length}
                  move={(d) => setC({ ...c, layouts: { ...c.layouts, list: moved(c.layouts.list, i, d) } })}
                  remove={c.layouts.list.length > 1 ? () => setC({ ...c, layouts: { ...c.layouts, list: c.layouts.list.filter((x) => x !== name) } }) : undefined}
                />
              </li>
            ))}
          </ul>
          {remaining.length > 0 && (
            <div className="mt-3 flex flex-col gap-2 md:flex-row md:items-end">
              <SelectField label={t('cfg.addColumn')} value={adding} onChange={(e) => setAdding(e.target.value)} className="md:w-72">
                <option value="">-</option>
                {remaining.map((f) => (
                  <option key={f} value={f}>
                    {f}
                  </option>
                ))}
              </SelectField>
              <Button
                variant="outline"
                title={t('cfg.addColumn')}
                icon={<Plus className="h-4 w-4" />}
                disabled={!adding}
                onClick={() => {
                  setC({ ...c, layouts: { ...c.layouts, list: [...c.layouts.list, adding] } })
                  setAdding('')
                }}
                className="w-full md:w-auto"
              >
                {t('cfg.addColumn')}
              </Button>
            </div>
          )}
        </Card>
      )}
      <Card>
        <CardHeader title={t('cfg.edit')} subtitle={t('cfg.editHelp')} />
        <ul className="flex flex-col gap-3">
          {edit.map((f, i) => {
            const type = attrs[f.name]?.type ?? ''
            const m = c.metadatas[f.name]?.edit
            return (
              <li key={f.name} className="rounded-md border border-border p-3">
                <div className="mb-2 flex items-center justify-between gap-2">
                  <span className="text-sm font-medium">{f.name}</span>
                  <Tools i={i} n={edit.length} move={(d) => setEdit(moved(edit, i, d))} remove={edit.length > 1 ? () => setEdit(edit.filter((x) => x.name !== f.name)) : undefined} />
                </div>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
                  <SelectField label={t('cfg.size')} value={String(f.size)} disabled={fullWidth.has(type)} onChange={(e) => setEdit(edit.map((x, j) => (j === i ? { ...x, size: Number(e.target.value) } : x)))}>
                    {[4, 6, 8, 12].map((n) => (
                      <option key={n} value={n}>
                        {n}/12
                      </option>
                    ))}
                  </SelectField>
                  <TextField label={t('cfg.label')} value={m?.label ?? ''} onChange={(e) => setMeta(f.name, 'edit', { label: e.target.value })} />
                  <TextField label={t('cfg.description')} value={m?.description ?? ''} onChange={(e) => setMeta(f.name, 'edit', { description: e.target.value })} />
                  <TextField label={t('cfg.placeholder')} value={m?.placeholder ?? ''} onChange={(e) => setMeta(f.name, 'edit', { placeholder: e.target.value })} />
                  <Checkbox label={t('cfg.visible')} checked={m?.visible ?? true} onChange={(e) => setMeta(f.name, 'edit', { visible: e.target.checked })} />
                  <Checkbox label={t('cfg.editable')} checked={m?.editable ?? true} onChange={(e) => setMeta(f.name, 'edit', { editable: e.target.checked })} />
                </div>
              </li>
            )
          })}
        </ul>
        {unplaced.length > 0 && (
          <div className="mt-3 flex flex-col gap-2 md:flex-row md:items-end">
            <SelectField label={t('cfg.addField')} value={addingField} onChange={(e) => setAddingField(e.target.value)} className="md:w-72">
              <option value="">-</option>
              {unplaced.map((f) => (
                <option key={f} value={f}>
                  {f}
                </option>
              ))}
            </SelectField>
            <Button
              variant="outline"
              title={t('cfg.addField')}
              icon={<Plus className="h-4 w-4" />}
              disabled={!addingField}
              onClick={() => {
                setEdit([...edit, { name: addingField, size: defaultSize(attrs[addingField]?.type ?? '') }])
                setAddingField('')
              }}
              className="w-full md:w-auto"
            >
              {t('cfg.addField')}
            </Button>
          </div>
        )}
      </Card>
    </div>
  )
}
