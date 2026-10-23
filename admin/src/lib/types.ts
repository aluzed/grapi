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
