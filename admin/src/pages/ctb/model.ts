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
