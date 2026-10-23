export type Attr = Record<string, unknown> & { name: string; type: string }

export type SchemaFile = {
  uid: string
  kind?: 'collectionType' | 'singleType'
  category?: string
  info: { displayName?: string; singularName?: string; pluralName?: string; description?: string; icon?: string }
  options: { draftAndPublish?: boolean } & Record<string, unknown>
  pluginOptions: { i18n?: { localized?: boolean } } & Record<string, unknown>
  attributes: Attr[]
  editable: boolean
}

export type Action = 'create' | 'update' | 'delete'

export const kebab = (s: string) =>
  s
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')

export const pluralize = (s: string) => (s.endsWith('s') ? `${s}es` : s.endsWith('y') ? `${s.slice(0, -1)}ies` : `${s}s`)

export const fieldTypes = [
  'string', 'text', 'richtext', 'blocks', 'email', 'password', 'uid', 'enumeration', 'integer', 'biginteger', 'float', 'decimal',
  'date', 'datetime', 'time', 'boolean', 'json', 'media', 'relation', 'component', 'dynamiczone',
] as const

export const relationKinds = ['oneWay', 'oneToOne', 'oneToMany', 'manyToOne', 'manyToMany', 'manyWay'] as const
export const bidirectional = new Set(['oneToOne', 'oneToMany', 'manyToOne', 'manyToMany'])

// What a deletion also changes elsewhere, as the server applies it.
export type Impact = { owner: string; field: string; kind: 'relation' | 'component' | 'zone' | 'inverse' }

const nameOf = (f: SchemaFile) => f.info.displayName ?? f.uid

export function typeDeletionImpacts(uid: string, all: SchemaFile[]): Impact[] {
  const out: Impact[] = []
  for (const f of all) {
    if (f.uid === uid) continue
    for (const a of f.attributes) {
      if (a.type === 'relation' && a.target === uid) out.push({ owner: nameOf(f), field: a.name, kind: 'relation' })
      else if (a.type === 'component' && a.component === uid) out.push({ owner: nameOf(f), field: a.name, kind: 'component' })
      else if (a.type === 'dynamiczone' && Array.isArray(a.components) && a.components.includes(uid)) out.push({ owner: nameOf(f), field: a.name, kind: 'zone' })
    }
  }
  return out
}

export function fieldDeletionImpacts(owner: string, attr: Attr, all: SchemaFile[]): Impact[] {
  if (attr.type !== 'relation') return []
  const other = attr.inversedBy ?? attr.mappedBy ?? attr.targetAttribute
  const target = all.find((f) => f.uid === attr.target)
  if (typeof other !== 'string' || !other || !target) return []
  const back = target.attributes.find((a) => a.name === other)
  if (!back || (back.inversedBy !== attr.name && back.mappedBy !== attr.name)) return []
  return target.uid === owner ? [] : [{ owner: nameOf(target), field: other, kind: 'inverse' }]
}
