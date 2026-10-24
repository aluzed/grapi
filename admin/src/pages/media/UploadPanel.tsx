import { useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { File as FileIcon, Link2, Plus, Upload, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { SelectField } from '@/components/ui/field'
import { api, ApiError, upload } from '@/lib/api'
import { cn } from '@/lib/cn'
import { accepts } from '@/lib/media'
import type { MediaFile, MediaFolder } from '@/lib/types'
import { useI18n } from '@/providers/I18nProvider'

type Status = 'pending' | 'uploading' | 'done' | 'error' | 'cancelled'

type Entry = { key: number; file?: File; url?: string; status: Status; progress: number; error?: string; rejected?: boolean; result?: MediaFile[] }

let seq = 0

// UploadPanel is the "add files" side of the media picker: a queue of
// local files and URLs, sent one by one so that each shows its own progress
// and error, then handed back to select them.
export function UploadPanel({ allowedTypes, onDone }: { allowedTypes?: string[]; onDone: (files: MediaFile[]) => void }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const folders = useQuery({ queryKey: ['media', 'folders', 'all'], queryFn: () => api.get<MediaFolder[]>('/upload/folders?all=true') })
  const [dest, setDest] = useState('root')
  const [queue, setQueue] = useState<Entry[]>([])
  const [urls, setUrls] = useState('')
  const [dragging, setDragging] = useState(false)
  const [running, setRunning] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const aborts = useRef(new Map<number, AbortController>())

  const patch = (key: number, p: Partial<Entry>) => setQueue((q) => q.map((e) => (e.key === key ? { ...e, ...p } : e)))
  const addFiles = (list: FileList | null) => {
    if (!list?.length) return
    setQueue((q) => [
      ...q,
      ...[...list].map((file): Entry => {
        const ok = accepts(allowedTypes, file.type || 'application/octet-stream')
        return { key: ++seq, file, status: ok ? 'pending' : 'error', progress: 0, error: ok ? undefined : t('media.notAllowedType'), rejected: !ok }
      }),
    ])
  }
  const addUrls = () => {
    const list = urls
      .split('\n')
      .map((u) => u.trim())
      .filter(Boolean)
    setQueue((q) => [...q, ...list.map((url): Entry => ({ key: ++seq, url, status: 'pending', progress: 0 }))])
    setUrls('')
  }

  const sendOne = async (e: Entry, folder: number | null): Promise<MediaFile[]> => {
    const ctrl = new AbortController()
    aborts.current.set(e.key, ctrl)
    patch(e.key, { status: 'uploading', progress: 0, error: undefined })
    try {
      let res: MediaFile[]
      if (e.file) {
        const form = new FormData()
        form.append('files', e.file)
        res = await upload<MediaFile[]>(`/upload?folder=${folder ?? 'root'}`, form, { onProgress: (p) => patch(e.key, { progress: p }), signal: ctrl.signal })
      } else {
        res = await api.post<MediaFile[]>('/upload/actions/upload-from-urls', { urls: [e.url], folder })
      }
      const kept = res.filter((f) => accepts(allowedTypes, f.mime))
      patch(e.key, kept.length ? { status: 'done', progress: 1, result: kept } : { status: 'error', error: t('media.notAllowedType'), rejected: true })
      return kept
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') patch(e.key, { status: 'cancelled' })
      else patch(e.key, { status: 'error', error: err instanceof ApiError ? err.fieldErrors()[0]?.message || err.message : t('common.error') })
      return []
    } finally {
      aborts.current.delete(e.key)
    }
  }

  const start = async () => {
    setRunning(true)
    const folder = dest === 'root' ? null : Number(dest)
    const out: MediaFile[] = []
    for (const e of queue.filter((x) => x.status === 'pending' || x.status === 'cancelled' || (x.status === 'error' && !x.rejected))) {
      out.push(...(await sendOne(e, folder)))
    }
    setRunning(false)
    void queryClient.invalidateQueries({ queryKey: ['media'] })
    const done = [...queue.flatMap((e) => (e.status === 'done' ? (e.result ?? []) : [])), ...out]
    if (done.length) onDone(done)
  }

  const pending = queue.some((e) => e.status !== 'done' && !e.rejected)

  return (
    <div className="flex flex-col gap-4">
      <SelectField label={t('media.moveTo')} value={dest} onChange={(e) => setDest(e.target.value)} className="md:w-72">
        <option value="root">{t('media.root')}</option>
        {folders.data?.map((f) => (
          <option key={f.id} value={f.id}>
            {'· '.repeat(Math.max(0, f.path.split('/').length - 2))}
            {f.name}
          </option>
        ))}
      </SelectField>
      <div
        className={cn('flex flex-col items-center gap-3 rounded-lg border-2 border-dashed p-6 text-center', dragging ? 'border-primary bg-primary/5' : 'border-border')}
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault()
          setDragging(false)
          addFiles(e.dataTransfer.files)
        }}
      >
        <Upload className="h-8 w-8 text-muted" aria-hidden />
        <p className="text-sm text-muted">{t('media.drop')}</p>
        <Button variant="outline" title={t('media.browseComputer')} icon={<Plus className="h-4 w-4" />} onClick={() => input.current?.click()} className="w-full md:w-auto">
          {t('media.browseComputer')}
        </Button>
        <input ref={input} type="file" multiple hidden onChange={(e) => addFiles(e.target.files)} />
      </div>
      <label className="flex flex-col gap-1.5 text-sm">
        <span className="font-medium">{t('media.urls')}</span>
        <textarea
          value={urls}
          rows={3}
          placeholder="https://"
          onChange={(e) => setUrls(e.target.value)}
          className="rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-primary focus:outline-none"
        />
        <span className="text-xs text-muted">{t('media.urlsHelp')}</span>
      </label>
      <div>
        <Button variant="outline" title={t('media.addUrls')} icon={<Link2 className="h-4 w-4" />} disabled={!urls.trim()} onClick={addUrls} className="w-full md:w-auto">
          {t('media.addUrls')}
        </Button>
      </div>
      {queue.length > 0 && (
        <ul className="flex flex-col gap-2" aria-label={t('media.queue')}>
          {queue.map((e) => (
            <li key={e.key} className="flex flex-col gap-2 rounded-md border border-border bg-surface p-2 md:flex-row md:items-center">
              <span className="flex min-w-0 flex-1 items-center gap-2">
                <FileIcon className="h-4 w-4 shrink-0 text-muted" aria-hidden />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm">{e.file?.name ?? e.url}</span>
                  {e.status === 'uploading' && (
                    <span className="mt-1 block h-1.5 overflow-hidden rounded bg-background" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(e.progress * 100)}>
                      <span className="block h-full bg-primary" style={{ width: `${Math.round(e.progress * 100)}%` }} />
                    </span>
                  )}
                  {e.status === 'error' && <span className="block text-xs text-destructive">{e.error}</span>}
                  {e.status === 'done' && <span className="block text-xs text-success">{t('media.queueDone')}</span>}
                  {e.status === 'cancelled' && <span className="block text-xs text-muted">{t('media.uploadCancelled')}</span>}
                </span>
              </span>
              {e.status === 'uploading' ? (
                <Button variant="destructive-outline" title={t('media.cancelUpload')} icon={<X className="h-4 w-4" />} onClick={() => aborts.current.get(e.key)?.abort()} className="w-full md:w-auto">
                  {t('media.cancelUpload')}
                </Button>
              ) : (
                e.status !== 'done' && (
                  <Button variant="destructive-outline" title={t('media.queueRemove')} icon={<X className="h-4 w-4" />} disabled={running} onClick={() => setQueue((q) => q.filter((x) => x.key !== e.key))} className="w-full md:w-auto">
                    {t('media.queueRemove')}
                  </Button>
                )
              )}
            </li>
          ))}
        </ul>
      )}
      <div className="flex justify-end">
        <Button title={t('media.uploadQueue')} icon={<Upload className="h-4 w-4" />} loading={running} disabled={!pending} onClick={() => void start()} className="w-full md:w-auto">
          {t('media.uploadQueue')}
        </Button>
      </div>
    </div>
  )
}
