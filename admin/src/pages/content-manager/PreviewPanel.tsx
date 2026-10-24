import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Columns2, Copy, ExternalLink, Maximize2, Monitor, X } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Tabs } from '@/components/ui/tabs'
import { api, ApiError } from '@/lib/api'
import { cn } from '@/lib/cn'
import type { Model } from '@/lib/types'
import { useI18n } from '@/providers/I18nProvider'

export type PreviewMode = 'full' | 'side'

type PreviewURL = { url: string; origin: string }

const modeKey = 'grapi.preview.mode'

export function storedPreviewMode(): PreviewMode {
  try {
    return localStorage.getItem(modeKey) === 'side' ? 'side' : 'full'
  } catch {
    return 'full'
  }
}

function storeMode(m: PreviewMode) {
  try {
    localStorage.setItem(modeKey, m)
  } catch {
    // Private mode: the choice lasts for the page.
  }
}

export function usePreviewURL(model: Model, documentId: string | undefined, locale: string, status: 'draft' | 'published', enabled: boolean) {
  const params = new URLSearchParams({ status })
  if (documentId) params.set('documentId', documentId)
  if (locale) params.set('locale', locale)
  return useQuery({
    queryKey: ['cm', 'preview', model.uid, documentId, locale, status],
    queryFn: () => api.get<PreviewURL | undefined>(`/preview/url/${encodeURIComponent(model.uid)}?${params}`).then((v) => v ?? null),
    enabled,
    retry: false,
  })
}

// PreviewPanel shows the front in a frame. After each save (saves counts
// them) it posts strapiUpdate to the front, once the front has said
// previewReady; a front that never did is reloaded instead.
export function PreviewPanel({
  model,
  documentId,
  locale,
  everPublished,
  saves,
  mode,
  onMode,
  onClose,
}: {
  model: Model
  documentId?: string
  locale: string
  everPublished: boolean
  saves: number
  mode: PreviewMode
  onMode: (m: PreviewMode) => void
  onClose: () => void
}) {
  const { t } = useI18n()
  const [status, setStatus] = useState<'draft' | 'published'>('draft')
  const [device, setDevice] = useState<'desktop' | 'mobile'>('desktop')
  const [reload, setReload] = useState(0)
  const frame = useRef<HTMLIFrameElement>(null)
  const ready = useRef(false)
  const preview = usePreviewURL(model, documentId, locale, status, true)
  const target = preview.data

  useEffect(() => {
    ready.current = false
    if (!target) return
    const onMessage = (e: MessageEvent) => {
      // Only the framed front, from its own origin, is listened to.
      if (e.origin !== target.origin || e.source !== frame.current?.contentWindow) return
      if ((e.data as { type?: string } | null)?.type === 'previewReady') ready.current = true
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [target?.url, reload])

  const first = useRef(saves)
  useEffect(() => {
    if (saves === first.current || !target) return
    // A saved value may be in the URL (a slug): a new URL loads by itself.
    void preview.refetch().then((res) => {
      if (res.data?.url !== target.url) return
      if (ready.current) frame.current?.contentWindow?.postMessage({ type: 'strapiUpdate' }, target.origin)
      else setReload((n) => n + 1)
    })
  }, [saves])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && mode === 'full' && !document.querySelector('dialog[open]')) onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [mode, onClose])

  const error = preview.error instanceof ApiError ? preview.error.message : preview.error ? t('common.error') : ''

  return (
    <section
      aria-label={t('preview.title')}
      className={cn('flex flex-col overflow-hidden border border-border bg-surface', mode === 'full' ? 'fixed inset-0 z-50' : 'h-[calc(100dvh-3rem)] rounded-lg lg:sticky lg:top-6')}
    >
      <div className="flex flex-col gap-2 border-b border-border p-3 xl:flex-row xl:flex-wrap xl:items-center">
        <h2 className="text-base font-semibold xl:mr-auto">{t('preview.title')}</h2>
        {model.draftAndPublish && (
          <Tabs
            tabs={[
              { id: 'draft', label: t('cm.status.draft') },
              { id: 'published', label: t('cm.status.published'), disabled: !everPublished, hint: t('preview.neverPublished') },
            ]}
            value={status}
            onChange={(id) => setStatus(id as 'draft' | 'published')}
          />
        )}
        <Tabs
          tabs={[
            { id: 'desktop', label: t('preview.desktop') },
            { id: 'mobile', label: t('preview.mobile') },
          ]}
          value={device}
          onChange={(id) => setDevice(id as 'desktop' | 'mobile')}
        />
        <div className="flex flex-col gap-2 md:flex-row md:flex-wrap">
          <Button
            variant="outline"
            title={t('preview.copy')}
            icon={<Copy className="h-4 w-4" />}
            disabled={!target}
            onClick={() => target && void navigator.clipboard?.writeText(target.url).then(() => toast.success(t('preview.copied')))}
            className="w-full md:w-auto"
          >
            {t('preview.copy')}
          </Button>
          <Button variant="outline" title={t('preview.newTab')} icon={<ExternalLink className="h-4 w-4" />} disabled={!target} onClick={() => target && window.open(target.url, '_blank', 'noopener')} className="w-full md:w-auto">
            {t('preview.newTab')}
          </Button>
          {mode === 'full' ? (
            // Side by side needs the width of a large screen.
            <span className="hidden lg:block">
              <Button variant="outline" title={t('preview.side')} icon={<Columns2 className="h-4 w-4" />} onClick={() => (storeMode('side'), onMode('side'))}>
                {t('preview.side')}
              </Button>
            </span>
          ) : (
            <Button variant="outline" title={t('preview.full')} icon={<Maximize2 className="h-4 w-4" />} onClick={() => (storeMode('full'), onMode('full'))} className="w-full md:w-auto">
              {t('preview.full')}
            </Button>
          )}
          <Button variant="outline" title={t('preview.close')} icon={<X className="h-4 w-4" />} onClick={onClose} className="w-full md:w-auto">
            {t('preview.close')}
          </Button>
        </div>
      </div>
      <div className="flex min-h-0 flex-1 justify-center overflow-auto bg-background p-2">
        {error ? (
          <p role="alert" className="m-auto max-w-md text-center text-sm text-destructive">
            {error}
          </p>
        ) : target ? (
          <iframe
            key={`${target.url}-${reload}`}
            ref={frame}
            title={t('preview.title')}
            src={target.url}
            className={cn('h-full rounded border border-border bg-white', device === 'mobile' ? 'w-[375px] max-w-full' : 'w-full')}
          />
        ) : (
          <p className="m-auto text-sm text-muted">{preview.isPending ? t('common.loading') : t('preview.none')}</p>
        )}
      </div>
    </section>
  )
}

export function PreviewButton({ disabled, onOpen }: { disabled: boolean; onOpen: () => void }) {
  const { t } = useI18n()
  return (
    <Button variant="outline" title={disabled ? t('preview.saveFirst') : t('preview.open')} icon={<Monitor className="h-4 w-4" />} disabled={disabled} onClick={onOpen} className="w-full">
      {t('preview.open')}
    </Button>
  )
}
