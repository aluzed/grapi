export type ReleaseStatus = 'empty' | 'ready' | 'blocked' | 'failed' | 'done'

export type ReleaseAction = {
  id: number
  type: 'publish' | 'unpublish'
  uid: string
  documentId: string
  locale: string | null
  title?: string
  contentType?: string
  state?: 'ready' | 'not-ready' | 'already-published' | 'already-unpublished'
  errors?: string[]
}

export type Release = {
  id: number
  name: string
  scheduledAt: string | null
  timezone: string | null
  status: ReleaseStatus
  releasedAt: string | null
  error: string | null
  createdAt: string
  count: number
  actions?: ReleaseAction[]
}

export const statusTone: Record<string, string> = {
  done: 'border-success/20 bg-success/10 text-success',
  ready: 'border-primary/20 bg-primary/10 text-primary',
  blocked: 'border-amber-200 bg-amber-50 text-amber-800',
  failed: 'border-destructive/20 bg-destructive/10 text-destructive',
  'not-ready': 'border-destructive/20 bg-destructive/10 text-destructive',
  empty: 'border-zinc-200 bg-zinc-100 text-zinc-700',
}
