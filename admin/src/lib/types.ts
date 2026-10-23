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

export type Information = { apiVersion: string; develop: boolean; languages: string[] }

export type Attribute = {
  type: string
  required?: boolean
  enum?: string[]
  relation?: string
  target?: string
  multiple?: boolean
  component?: string
  repeatable?: boolean
  targetField?: string
  default?: unknown
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
}

export type Entry = Record<string, unknown> & { id: number; documentId: string; status?: 'draft' | 'published' | 'modified' }

export type Pagination = { page: number; pageSize: number; pageCount: number; total: number }

export type Locale = { code: string; name: string; isDefault: boolean }

export type BulkResult = { count: number; documentIds: string[]; errors: { documentId: string; message: string }[] }
