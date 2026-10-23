import type { Attribute, Model } from '@/lib/types'
import { useI18n } from '@/providers/I18nProvider'

type Media = { url?: string; name?: string; mime?: string; formats?: { thumbnail?: { url?: string } } }

const mainOf = (v: Record<string, unknown>) =>
  String(v.title ?? v.name ?? v.username ?? v.email ?? v.label ?? v.slug ?? v.documentId ?? v.id ?? '')

function MediaThumb({ m }: { m: Media }) {
  const src = m.formats?.thumbnail?.url ?? m.url
  if (src && m.mime?.startsWith('image/')) return <img src={src} alt={m.name ?? ''} className="h-8 w-8 rounded object-cover" loading="lazy" />
  return <span className="text-xs text-muted">{m.name}</span>
}

export function Cell({ value, attr, models }: { value: unknown; attr?: Attribute; models: Model[] }) {
  const { t, language } = useI18n()
  if (value === null || value === undefined || value === '') return <span className="text-muted">-</span>
  switch (attr?.type) {
    case 'boolean':
      return <>{value ? t('cm.yes') : t('cm.no')}</>
    case 'date':
      return <>{new Date(String(value)).toLocaleDateString(language)}</>
    case 'datetime':
    case 'timestamp':
      return <>{new Date(String(value)).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'short' })}</>
    case 'media': {
      const list = (Array.isArray(value) ? value : [value]) as Media[]
      return (
        <span className="flex gap-1">
          {list.slice(0, 3).map((m, i) => (
            <MediaThumb key={i} m={m} />
          ))}
        </span>
      )
    }
    case 'relation': {
      const target = models.find((m) => m.uid === attr.target)
      const main = (v: Record<string, unknown>) => (target ? String(v[target.mainField] ?? v.documentId ?? '') : mainOf(v))
      if (Array.isArray(value)) {
        const items = value as Record<string, unknown>[]
        return <span className="line-clamp-2">{items.map(main).join(', ') || '-'}</span>
      }
      return <>{main(value as Record<string, unknown>)}</>
    }
    case 'json':
      return <code className="line-clamp-1 text-xs">{JSON.stringify(value)}</code>
  }
  if (typeof value === 'object') return <code className="line-clamp-1 text-xs">{JSON.stringify(value)}</code>
  return <span className="line-clamp-2 break-words">{String(value)}</span>
}

export function StatusBadge({ status }: { status?: string }) {
  const { t } = useI18n()
  if (!status) return null
  const styles: Record<string, string> = {
    draft: 'bg-zinc-100 text-zinc-700 border-zinc-200',
    published: 'bg-success/10 text-success border-success/20',
    modified: 'bg-amber-50 text-amber-800 border-amber-200',
  }
  const key = `cm.status.${status}` as 'cm.status.draft'
  return <span className={`inline-flex rounded border px-1.5 py-0.5 text-xs font-medium ${styles[status] ?? ''}`}>{t(key)}</span>
}
