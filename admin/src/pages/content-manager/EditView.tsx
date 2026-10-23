import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Save, Send, Settings2, Trash2, Undo2 } from 'lucide-react'
import { useNavigate, useParams, useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { SelectField } from '@/components/ui/field'
import { api, ApiError } from '@/lib/api'
import { showError } from '@/lib/form'
import type { BulkResult, ComponentModel, Entry, Model } from '@/lib/types'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { StatusBadge } from './cell'
import { useModel } from './ContentManager'
import { fromInput, isEditable, JSONError, LayoutGrid, toInput, type FieldContext, type Values } from './fields'
import { entryPath, useLocales, useModels } from './hooks'

const readOnlyFields = new Set(['id', 'documentId', 'createdAt', 'updatedAt'])

const singlePath = (uid: string) => `/content-manager/single-types/${encodeURIComponent(uid)}`

export function useComponents() {
  return useQuery({ queryKey: ['cm', 'components'], queryFn: () => api.get<ComponentModel[]>('/content-manager/components'), staleTime: 60_000 })
}

export function EditView() {
  const model = useModel()
  const { documentId } = useParams()
  const [params] = useSearchParams()
  const components = useComponents()
  if (!model || !components.data) return null
  return <Edit key={`${model.uid}/${documentId ?? 'create'}/${params.get('locale') ?? ''}`} model={model} documentId={documentId} single={model.kind === 'singleType'} components={components.data} />
}

function Edit({ model, documentId, single, components }: { model: Model; documentId?: string; single: boolean; components: ComponentModel[] }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const models = useModels().data ?? []
  const [params, setParams] = useSearchParams()
  const locales = useLocales(model.localized)
  const locale = params.get('locale') ?? (model.localized ? (locales.data?.find((l) => l.isDefault)?.code ?? '') : '')
  const loc = locale ? `?locale=${locale}` : ''
  const fields = model.fields.filter((f) => !readOnlyFields.has(f))
  const [values, setValues] = useState<Values>({})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const listPath = `/content-manager/collection-types/${encodeURIComponent(model.uid)}${loc}`
  const ctx: FieldContext = useMemo(
    () => ({ model, models, components: new Map(components.map((c) => [c.uid, c])), locale: locale || undefined, labels: model.labels }),
    [model, models, components, locale],
  )

  const entry = useQuery({
    queryKey: ['cm', 'entry', model.uid, documentId, locale],
    queryFn: () =>
      api.get<Entry | null>(single ? `${singlePath(model.uid)}${loc}` : `${entryPath(model.uid, documentId)}${loc}`).catch((err) => {
        // Not written in this locale yet: an empty form creates it.
        if (err instanceof ApiError && err.status === 404 && model.localized && documentId !== undefined) return null
        throw err
      }),
    enabled: single || documentId !== undefined,
    retry: false,
  })
  const missingLocale = !single && documentId !== undefined && entry.isSuccess && entry.data === null
  // A new locale version shares the non localized fields of the others:
  // they come from the default locale, never from the defaults.
  const defaultLocale = locales.data?.find((l) => l.isDefault)?.code
  const source = useQuery({
    queryKey: ['cm', 'entry', model.uid, documentId, defaultLocale],
    queryFn: () => api.get<Entry>(`${entryPath(model.uid, documentId)}?locale=${defaultLocale}`),
    enabled: missingLocale && Boolean(defaultLocale) && defaultLocale !== locale,
  })
  const creating = single ? entry.isSuccess && entry.data === null : documentId === undefined || missingLocale
  const listStatus = useQuery({
    queryKey: ['cm', 'entry-status', model.uid, documentId, locale],
    queryFn: () => api.get<Entry[]>(`${entryPath(model.uid)}?pageSize=1&filters=${encodeURIComponent(JSON.stringify({ documentId: { $eq: documentId } }))}${locale ? `&locale=${locale}` : ''}`),
    enabled: !single && documentId !== undefined && model.draftAndPublish,
  })
  const status = single ? entry.data?.status : listStatus.data?.[0]?.status

  useEffect(() => {
    if (single ? !entry.isSuccess : !creating && !entry.data) return
    if (missingLocale && source.isPending && source.fetchStatus !== 'idle') return
    const src: Values = entry.data ?? {}
    const shared: Values = source.data ?? {}
    const next: Values = {}
    for (const name of fields) {
      const attr = model.attributes[name]
      if (!attr || !isEditable(attr)) continue
      if (missingLocale && attr.pluginOptions?.i18n?.localized === false && name in shared) next[name] = toInput(attr, shared[name], ctx)
      else next[name] = toInput(attr, attr.type === 'password' ? null : creating ? (attr.default ?? null) : src[name], ctx)
    }
    setValues(next)
    // ctx and fields derive from props that key this component.
  }, [entry.data, entry.isSuccess, creating, source.data, source.isPending])

  const payload = (): Values | null => {
    const out: Values = {}
    const errs: Record<string, string> = {}
    for (const name of fields) {
      const attr = model.attributes[name]
      if (!attr || !isEditable(attr)) continue
      try {
        const v = fromInput(attr, values[name], ctx)
        if (v !== undefined) out[name] = v
      } catch (e) {
        if (!(e instanceof JSONError)) throw e
        errs[name] = t('cm.invalidJson')
      }
    }
    setErrors(errs)
    return Object.keys(errs).length > 0 ? null : out
  }

  const refreshAll = () => queryClient.invalidateQueries({ queryKey: ['cm'] })
  const fail = (err: unknown) => {
    if (err instanceof ApiError) {
      const list = err.fieldErrors()
      if (list.length > 0) setErrors(Object.fromEntries(list.filter((e) => e.path.length === 1).map((e) => [e.path[0] ?? '', e.message])))
    }
    showError(err, undefined, t('common.error'))
  }

  const save = useMutation({
    mutationFn: (data: Values) =>
      single
        ? api.put<Entry>(singlePath(model.uid), { data, locale: locale || undefined })
        : documentId === undefined
          ? api.post<Entry>(entryPath(model.uid), { data, locale: locale || undefined })
          : api.put<Entry>(entryPath(model.uid, documentId), { data, locale: locale || undefined }),
    onSuccess: (e) => {
      toast.success(creating ? t('cm.created') : t('cm.saved'))
      void refreshAll()
      if (documentId === undefined && !single) navigate(`/content-manager/collection-types/${encodeURIComponent(model.uid)}/${encodeURIComponent(e.documentId)}${loc}`, { replace: true })
    },
    onError: fail,
  })

  const action = useMutation({
    mutationFn: async (op: 'publish' | 'unpublish' | 'delete') => {
      if (!single) return { r: await api.post<BulkResult>(`${entryPath(model.uid)}/actions/bulk-${op}`, { documentIds: [documentId], locale: locale || undefined }), op }
      if (op === 'delete') await api.del(`${singlePath(model.uid)}${loc}`)
      else await api.post(`${singlePath(model.uid)}/actions/${op}`, { locale: locale || undefined })
      return { r: { count: 1, documentIds: [], errors: [] } as BulkResult, op }
    },
    onSuccess: ({ r, op }) => {
      if (r.errors.length > 0) {
        toast.error(r.errors.map((e) => e.message).join(', '))
        return
      }
      toast.success(t(op === 'publish' ? 'cm.published' : op === 'unpublish' ? 'cm.unpublished' : 'cm.deleted'))
      void refreshAll()
      if (op === 'delete' && !single) navigate(listPath)
    },
    onError: fail,
  })

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const data = payload()
    if (data) save.mutate(data)
  }

  const title = single ? model.info.displayName : creating ? t('cm.newEntry') : String(entry.data?.[model.mainField] ?? entry.data?.documentId ?? '')
  const canSave = creating ? model.permissions.create : model.permissions.update

  return (
    <form onSubmit={submit} className="flex flex-col gap-6" noValidate>
      <div className="space-y-4">
        {!single && (
          <div>
            <Button variant="outline" title={t('cm.backToList')} icon={<ArrowLeft className="h-4 w-4" />} onClick={() => navigate(listPath)} className="w-full md:w-auto">
              {t('cm.backToList')}
            </Button>
          </div>
        )}
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div className="min-w-0">
            <h1 className="truncate text-2xl font-semibold tracking-tight">{title || '-'}</h1>
            <p className="mt-1 flex items-center gap-2 text-sm text-muted">
              {model.info.displayName}
              {status && <StatusBadge status={status} />}
            </p>
          </div>
          <div className="flex flex-col gap-2 md:flex-row md:items-end">
            {model.localized && (locales.data?.length ?? 0) > 1 && (
              <SelectField
                label={t('cm.locale')}
                value={locale}
                onChange={(e) => {
                  const next = new URLSearchParams(params)
                  next.set('locale', e.target.value)
                  setParams(next, { replace: true })
                }}
              >
                {locales.data?.map((l) => (
                  <option key={l.code} value={l.code}>
                    {l.name}
                  </option>
                ))}
              </SelectField>
            )}
            {single && model.canConfigure && (
              <Button variant="outline" title={t('cfg.open')} icon={<Settings2 className="h-4 w-4" />} onClick={() => navigate(`/content-manager/single-types/${encodeURIComponent(model.uid)}/configurations`)} className="w-full md:w-auto">
                {t('cfg.open')}
              </Button>
            )}
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

      {missingLocale && <Card className="text-sm text-muted">{t('cm.missingLocale', { locale: locales.data?.find((l) => l.code === locale)?.name ?? locale })}</Card>}
      {(single || documentId !== undefined) && entry.isPending ? (
        <p className="text-sm text-muted">{t('common.loading')}</p>
      ) : (
        <Card>
          <fieldset disabled={!canSave}>
            <LayoutGrid layout={model.editLayout ?? []} attrs={model.attributes} fields={fields} values={values} onChange={(name, v) => setValues((s) => ({ ...s, [name]: v }))} ctx={ctx} root errors={errors} />
          </fieldset>
        </Card>
      )}
    </form>
  )
}
