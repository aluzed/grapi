import { useState } from 'react'
import { Filter as FilterIcon, Plus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { SelectField, TextField } from '@/components/ui/field'
import type { Model } from '@/lib/types'
import type { MessageKey } from '@/i18n'
import { useI18n } from '@/providers/I18nProvider'
import { filterable, operators, type Filter } from './filters'
import { label } from './hooks'

const inputType = (t?: string) => ({ integer: 'number', float: 'number', decimal: 'number', date: 'date', datetime: 'datetime-local', time: 'time' })[t ?? ''] ?? 'text'

export function FiltersPanel({ model, value, onChange }: { model: Model; value: Filter[]; onChange: (f: Filter[]) => void }) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const fields = ['id', ...model.fields.filter((f) => model.attributes[f] && filterable(model.attributes[f]!))]
  const [draft, setDraft] = useState<Filter>({ field: fields[1] ?? 'id', op: operators(model.attributes[fields[1] ?? ''])[0]!, value: '' })
  const attr = model.attributes[draft.field]
  const opLabel = (op: string) => t(`filters.op.${op}` as MessageKey)
  const noValue = draft.op === '$null' || draft.op === '$notNull'
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-col gap-2 md:flex-row md:flex-wrap md:items-center">
        <Button variant="outline" title={t('filters.title')} icon={<FilterIcon className="h-4 w-4" />} onClick={() => setOpen((o) => !o)} aria-expanded={open} className="w-full md:w-auto">
          {t('filters.title')}
          {value.length > 0 ? ` (${value.length})` : ''}
        </Button>
        {value.map((f, i) => (
          <span key={i} className="inline-flex items-center justify-between gap-2 rounded-md border border-primary/30 bg-primary/5 px-2 py-1 text-xs">
            <span>
              {label(model, f.field)} {opLabel(f.op)} {f.op === '$null' || f.op === '$notNull' ? '' : <strong>{f.value}</strong>}
            </span>
            <button type="button" title={t('filters.remove')} aria-label={t('filters.remove')} onClick={() => onChange(value.filter((_, j) => j !== i))} className="rounded p-1 text-muted hover:text-destructive">
              <X className="h-3.5 w-3.5" />
            </button>
          </span>
        ))}
        {value.length > 1 && (
          <Button variant="outline" title={t('filters.clear')} icon={<X className="h-4 w-4" />} onClick={() => onChange([])} className="w-full md:w-auto">
            {t('filters.clear')}
          </Button>
        )}
      </div>
      {open && (
        <form
          className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-3 md:flex-row md:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            onChange([...value, draft])
            setDraft({ ...draft, value: '' })
            setOpen(false)
          }}
        >
          <SelectField
            label={t('filters.field')}
            value={draft.field}
            onChange={(e) => setDraft({ field: e.target.value, op: operators(model.attributes[e.target.value])[0]!, value: '' })}
            className="md:w-56"
          >
            {fields.map((f) => (
              <option key={f} value={f}>
                {label(model, f)}
              </option>
            ))}
          </SelectField>
          <SelectField label={t('filters.operator')} value={draft.op} onChange={(e) => setDraft({ ...draft, op: e.target.value })} className="md:w-56">
            {operators(attr).map((op) => (
              <option key={op} value={op}>
                {opLabel(op)}
              </option>
            ))}
          </SelectField>
          {!noValue &&
            (attr?.type === 'boolean' ? (
              <SelectField label={t('filters.value')} value={draft.value} onChange={(e) => setDraft({ ...draft, value: e.target.value })} className="md:w-40">
                <option value="true">{t('filters.true')}</option>
                <option value="false">{t('filters.false')}</option>
              </SelectField>
            ) : attr?.type === 'enumeration' ? (
              <SelectField label={t('filters.value')} value={draft.value} onChange={(e) => setDraft({ ...draft, value: e.target.value })} className="md:w-56">
                <option value="">-</option>
                {attr.enum?.map((o) => (
                  <option key={o} value={o}>
                    {o}
                  </option>
                ))}
              </SelectField>
            ) : (
              <TextField label={t('filters.value')} type={inputType(attr?.type)} value={draft.value} onChange={(e) => setDraft({ ...draft, value: e.target.value })} className="md:flex-1" />
            ))}
          <Button type="submit" title={t('filters.apply')} icon={<Plus className="h-4 w-4" />} disabled={!noValue && draft.value === '' && attr?.type !== 'boolean'} className="w-full md:w-auto">
            {t('filters.apply')}
          </Button>
        </form>
      )}
    </div>
  )
}
