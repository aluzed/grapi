import { lazy, Suspense, useEffect, useRef, useState } from 'react'
import { ArrowDown, ArrowUp, Plus, Sparkles, X } from 'lucide-react'
import { api } from '@/lib/api'
import { cn } from '@/lib/cn'
import { Button } from '@/components/ui/button'
import { SelectField, Switch, TextField } from '@/components/ui/field'
import { fieldLabel } from '@/lib/humanize'
import type { Attribute, ComponentModel, Model } from '@/lib/types'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { blocksItems } from './blocks/value'
import { MarkdownField } from './MarkdownField'
import { MediaField, mediaItems } from './MediaField'
import { isEditableRelation, RelationField, relationItems, type RelationItem } from './RelationField'
import type { MediaFile } from '@/lib/types'

// The blocks editor (Slate) loads with the first form that has one.
const BlocksField = lazy(() => import('./BlocksField').then((m) => ({ default: m.BlocksField })))

export type Values = Record<string, unknown>

export type FieldContext = {
  model: Model
  models: Model[]
  components: Map<string, ComponentModel>
  locale?: string
  labels: Record<string, string>
}

const scalarTypes = new Set(['string', 'text', 'richtext', 'email', 'password', 'uid', 'enumeration', 'integer', 'biginteger', 'float', 'decimal', 'boolean', 'date', 'time', 'datetime', 'timestamp', 'json'])

export function isEditable(attr: Attribute): boolean {
  if (scalarTypes.has(attr.type)) return true
  if (attr.type === 'relation') return isEditableRelation(attr)
  return attr.type === 'component' || attr.type === 'dynamiczone' || attr.type === 'media' || attr.type === 'blocks'
}

export function toInput(attr: Attribute, v: unknown, ctx: FieldContext): unknown {
  switch (attr.type) {
    case 'relation':
      return relationItems(v, attr, ctx.models)
    case 'media':
      return mediaItems(v)
    case 'blocks':
      return blocksItems(v)
    case 'component': {
      const c = ctx.components.get(attr.component ?? '')
      if (!c) return attr.repeatable ? [] : null
      if (attr.repeatable) return ((v as Values[] | null) ?? []).map((item) => toObject(c, item, ctx))
      return v ? toObject(c, v as Values, ctx) : null
    }
    case 'dynamiczone':
      return ((v as Values[] | null) ?? []).flatMap((item) => {
        const c = ctx.components.get(String(item.__component))
        return c ? [{ ...toObject(c, item, ctx), __component: c.uid }] : []
      })
  }
  if (v === null || v === undefined) return attr.type === 'boolean' ? false : ''
  if (attr.type === 'json') return JSON.stringify(v, null, 2)
  if (attr.type === 'datetime' || attr.type === 'timestamp') return toLocalDateTime(String(v))
  if (attr.type === 'time') return String(v).replace(/\.0+$/, '').replace(/^(\d\d:\d\d):00$/, '$1')
  return v
}

const pad = (n: number, size = 2) => String(n).padStart(size, '0')

// A datetime-local input holds local time without a zone: the stored UTC
// instant is shown in the browser's zone, down to the millisecond when set,
// so that saving an untouched form writes the same instant back.
export function toLocalDateTime(value: string): string {
  // A timestamp attribute reads back as milliseconds since the epoch.
  const d = new Date(/^\d+$/.test(value) ? Number(value) : value)
  if (Number.isNaN(d.getTime())) return value
  let out = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
  if (d.getSeconds() || d.getMilliseconds()) out += `:${pad(d.getSeconds())}`
  if (d.getMilliseconds()) out += `.${pad(d.getMilliseconds(), 3)}`
  return out
}

export function toObject(c: ComponentModel, src: Values | null, ctx: FieldContext): Values {
  const out: Values = {}
  if (src?.id !== undefined) out.id = src.id
  for (const name of c.fields) {
    const attr = c.attributes[name]
    if (attr && isEditable(attr)) out[name] = toInput(attr, src ? src[name] : (attr.default ?? null), ctx)
  }
  return out
}

export class JSONError extends Error {}

// fromInput converts the form value back; undefined leaves the field out.
export function fromInput(attr: Attribute, v: unknown, ctx: FieldContext): unknown {
  switch (attr.type) {
    case 'relation':
      return { set: ((v as RelationItem[] | undefined) ?? []).map((i) => ({ documentId: i.documentId })) }
    case 'blocks':
      return blocksItems(v)
    case 'media': {
      const ids = ((v as MediaFile[] | undefined) ?? []).map((f) => f.id)
      return attr.multiple ? ids : (ids[0] ?? null)
    }
    case 'component': {
      const c = ctx.components.get(attr.component ?? '')
      if (!c) return undefined
      if (attr.repeatable) return ((v as Values[] | undefined) ?? []).map((item) => fromObject(c, item, ctx))
      return v ? fromObject(c, v as Values, ctx) : null
    }
    case 'dynamiczone':
      return ((v as Values[] | undefined) ?? []).flatMap((item) => {
        const c = ctx.components.get(String(item.__component))
        return c ? [{ ...fromObject(c, item, ctx), __component: c.uid }] : []
      })
    case 'boolean':
      return Boolean(v)
  }
  if (v === '' || v === undefined) return attr.type === 'password' ? undefined : null
  switch (attr.type) {
    case 'integer':
      return Number.parseInt(String(v), 10)
    case 'float':
    case 'decimal':
      return Number(v)
    case 'json':
      try {
        return JSON.parse(String(v))
      } catch {
        throw new JSONError()
      }
    case 'datetime':
    case 'timestamp':
      return new Date(String(v)).toISOString()
    case 'time': {
      const t = String(v)
      return t.length === 5 ? `${t}:00.000` : t.includes('.') ? t : `${t}.000`
    }
  }
  return v
}

function fromObject(c: ComponentModel, item: Values, ctx: FieldContext): Values {
  const out: Values = {}
  if (item.id !== undefined) out.id = item.id
  for (const name of c.fields) {
    const attr = c.attributes[name]
    if (!attr || !isEditable(attr)) continue
    const v = fromInput(attr, item[name], ctx)
    if (v !== undefined) out[name] = v
  }
  return out
}

// fieldHint is Strapi's help under a field: its length bounds with a
// counter, or its number bounds.
function useFieldHint(attr: Attribute, value: unknown): string | undefined {
  const { t } = useI18n()
  const { minLength: lo, maxLength: hi } = attr
  if (lo || hi) {
    const n = String(value ?? '').length
    const bounds = lo && hi ? t('field.lengthBetween', { min: String(lo), max: String(hi) }) : lo ? t('field.lengthMin', { min: String(lo) }) : t('field.lengthMax', { max: String(hi) })
    return `${bounds} · ${t('field.count', { n: String(n) })}`
  }
  if (attr.min !== undefined || attr.max !== undefined) {
    if (attr.min !== undefined && attr.max !== undefined) return t('field.between', { min: String(attr.min), max: String(attr.max) })
    return attr.min !== undefined ? t('field.min', { min: String(attr.min) }) : t('field.max', { max: String(attr.max) })
  }
  return undefined
}

function ScalarField({ name, attr, text, value, error, onChange }: { name: string; attr: Attribute; text: string; value: unknown; error?: string; onChange: (v: unknown) => void }) {
  const hint = useFieldHint(attr, value)
  const common = { label: text, error, name, hint }
  switch (attr.type) {
    case 'boolean':
      return <Switch label={text} checked={Boolean(value)} onChange={(e) => onChange(e.target.checked)} />
    case 'enumeration':
      return (
        <SelectField label={text} error={error} name={name} value={String(value ?? '')} onChange={(e) => onChange(e.target.value)}>
          <option value="">-</option>
          {attr.enum?.map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </SelectField>
      )
    case 'text':
    case 'json':
      return (
        <div className="flex flex-col gap-1.5 sm:col-span-2">
          <label className="text-sm font-medium">
            {text}
            <textarea
              name={name}
              rows={attr.type === 'text' ? 3 : 10}
              value={String(value ?? '')}
              onChange={(e) => onChange(e.target.value)}
              aria-invalid={error ? true : undefined}
              className={`mt-1.5 block max-h-96 min-h-20 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm font-normal [field-sizing:content] focus:border-primary focus:ring-2 focus:ring-primary/20 focus:outline-none aria-[invalid=true]:border-destructive ${attr.type === 'json' ? 'font-mono' : ''}`}
            />
          </label>
          {hint && !error && <p className="text-xs text-muted">{hint}</p>}
          {error && <p className="text-xs text-destructive">{error}</p>}
        </div>
      )
  }
  const type = { integer: 'number', float: 'number', decimal: 'number', email: 'email', password: 'password', date: 'date', time: 'time', datetime: 'datetime-local', timestamp: 'datetime-local' }[attr.type] ?? 'text'
  return (
    <TextField
      {...common}
      type={type}
      step={attr.type === 'integer' ? 1 : attr.type === 'float' || attr.type === 'decimal' ? 'any' : attr.type === 'time' || attr.type === 'datetime' || attr.type === 'timestamp' ? 'any' : undefined}
      autoComplete={attr.type === 'password' ? 'new-password' : 'off'}
      value={String(value ?? '')}
      onChange={(e) => onChange(e.target.value)}
      placeholder={attr.type === 'password' ? '••••••••' : undefined}
    />
  )
}

const small =
  'inline-flex min-h-9 items-center gap-1 rounded-md border px-2 text-xs font-medium disabled:cursor-not-allowed disabled:bg-zinc-200 disabled:text-zinc-500'

function ItemTools({ index, count, onMove, onRemove }: { index: number; count: number; onMove?: (d: number) => void; onRemove: () => void }) {
  const { t } = useI18n()
  const confirm = useConfirm()
  return (
    <span className="flex shrink-0 gap-1">
      {onMove && (
        <>
          <button type="button" title={t('rel.up')} disabled={index === 0} onClick={() => onMove(-1)} className={`${small} border-border bg-surface`}>
            <ArrowUp className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">{t('rel.up')}</span>
          </button>
          <button type="button" title={t('rel.down')} disabled={index === count - 1} onClick={() => onMove(1)} className={`${small} border-border bg-surface`}>
            <ArrowDown className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">{t('rel.down')}</span>
          </button>
        </>
      )}
      <button
        type="button"
        title={t('comp.remove')}
        onClick={async () => {
          if (await confirm({ title: t('comp.remove'), message: t('comp.removeConfirm'), confirmLabel: t('comp.remove'), destructive: true })) onRemove()
        }}
        className={`${small} border-destructive/40 bg-surface text-destructive`}
      >
        <X className="h-3.5 w-3.5" />
        <span className="hidden sm:inline">{t('comp.remove')}</span>
      </button>
    </span>
  )
}

function move<T>(list: T[], i: number, d: number): T[] {
  const next = [...list]
  const [item] = next.splice(i, 1)
  if (item !== undefined) next.splice(i + d, 0, item)
  return next
}

function Group({ title, tools, children }: { title: string; tools: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="rounded-md border border-border bg-background/50">
      <div className="flex items-center justify-between gap-2 border-b border-border px-3 py-2">
        <span className="min-w-0 truncate text-sm font-medium">{title}</span>
        {tools}
      </div>
      <div className="p-3">{children}</div>
    </div>
  )
}

function ComponentField({ attr, text, value, onChange, ctx }: { attr: Attribute; text: string; value: unknown; onChange: (v: unknown) => void; ctx: FieldContext }) {
  const { t } = useI18n()
  const c = ctx.components.get(attr.component ?? '')
  if (!c) return null
  const empty = () => toObject(c, null, ctx)
  if (!attr.repeatable) {
    const v = value as Values | null
    return (
      <fieldset className="flex flex-col gap-2 sm:col-span-2">
        <legend className="mb-1.5 text-sm font-medium">{text}</legend>
        {v ? (
          <Group title={c.info.displayName} tools={<ItemTools index={0} count={1} onRemove={() => onChange(null)} />}>
            <FieldsGrid attrs={c.attributes} fields={c.fields} values={v} onChange={(name, x) => onChange({ ...v, [name]: x })} ctx={ctx} root={false} component={c.uid} />
          </Group>
        ) : (
          <Button variant="outline" title={`${t('comp.add')} : ${c.info.displayName}`} icon={<Plus className="h-4 w-4" />} onClick={() => onChange(empty())} className="w-full md:w-auto">
            {`${t('comp.add')} : ${c.info.displayName}`}
          </Button>
        )}
      </fieldset>
    )
  }
  const list = (value as Values[]) ?? []
  return (
    <fieldset className="flex flex-col gap-2 sm:col-span-2">
      <legend className="mb-1.5 text-sm font-medium">{text}</legend>
      {list.map((item, i) => (
        <Group
          key={String(item.id ?? `new-${i}`)}
          title={`${c.info.displayName} · ${t('comp.entry', { n: String(i + 1) })}`}
          tools={<ItemTools index={i} count={list.length} onMove={(d) => onChange(move(list, i, d))} onRemove={() => onChange(list.filter((_, j) => j !== i))} />}
        >
          <FieldsGrid attrs={c.attributes} fields={c.fields} values={item} onChange={(name, x) => onChange(list.map((it, j) => (j === i ? { ...it, [name]: x } : it)))} ctx={ctx} root={false} component={c.uid} />
        </Group>
      ))}
      <Button variant="outline" title={t('comp.addEntry')} icon={<Plus className="h-4 w-4" />} onClick={() => onChange([...list, empty()])} className="w-full md:w-auto">
        {t('comp.addEntry')}
      </Button>
    </fieldset>
  )
}

function DynamicZoneField({ attr, text, value, onChange, ctx }: { attr: Attribute; text: string; value: unknown; onChange: (v: unknown) => void; ctx: FieldContext }) {
  const { t } = useI18n()
  const allowed = (attr.components ?? []).flatMap((uid) => {
    const c = ctx.components.get(uid)
    return c ? [c] : []
  })
  const [choice, setChoice] = useState(allowed[0]?.uid ?? '')
  const list = (value as Values[]) ?? []
  return (
    <fieldset className="flex flex-col gap-2 sm:col-span-2">
      <legend className="mb-1.5 text-sm font-medium">{text}</legend>
      {list.length === 0 && <p className="text-xs text-muted">{t('comp.empty')}</p>}
      {list.map((item, i) => {
        const c = ctx.components.get(String(item.__component))
        if (!c) return null
        return (
          <Group
            key={`${c.uid}-${String(item.id ?? `new-${i}`)}`}
            title={c.info.displayName}
            tools={<ItemTools index={i} count={list.length} onMove={(d) => onChange(move(list, i, d))} onRemove={() => onChange(list.filter((_, j) => j !== i))} />}
          >
            <FieldsGrid attrs={c.attributes} fields={c.fields} values={item} onChange={(name, x) => onChange(list.map((it, j) => (j === i ? { ...it, [name]: x } : it)))} ctx={ctx} root={false} component={c.uid} />
          </Group>
        )
      })}
      {allowed.length > 0 && (
        <div className="flex flex-col gap-2 md:flex-row md:items-end">
          <SelectField label={t('comp.choose')} value={choice} onChange={(e) => setChoice(e.target.value)} className="md:w-72">
            {allowed.map((c) => (
              <option key={c.uid} value={c.uid}>
                {c.info.displayName}
              </option>
            ))}
          </SelectField>
          <Button
            variant="outline"
            title={t('comp.add')}
            icon={<Plus className="h-4 w-4" />}
            className="w-full md:w-auto"
            onClick={() => {
              const c = ctx.components.get(choice)
              if (c) onChange([...list, { ...toObject(c, null, ctx), __component: c.uid }])
            }}
          >
            {t('comp.add')}
          </Button>
        </div>
      )}
    </fieldset>
  )
}

function UIDField({ name, attr, text, value, values, error, onChange, ctx }: { name: string; attr: Attribute; text: string; value: unknown; values: Values; error?: string; onChange: (v: unknown) => void; ctx: FieldContext }) {
  const { t } = useI18n()
  const [check, setCheck] = useState<{ isAvailable: boolean; suggestion: string | null } | null>(null)
  const current = String(value ?? '')
  const body = { contentTypeUID: ctx.model.uid, field: name, locale: ctx.locale }
  // The first value is the document's own: it is taken, by itself.
  const initial = useRef<string | null>(null)
  if (initial.current === null && current) initial.current = current
  useEffect(() => {
    if (!current || current === initial.current) {
      setCheck(null)
      return
    }
    const h = setTimeout(() => {
      api
        .post<{ isAvailable: boolean; suggestion: string | null }>('/content-manager/uid/check-availability', { ...body, value: current })
        .then(setCheck)
        .catch(() => setCheck(null))
    }, 400)
    return () => clearTimeout(h)
    // body derives from the field identity, stable for this component.
  }, [current])
  const generate = async () => {
    const data: Values = {}
    if (attr.targetField) data[attr.targetField] = values[attr.targetField]
    if (initial.current === null) initial.current = ''
    onChange(await api.post<string>('/content-manager/uid/generate', { ...body, data }))
  }
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
        <TextField label={text} name={name} value={current} error={error} autoComplete="off" onChange={(e) => onChange(e.target.value)} className="flex-1" />
        <Button variant="outline" title={t('uid.generate')} icon={<Sparkles className="h-4 w-4" />} onClick={() => void generate()} className="w-full sm:w-auto">
          {t('uid.generate')}
        </Button>
      </div>
      {check && current && (
        <p className={`text-xs ${check.isAvailable ? 'text-success' : 'text-destructive'}`}>{check.isAvailable ? t('uid.available') : t('uid.taken', { s: check.suggestion ?? '' })}</p>
      )}
    </div>
  )
}

const span: Record<number, string> = {
  2: 'md:col-span-2',
  3: 'md:col-span-3',
  4: 'md:col-span-4',
  6: 'md:col-span-6',
  8: 'md:col-span-8',
  9: 'md:col-span-9',
  10: 'md:col-span-10',
  12: 'md:col-span-12',
}

// LayoutGrid renders the root fields along the configured edit layout:
// rows of 12 columns on desktop, stacked on mobile.
export function LayoutGrid(
  props: Omit<Parameters<typeof FieldsGrid>[0], 'fields'> & {
    layout: { name: string; size: number }[][]
    fields: string[]
    // writable says whether the user may change a field; the others are read only.
    writable?: (name: string) => boolean
  },
) {
  const { layout, fields, writable, ...rest } = props
  // As in Strapi, a field left out of the configured layout is not shown.
  return (
    <div className="flex flex-col gap-4">
      {layout.map((row, i) => (
        <div key={i} className="grid grid-cols-1 gap-4 md:grid-cols-12">
          {row
            .filter((f) => fields.includes(f.name))
            .map((f) => (
              <fieldset key={f.name} id={`field-${f.name}`} disabled={writable ? !writable(f.name) : undefined} className={cn('scroll-mt-24', span[f.size] ?? 'md:col-span-12')}>
                <FieldsGrid {...rest} fields={[f.name]} single />
              </fieldset>
            ))}
        </div>
      ))}
    </div>
  )
}

export function FieldsGrid({
  attrs,
  fields,
  values,
  onChange,
  ctx,
  root,
  component,
  single,
  errors = {},
}: {
  attrs: Record<string, Attribute>
  fields: string[]
  values: Values
  onChange: (name: string, v: unknown) => void
  ctx: FieldContext
  root: boolean
  // component is the uid of the component these fields belong to.
  component?: string
  // single renders one field, without the two column grid.
  single?: boolean
  errors?: Record<string, string>
}) {
  const { t } = useI18n()
  return (
    <div className={single ? 'grid grid-cols-1' : 'grid grid-cols-1 gap-4 sm:grid-cols-2'}>
      {fields.map((name) => {
        const attr = attrs[name]
        if (!attr) return null
        const meta = root ? ctx.model.editMeta?.[name] : undefined
        if (meta && !meta.visible) return null
        const text = fieldLabel(name, meta?.label) + (attr.required ? ' *' : '')
        if (!isEditable(attr)) {
          return (
            <div key={name} className="flex flex-col gap-1.5">
              <span className="text-sm font-medium">{text}</span>
              <p className="rounded-md border border-dashed border-border px-3 py-2 text-xs text-muted">{t('cm.notEditable')}</p>
            </div>
          )
        }
        const render = () => {
        switch (attr.type) {
          case 'relation':
            return (
              <RelationField
                key={name}
                uid={ctx.model.uid}
                component={component}
                name={name}
                attr={attr}
                label={text}
                value={(values[name] as RelationItem[] | undefined) ?? []}
                locale={ctx.locale}
                models={ctx.models}
                openMode={ctx.model.settings.relationOpenMode}
                onChange={(v) => onChange(name, v)}
              />
            )
          case 'uid':
            if (root) return <UIDField key={name} name={name} attr={attr} text={text} value={values[name]} values={values} error={errors[name]} onChange={(v) => onChange(name, v)} ctx={ctx} />
            break
          case 'blocks':
            return (
              <Suspense key={name} fallback={<p className="text-sm text-muted sm:col-span-2">{text}</p>}>
                <BlocksField name={name} label={text} value={blocksItems(values[name])} error={errors[name]} onChange={(v) => onChange(name, v)} />
              </Suspense>
            )
          case 'media':
            return <MediaField key={name} attr={attr} label={text} value={(values[name] as MediaFile[] | undefined) ?? []} error={errors[name]} onChange={(v) => onChange(name, v)} />
          case 'component':
            return <ComponentField key={name} attr={attr} text={text} value={values[name]} onChange={(v) => onChange(name, v)} ctx={ctx} />
          case 'richtext':
            return <MarkdownField key={name} name={name} label={text} value={String(values[name] ?? '')} error={errors[name]} maxLength={attr.maxLength} onChange={(v) => onChange(name, v)} />
          case 'dynamiczone':
            return <DynamicZoneField key={name} attr={attr} text={text} value={values[name]} onChange={(v) => onChange(name, v)} ctx={ctx} />
        }
        return <ScalarField key={name} name={name} attr={attr} text={text} value={values[name]} error={errors[name]} onChange={(v) => onChange(name, v)} />
        }
        const el = render()
        if (!meta || (meta.editable && !meta.description)) return el
        return (
          <fieldset key={name} disabled={!meta.editable} className="flex min-w-0 flex-col gap-1">
            {el}
            {meta.description && <p className="text-xs text-muted">{meta.description}</p>}
          </fieldset>
        )
      })}
    </div>
  )
}

// withoutIDs drops the component ids of a form value, so that saving a
// copy creates new components instead of moving the original ones.
export function withoutIDs(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(withoutIDs)
  if (v && typeof v === 'object' && !(v instanceof Date)) {
    const o = v as Record<string, unknown>
    if ('documentId' in o) return o
    return Object.fromEntries(Object.entries(o).filter(([k]) => k !== 'id').map(([k, x]) => [k, withoutIDs(x)]))
  }
  return v
}
