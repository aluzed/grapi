import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Save, Send, Trash2, Undo2 } from 'lucide-react'
import { useNavigate, useParams, useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Checkbox, SelectField, TextField } from '@/components/ui/field'
import { api, ApiError } from '@/lib/api'
import { showError } from '@/lib/form'
import type { Attribute, BulkResult, Entry, Model } from '@/lib/types'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { StatusBadge } from './cell'
import { useModel } from './ContentManager'
import { entryPath, label, useModels } from './hooks'
import { isEditableRelation, RelationField, relationItems, type RelationItem } from './RelationField'

const editableTypes = new Set(['string', 'text', 'richtext', 'email', 'password', 'uid', 'enumeration', 'integer', 'biginteger', 'float', 'decimal', 'boolean', 'date', 'time', 'datetime', 'json'])

type Values = Record<string, unknown>

const editable = (attr: Attribute) => editableTypes.has(attr.type) || isEditableRelation(attr)

const readOnlyFields = new Set(['id', 'documentId', 'createdAt', 'updatedAt'])

function toInput(attr: Attribute, v: unknown, models: Model[]): unknown {
  if (attr.type === 'relation') return relationItems(v, attr, models)
  if (v === null || v === undefined) return attr.type === 'boolean' ? false : ''
  if (attr.type === 'json') return JSON.stringify(v, null, 2)
  if (attr.type === 'datetime') return String(v).slice(0, 16)
  if (attr.type === 'time') return String(v).slice(0, 5)
  return v
}

// fromInput converts the form value back; undefined leaves the field out.
function fromInput(attr: Attribute, v: unknown): unknown {
  if (attr.type === 'relation') return { set: ((v as RelationItem[] | undefined) ?? []).map((i) => ({ documentId: i.documentId })) }
  if (attr.type === 'boolean') return Boolean(v)
  if (v === '' || v === undefined) return attr.type === 'password' ? undefined : null
  switch (attr.type) {
    case 'integer':
      return Number.parseInt(String(v), 10)
    case 'float':
    case 'decimal':
      return Number(v)
    case 'json':
      return JSON.parse(String(v))
    case 'datetime':
      return new Date(String(v)).toISOString()
    case 'time':
      return `${String(v)}:00.000`
  }
  return v
}

function Field({ name, attr, model, value, error, onChange }: { name: string; attr: Attribute; model: Model; value: unknown; error?: string; onChange: (v: unknown) => void }) {
  const text = label(model, name) + (attr.required ? ' *' : '')
  const common = { label: text, error, name }
  switch (attr.type) {
    case 'boolean':
      return <Checkbox label={text} checked={Boolean(value)} onChange={(e) => onChange(e.target.checked)} />
    case 'enumeration':
      return (
        <SelectField {...common} value={String(value ?? '')} onChange={(e) => onChange(e.target.value)}>
          <option value="">-</option>
          {attr.enum?.map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </SelectField>
      )
    case 'text':
    case 'richtext':
    case 'json':
      return (
        <div className="flex flex-col gap-1.5 sm:col-span-2">
          <label htmlFor={`f-${name}`} className="text-sm font-medium">
            {text}
          </label>
          <textarea
            id={`f-${name}`}
            rows={attr.type === 'text' ? 4 : 10}
            value={String(value ?? '')}
            onChange={(e) => onChange(e.target.value)}
            aria-invalid={error ? true : undefined}
            className={`block w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-primary focus:outline-none aria-[invalid=true]:border-destructive ${attr.type === 'json' ? 'font-mono' : ''}`}
          />
          {error && <p className="text-xs text-destructive">{error}</p>}
        </div>
      )
  }
  const type =
    { integer: 'number', float: 'number', decimal: 'number', email: 'email', password: 'password', date: 'date', time: 'time', datetime: 'datetime-local' }[attr.type] ?? 'text'
  return (
    <TextField
      {...common}
      type={type}
      step={attr.type === 'integer' ? 1 : attr.type === 'float' || attr.type === 'decimal' ? 'any' : undefined}
      autoComplete={attr.type === 'password' ? 'new-password' : 'off'}
      value={String(value ?? '')}
      onChange={(e) => onChange(e.target.value)}
      placeholder={attr.type === 'password' ? '••••••••' : undefined}
    />
  )
}

export function EditView() {
  const model = useModel()
  const { documentId } = useParams()
  if (!model) return null
  return <Edit key={`${model.uid}/${documentId ?? 'create'}`} model={model} documentId={documentId} />
}

function Edit({ model, documentId }: { model: Model; documentId?: string }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const models = useModels().data ?? []
  const [params] = useSearchParams()
  const locale = params.get('locale') ?? ''
  const creating = documentId === undefined
  const fields = model.fields.filter((f) => !readOnlyFields.has(f)).map((f) => [f, model.attributes[f]!] as const)
  const [values, setValues] = useState<Values>({})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const listPath = `/content-manager/collection-types/${encodeURIComponent(model.uid)}${locale ? `?locale=${locale}` : ''}`

  const entry = useQuery({
    queryKey: ['cm', 'entry', model.uid, documentId, locale],
    queryFn: () => api.get<Entry>(`${entryPath(model.uid, documentId)}${locale ? `?locale=${locale}` : ''}`),
    enabled: !creating,
  })
  const listStatus = useQuery({
    queryKey: ['cm', 'entry-status', model.uid, documentId, locale],
    queryFn: () => api.get<Entry[]>(`${entryPath(model.uid)}?pageSize=1&filters=${encodeURIComponent(JSON.stringify({ documentId: { $eq: documentId } }))}${locale ? `&locale=${locale}` : ''}`),
    enabled: !creating && model.draftAndPublish,
  })
  const status = listStatus.data?.[0]?.status

  useEffect(() => {
    const src: Values = creating ? {} : (entry.data ?? {})
    if (!creating && !entry.data) return
    const next: Values = {}
    for (const [name, attr] of fields) {
      if (!editable(attr)) continue
      next[name] = toInput(attr, attr.type === 'password' ? null : creating ? (attr.default ?? null) : src[name], models)
    }
    setValues(next)
    // fields derive from model, which keys this component.
  }, [entry.data, creating])

  const payload = (): Values | null => {
    const out: Values = {}
    const errs: Record<string, string> = {}
    for (const [name, attr] of fields) {
      if (!editable(attr)) continue
      try {
        const v = fromInput(attr, values[name])
        if (v !== undefined) out[name] = v
      } catch {
        errs[name] = t('cm.invalidJson')
      }
    }
    setErrors(errs)
    return Object.keys(errs).length > 0 ? null : out
  }

  const refreshAll = () => queryClient.invalidateQueries({ queryKey: ['cm'] })
  const fail = (err: unknown) => {
    if (err instanceof ApiError) {
      const path = (err.details as { errors?: { path: string[]; message: string }[] }).errors
      if (path && path.length > 0) {
        setErrors(Object.fromEntries(path.map((e) => [e.path[e.path.length - 1] ?? '', e.message])))
      }
    }
    showError(err, undefined, t('common.error'))
  }

  const save = useMutation({
    mutationFn: (data: Values) =>
      creating ? api.post<Entry>(entryPath(model.uid), { data, locale: locale || undefined }) : api.put<Entry>(entryPath(model.uid, documentId), { data, locale: locale || undefined }),
    onSuccess: (e) => {
      toast.success(creating ? t('cm.created') : t('cm.saved'))
      void refreshAll()
      if (creating) navigate(`/content-manager/collection-types/${encodeURIComponent(model.uid)}/${encodeURIComponent(e.documentId)}${locale ? `?locale=${locale}` : ''}`, { replace: true })
    },
    onError: fail,
  })

  const action = useMutation({
    mutationFn: (op: 'publish' | 'unpublish' | 'delete') => api.post<BulkResult>(`${entryPath(model.uid)}/actions/bulk-${op}`, { documentIds: [documentId], locale: locale || undefined }).then((r) => ({ r, op })),
    onSuccess: ({ r, op }) => {
      if (r.errors.length > 0) {
        toast.error(r.errors.map((e) => e.message).join(', '))
        return
      }
      toast.success(t(op === 'publish' ? 'cm.published' : op === 'unpublish' ? 'cm.unpublished' : 'cm.deleted'))
      void refreshAll()
      if (op === 'delete') navigate(listPath)
    },
    onError: fail,
  })

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const data = payload()
    if (data) save.mutate(data)
  }

  const title = creating ? t('cm.newEntry') : String(entry.data?.[model.mainField] ?? entry.data?.documentId ?? '')
  const canSave = creating ? model.permissions.create : model.permissions.update

  return (
    <form onSubmit={submit} className="flex flex-col gap-6" noValidate>
      <div className="space-y-4">
        <div>
          <Button variant="outline" title={t('cm.backToList')} icon={<ArrowLeft className="h-4 w-4" />} onClick={() => navigate(listPath)} className="w-full md:w-auto">
            {t('cm.backToList')}
          </Button>
        </div>
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div className="min-w-0">
            <h1 className="truncate text-2xl font-semibold tracking-tight">{title || '-'}</h1>
            <p className="mt-1 flex items-center gap-2 text-sm text-muted">
              {model.info.displayName}
              {status && <StatusBadge status={status} />}
            </p>
          </div>
          <div className="flex flex-col gap-2 md:flex-row">
            {canSave && (
              <Button type="submit" title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={save.isPending} className="w-full md:w-auto">
                {t('common.save')}
              </Button>
            )}
            {!creating && model.draftAndPublish && model.permissions.publish && (
              <Button variant="outline" title={t('cm.publish')} icon={<Send className="h-4 w-4" />} loading={action.isPending && action.variables === 'publish'} onClick={() => action.mutate('publish')} className="w-full md:w-auto">
                {t('cm.publish')}
              </Button>
            )}
            {!creating && model.draftAndPublish && model.permissions.publish && status && status !== 'draft' && (
              <Button variant="outline" title={t('cm.unpublish')} icon={<Undo2 className="h-4 w-4" />} onClick={() => action.mutate('unpublish')} className="w-full md:w-auto">
                {t('cm.unpublish')}
              </Button>
            )}
            {!creating && model.permissions.delete && (
              <Button
                variant="destructive-outline"
                title={t('cm.delete')}
                icon={<Trash2 className="h-4 w-4" />}
                className="w-full md:w-auto"
                onClick={async () => {
                  if (await confirm({ title: t('cm.delete'), message: t('cm.deleteConfirm'), confirmLabel: t('cm.delete'), destructive: true })) action.mutate('delete')
                }}
              >
                {t('cm.delete')}
              </Button>
            )}
          </div>
        </div>
      </div>

      {!creating && entry.isPending ? (
        <p className="text-sm text-muted">{t('common.loading')}</p>
      ) : (
        <Card>
          <fieldset disabled={!canSave} className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {fields.map(([name, attr]) =>
              attr.type === 'relation' && editable(attr) ? (
                <RelationField
                  key={name}
                  uid={model.uid}
                  name={name}
                  attr={attr}
                  label={label(model, name) + (attr.required ? ' *' : '')}
                  value={(values[name] as RelationItem[] | undefined) ?? []}
                  locale={locale || undefined}
                  onChange={(v) => setValues((s) => ({ ...s, [name]: v }))}
                />
              ) : editable(attr) ? (
                <Field key={name} name={name} attr={attr} model={model} value={values[name]} error={errors[name]} onChange={(v) => setValues((s) => ({ ...s, [name]: v }))} />
              ) : (
                <div key={name} className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium">{label(model, name)}</span>
                  <p className="rounded-md border border-dashed border-border px-3 py-2 text-xs text-muted">{t('cm.notEditable')}</p>
                </div>
              ),
            )}
          </fieldset>
        </Card>
      )}
    </form>
  )
}
