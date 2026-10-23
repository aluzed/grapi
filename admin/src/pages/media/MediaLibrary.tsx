import { useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Copy, Download, RefreshCw, Save, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { TextField } from '@/components/ui/field'
import { api, upload } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import type { MediaFile } from '@/lib/types'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { MediaBrowser, Thumb } from './MediaBrowser'

const absolute = (url: string) => new URL(url, window.location.origin).toString()

function FileDialog({ file, onClose }: { file: MediaFile; onClose: () => void }) {
  const { t, language } = useI18n()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const { permissions } = useAuth()
  const canUpdate = can(permissions, 'plugin::upload.assets.update')
  const [form, setForm] = useState({ name: file.name, alternativeText: file.alternativeText ?? '', caption: file.caption ?? '' })
  const done = (msg: string) => {
    toast.success(msg)
    void queryClient.invalidateQueries({ queryKey: ['media'] })
    onClose()
  }
  const save = useMutation({ mutationFn: () => api.put(`/upload/files/${file.id}`, form), onSuccess: () => done(t('media.saved')), onError: (e) => showError(e) })
  const remove = useMutation({ mutationFn: () => api.del(`/upload/files/${file.id}`), onSuccess: () => done(t('media.deleted')), onError: (e) => showError(e) })
  const replaceInput = useRef<HTMLInputElement>(null)
  const replace = useMutation({
    mutationFn: (f: File) => {
      const form = new FormData()
      form.append('files', f)
      return upload(`/upload?id=${file.id}`, form)
    },
    onSuccess: () => done(t('media.replaced')),
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
          {file.mime.startsWith('image/') ? <img src={file.url} alt={file.alternativeText ?? file.name} className="max-h-full max-w-full object-contain" /> : <Thumb file={file} />}
        </div>
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
        </dl>
        <div className="flex flex-col gap-2 md:flex-row">
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
    </Dialog>
  )
}

export function MediaLibrary() {
  const { t } = useI18n()
  const [open, setOpen] = useState<MediaFile | null>(null)
  return (
    <div className="mx-auto max-w-6xl">
      <PageHeader title={t('media.title')} />
      <MediaBrowser onOpen={setOpen} />
      {open && <FileDialog key={open.id} file={open} onClose={() => setOpen(null)} />}
    </div>
  )
}
