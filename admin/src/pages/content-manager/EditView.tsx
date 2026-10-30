import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, AlignLeft, Braces, ChevronDown, Copy, FileText, Image as ImageIcon, Images, Link2, RotateCcw, Save, Send, Settings2, SlidersHorizontal, Trash2, Undo2 } from 'lucide-react'
import { useBlocker, useNavigate, useParams, useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { BackButton } from '@/components/BackButton'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { SelectField } from '@/components/ui/field'
import { api, ApiError, type FieldError } from '@/lib/api'
import { cn } from '@/lib/cn'
import { fieldLabel as labelOf } from '@/lib/humanize'
import { isEmbedded, notifySaved, withEmbed } from '@/lib/embed'
import { showError } from '@/lib/form'
import type { BulkResult, ComponentModel, Entry, Model } from '@/lib/types'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { StatusBadge } from './cell'
import { useModel } from './ContentManager'
import { fromInput, isEditable, JSONError, LayoutGrid, toInput, withoutIDs, type FieldContext, type Values } from './fields'
import { fillableFields, isLocalizedAttribute } from './fill'
import { FillFromLocale } from './FillFromLocale'
import { HistoryButton } from './HistoryDialog'
import { PreviewButton, PreviewPanel, storedPreviewMode, usePreviewURL, type PreviewMode } from './PreviewPanel'
import { ReleaseButton } from './ReleaseButton'
import { splitLayout, type Section as SectionName } from './sections'
import { entryPath, useLocales, useModels } from './hooks'
import { localizeValidationError } from './validation'

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
  const { t, language } = useI18n()
  const navigate = useNavigate()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const models = useModels().data ?? []
  const [params, setParams] = useSearchParams()
  const embedded = isEmbedded(`?${params}`)
  const locales = useLocales(model.localized)
  const locale = params.get('locale') ?? (model.localized ? (locales.data?.find((l) => l.isDefault)?.code ?? '') : '')
  const loc = locale ? `?locale=${locale}` : ''
  const fields = model.fields.filter((f) => !readOnlyFields.has(f))
  const [values, setValues] = useState<Values>({})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [errorList, setErrorList] = useState<FieldError[]>([])
  // saved is the form as last loaded or saved: the view is modified when
  // the values differ from it.
  const saved = useRef<string | null>(null)
  const latest = useRef(values)
  latest.current = values
  const load = (next: Values) => {
    saved.current = JSON.stringify(next)
    setValues(next)
  }
  const isDirty = () => saved.current !== null && saved.current !== JSON.stringify(latest.current)
  // submitted is the form as sent: once saved, it is the saved state.
  const submitted = useRef<string | null>(null)
  const [preview, setPreview] = useState<PreviewMode | null>(null)
  const [saves, setSaves] = useState(0)
  const dirty = saved.current !== null && saved.current !== JSON.stringify(values)
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
  const from = !single && documentId === undefined ? params.get('from') : null
  const original = useQuery({
    queryKey: ['cm', 'entry', model.uid, from, locale],
    queryFn: () => api.get<Entry>(`${entryPath(model.uid, from!)}${loc}`),
    enabled: from !== null,
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
    if (from !== null) {
      if (!original.data) return
      // A copy: unique values cannot be duplicated, they stay empty.
      const next: Values = {}
      for (const name of fields) {
        const attr = model.attributes[name]
        if (!attr || !isEditable(attr) || attr.type === 'password') continue
        next[name] = attr.unique || attr.type === 'uid' ? toInput(attr, null, ctx) : withoutIDs(toInput(attr, original.data[name], ctx))
      }
      load(next)
      return
    }
    const src: Values = entry.data ?? {}
    const shared: Values = source.data ?? {}
    const next: Values = {}
    for (const name of fields) {
      const attr = model.attributes[name]
      if (!attr || !isEditable(attr)) continue
      if (missingLocale && !isLocalizedAttribute(attr) && name in shared) next[name] = toInput(attr, shared[name], ctx)
      else next[name] = toInput(attr, attr.type === 'password' ? null : creating ? (attr.default ?? null) : src[name], ctx)
    }
    // Changes typed while a save was on its way stay: the server copy only
    // becomes the saved state they are compared with.
    if (isDirty()) saved.current = JSON.stringify(next)
    else load(next)
    // ctx and fields derive from props that key this component.
  }, [entry.data, entry.isSuccess, creating, source.data, source.isPending, original.data])

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
    setErrorList(Object.entries(errs).map(([name, message]) => ({ path: [name], message })))
    return Object.keys(errs).length > 0 ? null : out
  }

  const refreshAll = () => queryClient.invalidateQueries({ queryKey: ['cm'] })
  const showFieldErrors = (list: FieldError[]) => {
    if (list.length === 0) return
    setErrors(Object.fromEntries(list.filter((e) => e.path.length === 1).map((e) => [e.path[0] ?? '', e.message])))
    setErrorList(list)
  }
  const fail = (err: unknown) => {
    if (err instanceof ApiError) {
      const error = localizeValidationError(err, model.attributes, t('validation.required'))
      showFieldErrors(error.fields)
      if (error.localized) {
        toast.error(error.message)
        return
      }
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
      saved.current = submitted.current
      setErrorList([])
      setSaves((n) => n + 1)
      void refreshAll()
      if (embedded) notifySaved({ uid: model.uid, documentId: e.documentId, label: String(e[model.mainField] ?? e.documentId) })
      if (documentId === undefined && !single) {
        const path = `/content-manager/collection-types/${encodeURIComponent(model.uid)}/${encodeURIComponent(e.documentId)}${loc}`
        navigate(embedded ? withEmbed(path) : path, { replace: true })
      }
    },
    onError: fail,
  })

  const action = useMutation({
    mutationFn: async (op: 'publish' | 'unpublish' | 'delete' | 'discard') => {
      if (op === 'discard') {
        await api.post(single ? `${singlePath(model.uid)}/actions/discard` : `${entryPath(model.uid, documentId)}/actions/discard`, { locale: locale || undefined })
        return { r: { count: 1, documentIds: [], errors: [] } as BulkResult, op }
      }
      if (!single) return { r: await api.post<BulkResult>(`${entryPath(model.uid)}/actions/bulk-${op}`, { documentIds: [documentId], locale: locale || undefined }), op }
      if (op === 'delete') await api.del(`${singlePath(model.uid)}${loc}`)
      else await api.post(`${singlePath(model.uid)}/actions/${op}`, { locale: locale || undefined })
      return { r: { count: 1, documentIds: [], errors: [] } as BulkResult, op }
    },
    onSuccess: ({ r, op }) => {
      if (r.errors.length > 0) {
        const errors = r.errors.map((e) => localizeValidationError(e, model.attributes, t('validation.required')))
        showFieldErrors(errors.flatMap((e) => e.fields))
        toast.error(errors.map((e) => e.message).join(', '))
        return
      }
      toast.success(t(op === 'publish' ? 'cm.published' : op === 'unpublish' ? 'cm.unpublished' : op === 'discard' ? 'cm.discarded' : 'cm.deleted'))
      setErrorList([])
      // The entry reloads: a discarded draft brings the published values back.
      if (op === 'discard') saved.current = null
      void refreshAll()
      if (op === 'delete' && !single) {
        saved.current = null
        navigate(listPath)
      }
    },
    onError: fail,
  })

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const data = payload()
    if (data) {
      submitted.current = JSON.stringify(values)
      save.mutate(data)
    }
  }

  // Leaving with unsaved changes asks first: links, back, locale switch, tab close.
  const blocker = useBlocker(({ currentLocation, nextLocation }) => isDirty() && currentLocation.pathname + currentLocation.search !== nextLocation.pathname + nextLocation.search)
  useEffect(() => {
    if (blocker.state !== 'blocked') return
    void confirm({ title: t('cm.leave'), message: t('cm.leaveConfirm'), confirmLabel: t('cm.leave'), destructive: true }).then((ok) => (ok ? blocker.proceed() : blocker.reset()))
  }, [blocker.state])
  useEffect(() => {
    if (!dirty) return
    const warn = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])

  const form = useRef<HTMLFormElement>(null)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && !e.altKey && e.key.toLowerCase() === 's') {
        e.preventDefault()
        // Fields that write on blur (the blocks editor) get to do it first.
        ;(document.activeElement as HTMLElement | null)?.blur()
        setTimeout(() => form.current?.requestSubmit(), 0)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const goToField = (name: string) => {
    const el = document.getElementById(`field-${name}`)
    el?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    el?.querySelector<HTMLElement>('input, textarea, select, [contenteditable="true"], button')?.focus({ preventScroll: true })
  }

  const originalTitle = original.data ? String(original.data[model.mainField] ?? original.data.documentId) : ''
  const title = single
    ? model.info.displayName
    : from !== null
      ? t('cm.duplicateOf', { name: originalTitle })
      : creating
        ? t('cm.newEntry')
        : // An entry without a readable main field reads as its type and number.
          (model.mainField === 'documentId' || model.mainField === 'id') && entry.data
          ? `${model.info.displayName} #${String(entry.data.id)}`
          : String(entry.data?.[model.mainField] ?? entry.data?.documentId ?? '')
  const canSave = creating ? model.permissions.create : model.permissions.update
  const writableFields = creating ? model.permissions.createFields : model.permissions.updateFields
  // null allows every field; a field is writable when it or one of its sub-fields is allowed.
  const otherLocales = (locales.data ?? []).filter((l) => l.code !== locale)
  const writable = (name: string) => writableFields == null || writableFields.some((f) => f === name || f.startsWith(name + '.'))
  const fieldLabel = (path: string[]) => path.map((p, i) => (i === 0 ? labelOf(p, model.editMeta?.[p]?.label || model.labels[p]) : /^\d+$/.test(p) ? `#${Number(p) + 1}` : p)).join(' › ')
  const person = (v: unknown) => {
    const u = v as { firstname?: string; lastname?: string; username?: string } | null | undefined
    return u ? u.username || [u.firstname, u.lastname].filter(Boolean).join(' ') : ''
  }
  const when = (v: unknown) => (v ? new Date(String(v)).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'short' }) : '-')
  const docId = String(entry.data?.documentId ?? documentId ?? '')
  const previewURL = usePreviewURL(model, single ? undefined : documentId, locale, 'draft', !creating && !embedded)
  const side = preview === 'side'
  const sections = useMemo(() => splitLayout(model.editLayout ?? [], model.attributes), [model])
  const shown = (name: string) => fields.includes(name) && model.editMeta?.[name]?.visible !== false
  const has = (s: SectionName) => sections[s].some((row) => row.some((c) => shown(c.name)))
  const grid = (s: SectionName, rows = sections[s]) => (
    <fieldset disabled={!canSave}>
      <LayoutGrid layout={rows} writable={writable} attrs={model.attributes} fields={fields} values={values} onChange={(name, v) => setValues((cur) => ({ ...cur, [name]: v }))} ctx={ctx} root errors={errors} />
    </fieldset>
  )

  return (
    <div className={side ? 'lg:grid lg:grid-cols-2 lg:items-start lg:gap-6' : undefined}>
    <form ref={form} onSubmit={submit} className="flex min-w-0 flex-col gap-6 pb-24 md:pb-0" noValidate>
      {!single && !embedded && (
        <BackButton label={t('cm.backToList')} onClick={() => navigate(listPath)} />
      )}
      {/* A direct child of the form, so that it sticks while the whole form scrolls. */}
      <div
          className={cn(
            '-mt-2 flex flex-col gap-4 md:sticky md:top-0 md:z-20 md:flex-row md:items-center md:justify-between md:border-b md:border-border md:bg-background/95 md:py-3 md:backdrop-blur',
            embedded ? 'md:-mx-4 md:px-4' : 'md:-mx-8 md:px-8',
          )}
        >
          <div className="min-w-0">
            <h1 className="truncate text-2xl font-semibold tracking-tight">{title || '-'}</h1>
            <p className="mt-1 flex flex-wrap items-center gap-2 text-sm text-muted">
              {model.info.displayName}
              {status && <StatusBadge status={status} />}
              {dirty && <span className="rounded border border-amber-200 bg-amber-50 px-1.5 py-0.5 text-xs font-medium text-amber-800">{t('cm.unsaved')}</span>}
            </p>
          </div>
          <div className="flex shrink-0 flex-col gap-2 md:flex-row md:items-center">
            {!creating && model.draftAndPublish && model.permissions.publish && (
              <Button
                variant="outline"
                title={dirty ? t('cm.saveBeforePublish') : t('cm.publish')}
                icon={<Send className="h-4 w-4" />}
                loading={action.isPending && action.variables === 'publish'}
                disabled={dirty || status === 'published'}
                onClick={() => action.mutate('publish')}
                className="w-full md:w-auto"
              >
                {t('cm.publish')}
              </Button>
            )}
            {canSave && (
              <Button type="submit" title={`${t('common.save')} (Ctrl/⌘ S)`} icon={<Save className="h-4 w-4" />} loading={save.isPending} disabled={!dirty && !creating} className="w-full md:w-auto">
                {t('common.save')}
              </Button>
            )}
          </div>
      </div>

      {errorList.length > 0 && (
        <div role="alert" className="rounded-lg border border-destructive/40 bg-destructive/5 p-4">
          <p className="mb-2 flex items-center gap-2 text-sm font-semibold text-destructive">
            <AlertCircle className="h-4 w-4" aria-hidden />
            {t('cm.errorsTitle', { count: String(errorList.length) })}
          </p>
          <ul className="flex flex-col gap-1">
            {errorList.map((e, i) => (
              <li key={i} className="text-sm">
                <button type="button" title={t('cm.goToField')} onClick={() => goToField(e.path[0] ?? '')} className="text-left text-destructive underline underline-offset-2">
                  {fieldLabel(e.path)}
                </button>
                <span className="text-muted"> : {e.message}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {missingLocale && <Card className="text-sm text-muted">{t('cm.missingLocale', { locale: locales.data?.find((l) => l.code === locale)?.name ?? locale })}</Card>}

      <div className={cn('grid grid-cols-1 gap-6', side ? '2xl:grid-cols-[minmax(0,1fr)_20rem]' : 'lg:grid-cols-[minmax(0,1fr)_20rem]')}>
        <div className="flex min-w-0 flex-col gap-6">
          {(single || documentId !== undefined) && entry.isPending ? (
            <Card className="text-sm text-muted">{t('common.loading')}</Card>
          ) : (
            <>
              {has('general') && (
                <Section title={t('cm.section.general')} icon={<FileText className="h-4 w-4" />}>
                  {grid('general')}
                </Section>
              )}
              {has('content') && (
                <Section title={t('cm.section.content')} icon={<AlignLeft className="h-4 w-4" />}>
                  {grid('content')}
                </Section>
              )}
              {sections.structure.map((row) => (
                <Card key={row[0]!.name}>{grid('structure', [row])}</Card>
              ))}
              {has('media') && (
                <Section title={t('cm.section.media')} icon={<Images className="h-4 w-4" />}>
                  {grid('media')}
                </Section>
              )}
              {has('advanced') && (
                <div className="rounded-lg border border-border bg-surface shadow-sm">
                  <details className="group">
                    <summary className="flex min-h-12 cursor-pointer list-none items-center gap-2 px-4 text-sm font-semibold tracking-wide text-muted uppercase sm:px-6">
                      <Braces className="h-4 w-4" aria-hidden />
                      {t('cm.section.advanced')}
                      <ChevronDown className="ml-auto h-4 w-4 transition-transform group-open:rotate-180" aria-hidden />
                    </summary>
                    <div className="border-t border-border p-4 sm:p-6">{grid('advanced')}</div>
                  </details>
                </div>
              )}
            </>
          )}
        </div>

        <aside className="flex flex-col gap-6">
          {(!creating || (model.localized && (locales.data?.length ?? 0) > 1)) && (
          <Section title={t('cm.section.publication')} icon={<Send className="h-4 w-4" />}>
            <div className="flex flex-col gap-3">
              {status && (
                <p className="flex items-center justify-between gap-2 text-sm">
                  <span className="text-muted">{t('cm.statusColumn')}</span>
                  <StatusBadge status={status} />
                </p>
              )}
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
              {!creating && entry.data && (
                <dl className="grid grid-cols-1 gap-2 border-t border-border pt-3 text-sm">
                  <div>
                    <dt className="text-xs text-muted">{t('cm.createdAt')}</dt>
                    <dd>
                      {when(entry.data.createdAt)}
                      {person(entry.data.createdBy) && <span className="text-muted"> · {person(entry.data.createdBy)}</span>}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs text-muted">{t('cm.updatedAt')}</dt>
                    <dd>
                      {when(entry.data.updatedAt)}
                      {person(entry.data.updatedBy) && <span className="text-muted"> · {person(entry.data.updatedBy)}</span>}
                    </dd>
                  </div>
                </dl>
              )}
              <div className="flex flex-col gap-2 border-t border-border pt-3 empty:hidden">
                {previewURL.data && !preview && (
                  <PreviewButton
                    disabled={dirty}
                    onOpen={() => setPreview(storedPreviewMode() === 'side' && window.matchMedia('(min-width: 1024px)').matches ? 'side' : 'full')}
                  />
                )}
                {model.localized && canSave && (single || documentId !== undefined) && otherLocales.length > 0 && (
                  <FillFromLocale
                    locales={otherLocales}
                    load={(code) => api.get<Entry>(single ? `${singlePath(model.uid)}?locale=${code}` : `${entryPath(model.uid, documentId)}?locale=${code}`)}
                    onFill={(src) =>
                      setValues((current) => {
                        const next = { ...current }
                        for (const name of fillableFields(model.attributes, fields, writable)) next[name] = withoutIDs(toInput(model.attributes[name]!, src[name], ctx))
                        return next
                      })
                    }
                  />
                )}
                {!creating && docId && <HistoryButton model={model} documentId={docId} locale={locale || undefined} title={title} />}
                {!creating && docId && <ReleaseButton model={model} documentIds={[docId]} locale={locale || undefined} />}
                {!creating && !single && model.permissions.create && (
                  <Button
                    variant="outline"
                    title={t('cm.duplicate')}
                    icon={<Copy className="h-4 w-4" />}
                    onClick={() => navigate(`/content-manager/collection-types/${encodeURIComponent(model.uid)}/create?from=${encodeURIComponent(documentId!)}${locale ? `&locale=${locale}` : ''}`)}
                    className="w-full"
                  >
                    {t('cm.duplicate')}
                  </Button>
                )}
                {!creating && model.draftAndPublish && model.permissions.publish && status && status !== 'draft' && (
                  <Button variant="outline" title={t('cm.unpublish')} icon={<Undo2 className="h-4 w-4" />} onClick={() => action.mutate('unpublish')} className="w-full">
                    {t('cm.unpublish')}
                  </Button>
                )}
                {!creating && model.draftAndPublish && canSave && status === 'modified' && (
                  <Button
                    variant="outline"
                    title={t('cm.discard')}
                    icon={<RotateCcw className="h-4 w-4" />}
                    onClick={async () => {
                      if (await confirm({ title: t('cm.discard'), message: t('cm.discardConfirm'), confirmLabel: t('cm.discard'), destructive: true })) action.mutate('discard')
                    }}
                    className="w-full"
                  >
                    {t('cm.discard')}
                  </Button>
                )}
                {single && model.canConfigure && (
                  <Button variant="outline" title={t('cfg.open')} icon={<Settings2 className="h-4 w-4" />} onClick={() => navigate(`/content-manager/single-types/${encodeURIComponent(model.uid)}/configurations`)} className="w-full">
                    {t('cfg.open')}
                  </Button>
                )}
                {!creating && model.permissions.delete && (
                  <Button
                    variant="destructive-outline"
                    title={t('cm.delete')}
                    icon={<Trash2 className="h-4 w-4" />}
                    className="w-full"
                    onClick={async () => {
                      if (await confirm({ title: t('cm.delete'), message: t('cm.deleteConfirm'), confirmLabel: t('cm.delete'), destructive: true })) action.mutate('delete')
                    }}
                  >
                    {t('cm.delete')}
                  </Button>
                )}
              </div>
            </div>
          </Section>
          )}
          {!entry.isPending || creating ? (
            <>
              {has('cover') && (
                <Section title={t('cm.section.media')} icon={<ImageIcon className="h-4 w-4" />}>
                  {grid('cover')}
                </Section>
              )}
              {has('relations') && (
                <Section title={t('cm.section.relations')} icon={<Link2 className="h-4 w-4" />}>
                  {grid('relations')}
                </Section>
              )}
              {has('options') && (
                <Section title={t('cm.section.options')} icon={<SlidersHorizontal className="h-4 w-4" />}>
                  {grid('options')}
                </Section>
              )}
            </>
          ) : null}
        </aside>
      </div>

      {canSave && (dirty || creating) && (
        <div className="fixed inset-x-0 bottom-0 z-30 border-t border-border bg-surface/95 p-3 backdrop-blur md:hidden">
          <Button type="submit" title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={save.isPending} className="w-full">
            {t('common.save')}
          </Button>
        </div>
      )}
    </form>
    {preview && (
      <PreviewPanel
        model={model}
        documentId={single ? undefined : documentId}
        locale={locale}
        everPublished={status === 'published' || status === 'modified'}
        saves={saves}
        mode={preview}
        onMode={setPreview}
        onClose={() => setPreview(null)}
      />
    )}
    </div>
  )
}

function Section({ title, icon, children }: { title: string; icon: ReactNode; children: ReactNode }) {
  return (
    <Card>
      <h2 className="mb-4 flex items-center gap-2 text-sm font-semibold tracking-wide text-muted uppercase">
        <span aria-hidden>{icon}</span>
        {title}
      </h2>
      {children}
    </Card>
  )
}
