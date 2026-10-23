import type { Attribute, Model } from '@/lib/types'

export type Filter = { field: string; op: string; value: string }

const text = ['$containsi', '$notContainsi', '$eq', '$ne', '$startsWithi', '$endsWithi', '$null', '$notNull']
const ordered = ['$eq', '$ne', '$gt', '$gte', '$lt', '$lte', '$null', '$notNull']

export function operators(attr: Attribute | undefined): string[] {
  switch (attr?.type) {
    case 'integer':
    case 'biginteger':
    case 'float':
    case 'decimal':
    case 'date':
    case 'datetime':
    case 'time':
    case 'timestamp':
      return ordered
    case 'boolean':
      return ['$eq', '$null', '$notNull']
    case 'enumeration':
      return ['$eq', '$ne', '$null', '$notNull']
    case 'media':
      return ['$null', '$notNull']
  }
  return text
}

export const filterable = (attr: Attribute) => !['json', 'password', 'richtext', 'blocks', 'dynamiczone', 'component'].includes(attr.type) && !(attr.relation ?? '').startsWith('morph')

const typed = (attr: Attribute | undefined, v: string): unknown => {
  switch (attr?.type) {
    case 'integer':
    case 'float':
    case 'decimal':
      return Number(v)
    case 'boolean':
      return v === 'true'
  }
  return v
}

// toQuery is the REST filters object of a filter list: every filter must
// match. A relation is filtered on its target's main field.
export function toQuery(list: Filter[], model: Model, models: Model[]): Record<string, unknown> | null {
  const parts = list
    .filter((f) => model.attributes[f.field] || f.field === 'id')
    .map((f) => {
      const attr = model.attributes[f.field]
      const cond = f.op === '$null' || f.op === '$notNull' ? { [f.op]: true } : { [f.op]: typed(attr, f.value) }
      if (attr?.type === 'relation') {
        const main = models.find((m) => m.uid === attr.target)?.mainField ?? 'id'
        return { [f.field]: f.op === '$null' || f.op === '$notNull' ? { id: cond } : { [main]: cond } }
      }
      if (attr?.type === 'media') return { [f.field]: { id: cond } }
      return { [f.field]: cond }
    })
  if (parts.length === 0) return null
  return parts.length === 1 ? parts[0]! : { $and: parts }
}

export function parseFilters(raw: string | null): Filter[] {
  if (!raw) return []
  try {
    const v = JSON.parse(raw)
    return Array.isArray(v) ? v.filter((f) => f && typeof f.field === 'string' && typeof f.op === 'string') : []
  } catch {
    return []
  }
}
