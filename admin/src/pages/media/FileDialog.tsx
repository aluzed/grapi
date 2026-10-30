import { useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Crop, Download, ExternalLink, RefreshCw, Save, Trash2, X } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { TextField } from '@/components/ui/field'
import { api, upload } from '@/lib/api'
import { showError } from '@/lib/form'
import { uploadOrigin } from '@/lib/media'
import { can } from '@/lib/permissions'
import type { MediaFile } from '@/lib/types'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { CropDialog } from './CropDialog'
import { Thumb } from './MediaBrowser'

export const absolute = (url: string) => new URL(url, uploadOrigin()).toString()

// FileDialog edits a file's details. onSaved and onDeleted let a media field
// follow the change without reloading the document.
export function FileDialog({ file, onClose, onSaved, onDeleted }: { file: MediaFile; onClose: () => void; onSaved?: (f: MediaFile) => void; onDeleted?: (id: number) => void }) {
  const { t, language } = useI18n()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const related = useQuery({
    queryKey: ['media', 'related', file.id],
    queryFn: () => api.get<{ uid: string; kind: string; contentType: string; documentId: string; locale: string | null; field: string; title: string }[]>(`/upload/files/${file.id}/related`),
  })
  const { permissions } = useAuth()
  const canUpdate = can(permissions, 'plugin::upload.assets.update')
  const [form, setForm] = useState({ name: file.name, alternativeText: file.alternativeText ?? '', caption: file.caption ?? '', focalPoint: file.focalPoint ?? null })
  const done = (msg: string) => {
    toast.success(msg)
    void queryClient.invalidateQueries({ queryKey: ['media'] })
    onClose()
  }
  const save = useMutation({
    mutationFn: () => api.put(`/upload/files/${file.id}`, form),
    onSuccess: () => {
      onSaved?.({ ...file, ...form, alternativeText: form.alternativeText || null, caption: form.caption || null })
      done(t('media.saved'))
    },
    onError: (e) => showError(e),
  })
  const remove = useMutation({
    mutationFn: () => api.del(`/upload/files/${file.id}`),
    onSuccess: () => {
      onDeleted?.(file.id)
      done(t('media.deleted'))
    },
    onError: (e) => showError(e),
  })
  const replaceInput = useRef<HTMLInputElement>(null)
  const [cropping, setCropping] = useState(false)
  const replace = useMutation({
    mutationFn: (f: File) => {
      const form = new FormData()
      form.append('files', f)
      return upload(`/upload?id=${file.id}`, form)
    },
    onSuccess: async () => {
      if (onSaved) onSaved(await api.get<MediaFile>(`/upload/files/${file.id}`))
      done(t('media.replaced'))
    },
    onError: (e) => showError(e),
  })
  return (
    <Dialog
      open
      title={t('media.details')}
      onClose={onClose}
      closeLabel={t('users.close')}
      footer={
        canUpdate && (
          <>
            <Button
              variant="destructive-outline"
              title={t('media.delete')}
              icon={<Trash2 className="h-4 w-4" />}
              className="w-full md:w-auto"
              onClick={async () => {
                if (await confirm({ title: t('media.delete'), message: t('media.deleteConfirm'), confirmLabel: t('media.delete'), destructive: true })) remove.mutate()
              }}
            >
              {t('media.delete')}
            </Button>
            <Button title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={save.isPending} onClick={() => save.mutate()} className="w-full md:w-auto">
              {t('common.save')}
            </Button>
          </>
        )
      }
    >
      <div className="flex flex-col gap-4">
        <div className="flex aspect-video items-center justify-center overflow-hidden rounded-md bg-background">
          {file.mime.startsWith('image/') ? (
            <span className="relative inline-block max-h-full">
              <img
                src={file.url}
                alt={file.alternativeText ?? file.name}
                title={canUpdate ? t('media.focalHelp') : undefined}
                onClick={(e) => {
                  if (!canUpdate) return
                  const r = e.currentTarget.getBoundingClientRect()
                  const round = (v: number) => Math.round(Math.min(Math.max(v, 0), 1) * 1000) / 10
                  setForm({ ...form, focalPoint: { x: round((e.clientX - r.left) / r.width), y: round((e.clientY - r.top) / r.height) } })
                }}
                className={`block max-h-[40vh] max-w-full object-contain ${canUpdate ? 'cursor-crosshair' : ''}`}
              />
              {form.focalPoint && (
                <span
                  aria-hidden
                  className="pointer-events-none absolute h-5 w-5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white bg-primary/70 shadow"
                  style={{ left: `${form.focalPoint.x}%`, top: `${form.focalPoint.y}%` }}
                />
              )}
            </span>
          ) : (
            <Thumb file={file} />
          )}
        </div>
        {file.mime.startsWith('image/') && canUpdate && (
          <div className="flex flex-col gap-2 text-xs text-muted md:flex-row md:items-center md:justify-between">
            <span>{form.focalPoint ? t('media.focalSet', { x: String(form.focalPoint.x), y: String(form.focalPoint.y) }) : t('media.focalHelp')}</span>
            {form.focalPoint && (
              <Button variant="outline" title={t('media.focalClear')} icon={<X className="h-4 w-4" />} onClick={() => setForm({ ...form, focalPoint: null })} className="w-full md:w-auto">
                {t('media.focalClear')}
              </Button>
            )}
          </div>
        )}
        <dl className="grid grid-cols-2 gap-2 text-xs">
          <dt className="text-muted">{t('media.size')}</dt>
          <dd>{file.size.toLocaleString(language)} KB</dd>
          {file.width && (
            <>
              <dt className="text-muted">{t('media.dimensions')}</dt>
              <dd>
                {file.width} × {file.height}
              </dd>
            </>
          )}
          <dt className="text-muted">{t('media.mime')}</dt>
          <dd className="break-all">{file.mime}</dd>
          <dt className="text-muted">{t('media.createdAt')}</dt>
          <dd>{new Date(file.createdAt).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'short' })}</dd>
          {file.formats && Object.keys(file.formats).length > 0 && (
            <>
              <dt className="text-muted">{t('media.formats')}</dt>
              <dd className="flex flex-col gap-0.5">
                {Object.entries(file.formats)
                  .sort(([, a], [, b]) => a.width - b.width)
                  .map(([name, f]) => (
                    <span key={name}>
                      {name} ({f.width} × {f.height})
                    </span>
                  ))}
              </dd>
            </>
          )}
        </dl>
        <div className="text-xs">
          <p className="mb-1 font-semibold text-muted">{t('media.usedIn')}</p>
          {related.data && related.data.length === 0 && <p className="text-muted">{t('media.unused')}</p>}
          <ul className="flex flex-col gap-2">
            {related.data?.map((r) => (
              <li key={`${r.uid}-${r.documentId}-${r.locale}-${r.field}`} className="flex flex-col gap-2 rounded-md border border-border p-2 md:flex-row md:items-center md:justify-between">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{r.title}</p>
                  <p className="text-muted">
                    {r.contentType} · {r.field}
                    {r.locale ? ` · ${r.locale}` : ''}
                  </p>
                </div>
                <Button
                  variant="outline"
                  title={t('media.openEntry')}
                  icon={<ExternalLink className="h-4 w-4" />}
                  onClick={() =>
                    navigate(
                      `/content-manager/${r.kind === 'singleType' ? 'single-types' : 'collection-types'}/${encodeURIComponent(r.uid)}${r.kind === 'singleType' ? '' : `/${encodeURIComponent(r.documentId)}`}${r.locale ? `?locale=${r.locale}` : ''}`,
                    )
                  }
                  className="w-full md:w-auto"
                >
                  {t('media.openEntry')}
                </Button>
              </li>
            ))}
          </ul>
        </div>
        <div className="flex flex-col gap-2 md:flex-row md:flex-wrap *:whitespace-nowrap">
          <Button
            variant="outline"
            title={t('media.copy')}
            icon={<Copy className="h-4 w-4" />}
            className="w-full md:w-auto"
            onClick={() => void navigator.clipboard?.writeText(absolute(file.url)).then(() => toast.success(t('media.copied')))}
          >
            {t('media.copy')}
          </Button>
          {canUpdate && (
            <>
              <Button variant="outline" title={t('media.replace')} icon={<RefreshCw className="h-4 w-4" />} loading={replace.isPending} onClick={() => replaceInput.current?.click()} className="w-full md:w-auto">
                {t('media.replace')}
              </Button>
              {['image/jpeg', 'image/png', 'image/webp'].includes(file.mime) && (
                <Button variant="outline" title={t('media.crop')} icon={<Crop className="h-4 w-4" />} onClick={() => setCropping(true)} className="w-full md:w-auto">
                  {t('media.crop')}
                </Button>
              )}
              <input
                ref={replaceInput}
                type="file"
                hidden
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  if (f) replace.mutate(f)
                }}
              />
            </>
          )}
          <a href={file.url} download={file.name} title={t('media.download')} className="inline-flex min-h-10 w-full items-center justify-center gap-2 rounded-md border border-border bg-surface px-4 py-2 text-sm font-medium hover:bg-background md:w-auto">
            <Download className="h-4 w-4" />
            {t('media.download')}
          </a>
        </div>
        <fieldset disabled={!canUpdate} className="flex flex-col gap-4">
          <TextField label={t('media.name')} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <TextField label={t('media.alt')} value={form.alternativeText} onChange={(e) => setForm({ ...form, alternativeText: e.target.value })} />
          <TextField label={t('media.caption')} value={form.caption} onChange={(e) => setForm({ ...form, caption: e.target.value })} />
        </fieldset>
      </div>
      {cropping && <CropDialog file={file} folder={file.folder?.id ?? null} onClose={() => setCropping(false)} onDone={onClose} />}
    </Dialog>
  )
}
