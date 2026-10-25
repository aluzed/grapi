import { useState } from 'react'
import { ArrowLeft, Check } from 'lucide-react'
import { DialogActions } from '@/components/DialogActions'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Checkbox, SelectField, TextField } from '@/components/ui/field'
import type { MessageKey } from '@/i18n'
import { useI18n } from '@/providers/I18nProvider'
import { bidirectional, fieldTypes, relationKinds, type Attr, type SchemaFile } from './model'

const num = (v: string) => (v.trim() === '' ? undefined : Number(v))

// FieldDialog edits one attribute; onDone gets the attribute to store.
// Scalar fields can be renamed: a migration renames their column.
const renameable = new Set(['string', 'text', 'richtext', 'blocks', 'email', 'password', 'uid', 'enumeration', 'integer', 'biginteger', 'float', 'decimal', 'date', 'datetime', 'time', 'boolean', 'json'])

export function FieldDialog({
  initial,
  persisted,
  owner,
  contentTypes,
  components,
  onClose,
  onDone,
}: {
  initial: Attr | null
  // persisted is true when the field exists in the saved schema.
  persisted: boolean
  owner: SchemaFile
  contentTypes: SchemaFile[]
  components: SchemaFile[]
  onClose: () => void
  onDone: (a: Attr) => void
}) {
  const { t } = useI18n()
  const [attr, setAttr] = useState<Attr | null>(initial)
  const [error, setError] = useState<string>()
  const set = (patch: Record<string, unknown>) => setAttr((a) => (a ? ({ ...a, ...patch } as Attr) : a))
  const clean = (a: Attr): Attr => Object.fromEntries(Object.entries(a).filter(([, v]) => v !== undefined && v !== '' && !(Array.isArray(v) && v.length === 0))) as Attr
  const done = () => {
    if (!attr) return
    if (!/^[a-zA-Z][a-zA-Z0-9_]*$/.test(attr.name)) {
      setError(t('ctb.fieldName'))
      return
    }
    if (owner.attributes.some((a) => a.name === attr.name && a !== initial)) {
      setError(`${attr.name} ×2`)
      return
    }
    if (initial && persisted && attr.name !== initial.name) {
      onDone(clean({ ...attr, renamedFrom: (initial.renamedFrom as string | undefined) ?? initial.name }))
      return
    }
    onDone(clean(attr))
  }
  const tlabel = (k: string) => t(`ctb.t.${k}` as MessageKey)

  if (!attr) {
    return (
      <Dialog open wide title={t('ctb.chooseType')} onClose={onClose} closeLabel={t('common.cancel')}>
        <ul className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {fieldTypes
            .filter((ft) => !(owner.category && ft === 'dynamiczone'))
            .map((ft) => (
              <li key={ft}>
                <button
                  type="button"
                  title={tlabel(ft)}
                  onClick={() => setAttr({ name: '', type: ft, ...(ft === 'relation' ? { relation: 'manyToOne', target: contentTypes.find((c) => c.editable)?.uid } : {}) })}
                  className="w-full rounded-md border border-border bg-surface px-3 py-3 text-left text-sm font-medium hover:border-primary/40"
                >
                  {tlabel(ft)}
                </button>
              </li>
            ))}
        </ul>
      </Dialog>
    )
  }

  const type = attr.type
  const v = (k: string) => (attr[k] === undefined || attr[k] === null ? '' : String(attr[k]))
  const scalarText = ['string', 'text', 'email', 'password', 'uid', 'richtext'].includes(type)
  const numeric = ['integer', 'biginteger', 'float', 'decimal'].includes(type)

  return (
    <Dialog
      open
      wide
      title={`${tlabel(type)}${initial ? ` · ${initial.name}` : ''}`}
      onClose={onClose}
      closeLabel={t('common.cancel')}
      footer={
        <>
          {!initial && (
            <Button variant="outline" title={t('ctb.back')} icon={<ArrowLeft className="h-4 w-4" />} onClick={() => setAttr(null)} className="w-full md:mr-auto md:w-auto">
              {t('ctb.back')}
            </Button>
          )}
          <DialogActions onCancel={onClose} onConfirm={done} label={t('ctb.done')} icon={<Check className="h-4 w-4" />} />
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <TextField
          label={t('ctb.fieldName')}
          value={attr.name}
          error={error}
          autoFocus
          onChange={(e) => set({ name: e.target.value })}
          disabled={Boolean(initial) && !renameable.has(type)}
          hint={initial && renameable.has(type) && persisted ? t('ctb.renameHint') : undefined}
        />
        {type === 'enumeration' && (
          <label className="flex flex-col gap-1.5 text-sm font-medium">
            {t('ctb.enum')}
            <textarea
              rows={5}
              value={((attr.enum as string[] | undefined) ?? []).join('\n')}
              onChange={(e) => set({ enum: e.target.value.split('\n').map((x) => x.trim()).filter(Boolean) })}
              className="block w-full rounded-md border border-border bg-surface px-3 py-2 text-sm font-normal focus:border-primary focus:outline-none"
            />
          </label>
        )}
        {type === 'uid' && (
          <SelectField label={t('ctb.targetField')} value={v('targetField')} onChange={(e) => set({ targetField: e.target.value || undefined })}>
            <option value="">-</option>
            {owner.attributes
              .filter((a) => a.type === 'string' || a.type === 'text')
              .map((a) => (
                <option key={a.name} value={a.name}>
                  {a.name}
                </option>
              ))}
          </SelectField>
        )}
        {type === 'media' && (
          <div className="flex flex-col gap-1">
            <Checkbox label={t('ctb.multiple')} checked={Boolean(attr.multiple)} onChange={(e) => set({ multiple: e.target.checked })} />
            <span className="text-sm font-medium">{t('ctb.allowedTypes')}</span>
            <div className="grid grid-cols-2 sm:grid-cols-4">
              {['images', 'videos', 'audios', 'files'].map((k) => {
                const list = (attr.allowedTypes as string[] | undefined) ?? []
                return <Checkbox key={k} label={k} checked={list.includes(k)} onChange={(e) => set({ allowedTypes: e.target.checked ? [...list, k] : list.filter((x) => x !== k) })} />
              })}
            </div>
          </div>
        )}
        {type === 'relation' && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <SelectField label={t('ctb.relation')} value={v('relation')} onChange={(e) => set({ relation: e.target.value, targetAttribute: bidirectional.has(e.target.value) ? attr.targetAttribute : undefined })} disabled={Boolean(initial)}>
              {relationKinds.map((k) => (
                <option key={k} value={k}>
                  {t(`ctb.r.${k}` as MessageKey)}
                </option>
              ))}
            </SelectField>
            <SelectField label={t('ctb.target')} value={v('target')} onChange={(e) => set({ target: e.target.value })} disabled={Boolean(initial)}>
              {contentTypes.map((c) => (
                <option key={c.uid} value={c.uid}>
                  {c.info.displayName ?? c.uid}
                </option>
              ))}
            </SelectField>
            {bidirectional.has(v('relation')) && !initial && (
              <TextField label={t('ctb.targetAttribute')} value={v('targetAttribute')} onChange={(e) => set({ targetAttribute: e.target.value })} className="sm:col-span-2" />
            )}
          </div>
        )}
        {type === 'component' && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <SelectField label={t('ctb.component')} value={v('component')} onChange={(e) => set({ component: e.target.value })}>
              <option value="">-</option>
              {components
                .filter((c) => c.uid !== owner.uid)
                .map((c) => (
                  <option key={c.uid} value={c.uid}>
                    {c.uid}
                  </option>
                ))}
            </SelectField>
            <Checkbox label={t('ctb.repeatable')} checked={Boolean(attr.repeatable)} onChange={(e) => set({ repeatable: e.target.checked })} />
          </div>
        )}
        {type === 'dynamiczone' && (
          <fieldset>
            <legend className="mb-1 text-sm font-medium">{t('ctb.dzComponents')}</legend>
            <div className="grid grid-cols-1 sm:grid-cols-2">
              {components.map((c) => {
                const list = (attr.components as string[] | undefined) ?? []
                return <Checkbox key={c.uid} label={c.uid} checked={list.includes(c.uid)} onChange={(e) => set({ components: e.target.checked ? [...list, c.uid] : list.filter((x) => x !== c.uid) })} />
              })}
            </div>
          </fieldset>
        )}
        {!['relation', 'component', 'dynamiczone'].includes(type) && (
          <div className="grid grid-cols-1 gap-x-4 sm:grid-cols-3">
            <Checkbox label={t('ctb.required')} checked={Boolean(attr.required)} onChange={(e) => set({ required: e.target.checked || undefined })} />
            {!['media', 'json', 'boolean', 'blocks', 'richtext', 'password'].includes(type) && (
              <Checkbox label={t('ctb.unique')} checked={Boolean(attr.unique)} onChange={(e) => set({ unique: e.target.checked || undefined })} />
            )}
            <Checkbox label={t('ctb.private')} checked={Boolean(attr.private)} onChange={(e) => set({ private: e.target.checked || undefined })} />
          </div>
        )}
        {(scalarText || numeric || type === 'enumeration' || type === 'boolean') && type !== 'password' && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {type === 'boolean' ? (
              <SelectField label={t('ctb.default')} value={attr.default === undefined ? '' : String(attr.default)} onChange={(e) => set({ default: e.target.value === '' ? undefined : e.target.value === 'true' })}>
                <option value="">-</option>
                <option value="true">true</option>
                <option value="false">false</option>
              </SelectField>
            ) : type === 'enumeration' ? (
              <SelectField label={t('ctb.default')} value={v('default')} onChange={(e) => set({ default: e.target.value || undefined })}>
                <option value="">-</option>
                {((attr.enum as string[] | undefined) ?? []).map((x) => (
                  <option key={x} value={x}>
                    {x}
                  </option>
                ))}
              </SelectField>
            ) : (
              <TextField label={t('ctb.default')} value={v('default')} onChange={(e) => set({ default: numeric && type !== 'biginteger' ? num(e.target.value) : e.target.value || undefined })} />
            )}
            {scalarText && type !== 'uid' && (
              <>
                <TextField label={t('ctb.minLength')} type="number" value={v('minLength')} onChange={(e) => set({ minLength: num(e.target.value) })} />
                <TextField label={t('ctb.maxLength')} type="number" value={v('maxLength')} onChange={(e) => set({ maxLength: num(e.target.value) })} />
              </>
            )}
            {type === 'string' && <TextField label={t('ctb.regex')} value={v('regex')} onChange={(e) => set({ regex: e.target.value || undefined })} />}
            {numeric && (
              <>
                <TextField label={t('ctb.min')} type="number" value={v('min')} onChange={(e) => set({ min: num(e.target.value) })} />
                <TextField label={t('ctb.max')} type="number" value={v('max')} onChange={(e) => set({ max: num(e.target.value) })} />
              </>
            )}
          </div>
        )}
      </div>
    </Dialog>
  )
}
