export type Role = { id: number; name: string; code: string; description: string }

export type AdminUser = {
  id: number
  firstname: string
  lastname: string
  username: string
  email: string
  isActive: boolean
  blocked: boolean
  preferedLanguage: string
  roles: Role[]
}

export type Permission = {
  action: string
  subject: string | null
  properties: { fields?: string[] | null; locales?: string[] | null }
  conditions: string[]
}

export type AdminSession = {
  id: string
  deviceId: string
  type: 'refresh' | 'session'
  createdAt: string
  expiresAt: string
  metadata: { userAgent?: string; loginAt?: string } | null
  isCurrent: boolean
}

export type Information = {
  apiVersion: string
  develop: boolean
  languages: string[]
  version: string
  goVersion: string
  database: string
  environment: string
  contentTypes: number
  edition: string
}

export type Attribute = {
  type: string
  required?: boolean
  unique?: boolean
  enum?: string[]
  relation?: string
  target?: string
  multiple?: boolean
  component?: string
  repeatable?: boolean
  targetField?: string
  default?: unknown
  components?: string[]
  allowedTypes?: string[]
  pluginOptions?: { i18n?: { localized?: boolean } }
}

export type Model = {
  uid: string
  kind: 'collectionType' | 'singleType'
  info: { displayName: string; singularName: string; pluralName: string; description?: string }
  draftAndPublish: boolean
  localized: boolean
  attributes: Record<string, Attribute>
  fields: string[]
  mainField: string
  listColumns: string[]
  labels: Record<string, string>
  settings: { pageSize: number; defaultSortBy: string; defaultSortOrder: 'ASC' | 'DESC'; searchable: boolean; filterable: boolean; bulkable: boolean }
  permissions: { create: boolean; read: boolean; update: boolean; delete: boolean; publish: boolean }
  editLayout: { name: string; size: number }[][]
  editMeta: Record<string, { label: string; description: string; placeholder: string; visible: boolean; editable: boolean }>
  canConfigure: boolean
}

export type Entry = Record<string, unknown> & { id: number; documentId: string; status?: 'draft' | 'published' | 'modified' }

export type Pagination = { page: number; pageSize: number; pageCount: number; total: number }

export type Locale = { id: number; code: string; name: string; isDefault: boolean }

export type BulkResult = { count: number; documentIds: string[]; errors: { documentId: string; message: string }[] }

export type RoleWithCount = Role & { usersCount: number }

export type CatalogAction = { actionId: string; displayName: string; plugin: string; category?: string; subjects?: string[]; applyToProperties?: string[] }

export type Catalog = {
  sections: { contentTypes: CatalogAction[]; plugins: CatalogAction[]; settings: CatalogAction[] }
  conditions: { id: string; displayName: string; category: string }[]
}

export type ComponentModel = { uid: string; category: string; info: { displayName: string; icon?: string }; attributes: Record<string, Attribute>; fields: string[] }

export type MediaFile = {
  id: number
  documentId: string
  name: string
  alternativeText: string | null
  caption: string | null
  width: number | null
  height: number | null
  formats: Record<string, { url: string; width: number; height: number }> | null
  ext: string
  mime: string
  size: number
  url: string
  createdAt: string
  folder?: { id: number; name: string; path: string } | null
  focalPoint?: { x: number; y: number } | null
}

export type MediaFolder = { id: number; name: string; path: string; pathId: number; filesCount: number; childrenCount: number; totalFiles?: number; totalFolders?: number }
