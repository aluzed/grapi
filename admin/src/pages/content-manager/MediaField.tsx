import { useRef, useState } from 'react'
import { ArrowLeft, ArrowRight, Copy, File as FileIcon, ImagePlus, Pencil, Upload, X } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { upload } from '@/lib/api'
import { cn } from '@/lib/cn'
import { showError } from '@/lib/form'
import { checkDrop, previewOf, reorder } from '@/lib/media'
import { can } from '@/lib/permissions'
import type { Attribute, MediaFile } from '@/lib/types'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { absolute, FileDialog } from '../media/FileDialog'
import { MediaPicker } from '../media/MediaPicker'
import { Thumb } from '../media/MediaBrowser'

const small =
  'inline-flex min-h-9 items-center gap-1 rounded-md border px-2 text-xs font-medium disabled:cursor-not-allowed disabled:bg-zinc-200 disabled:text-zinc-500'

const reorderType = 'application/x-grapi-media-index'

export function mediaItems(v: unknown): MediaFile[] {
  if (!v) return []
  return (Array.isArray(v) ? v : [v]) as MediaFile[]
}

function Preview({ file }: { file: MediaFile }) {
  switch (previewOf(file.mime)) {
    case 'image':
      return <img src={file.formats?.medium?.url ?? file.url} alt={file.alternativeText ?? file.name} className="h-full w-full object-contain" />
    case 'video':
      return <video src={file.url} controls preload="metadata" className="h-full w-full" />
    case 'audio':
      return (
        <span className="flex h-full w-full items-center justify-center p-4">
          <audio src={file.url} controls preload="metadata" className="w-full" />
        </span>
      )
    case 'pdf':
      return (
        <object data={file.url} type="application/pdf" className="h-full w-full">
          <Thumb file={file} />
        </object>
      )
  }
  return (
    <span className="flex h-full w-full flex-col items-center justify-center gap-2 text-muted">
      <FileIcon className="h-10 w-10" aria-hidden />
      <span className="text-xs uppercase">{file.ext.replace('.', '')}</span>
    </span>
  )
}

export function MediaField({
  attr,
  label,
  value,
  error,
  onChange,
}: {
  attr: Attribute & { allowedTypes?: string[] }
  label: string
  value: MediaFile[]
  error?: string
  onChange: (v: MediaFile[]) => void
}) {
  const { t } = useI18n()
  const confirm = useConfirm()
  const { permissions } = useAuth()
  const canUpload = can(permissions, 'plugin::upload.assets.create')
  const multiple = Boolean(attr.multiple)
  const [picking, setPicking] = useState(false)
  const [editing, setEditing] = useState<MediaFile | null>(null)
  const [index, setIndex] = useState(0)
  const [dragging, setDragging] = useState(false)
  const [progress, setProgress] = useState<number | null>(null)
  const [dragFrom, setDragFrom] = useState<number | null>(null)
  const root = useRef<HTMLFieldSetElement>(null)
  const current = value[Math.min(index, value.length - 1)]
  const at = Math.min(index, Math.max(value.length - 1, 0))
  // A field made read only sits in a disabled fieldset, which does not stop drops.
  const readOnly = () => Boolean(root.current?.matches(':disabled'))

  const move = (from: number, to: number) => {
    const next = reorder(value, from, to)
    if (next === value) return
    onChange(next)
    if (from === at) setIndex(to)
  }
  const add = (files: MediaFile[]) => {
    if (!multiple) {
      onChange(files.slice(0, 1))
      setIndex(0)
      return
    }
    const known = new Set(value.map((f) => f.id))
    const fresh = files.filter((f) => !known.has(f.id))
    onChange([...value, ...fresh])
    if (fresh.length) setIndex(value.length)
  }
  const dropFiles = async (list: File[]) => {
    const err = checkDrop(list, { multiple, allowedTypes: attr.allowedTypes })
    if (err) return void toast.error(err === 'single' ? t('media.singleOnly') : t('media.notAllowedType'))
    const form = new FormData()
    for (const f of list) form.append('files', f)
    setProgress(0)
    try {
      add(await upload<MediaFile[]>('/upload?folder=root', form, { onProgress: setProgress }))
    } catch (e) {
      showError(e, undefined, t('common.error'))
    } finally {
      setProgress(null)
    }
  }
  const remove = async (f: MediaFile) => {
    if (!(await confirm({ title: t('rel.remove'), message: t('rel.removeConfirm'), confirmLabel: t('rel.remove'), destructive: true }))) return
    onChange(value.filter((x) => x.id !== f.id))
    setIndex(Math.max(0, at - 1))
  }

  return (
    <fieldset ref={root} className="flex min-w-0 flex-col gap-1.5 sm:col-span-2">
      <legend className="mb-1.5 text-sm font-medium">{label}</legend>
      <div
        className={cn('flex flex-col overflow-hidden rounded-md border bg-surface', dragging ? 'border-primary ring-2 ring-primary' : error ? 'border-destructive' : 'border-border')}
        onDragOver={(e) => {
          if (!canUpload || !e.dataTransfer.types.includes('Files') || readOnly()) return
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          if (!e.dataTransfer.types.includes('Files')) return
          e.preventDefault()
          setDragging(false)
          if (canUpload && !readOnly()) void dropFiles([...e.dataTransfer.files])
        }}
      >
        {current ? (
          <>
            <div className="relative flex h-56 items-center justify-center bg-background md:h-64">
              <Preview key={current.id} file={current} />
              {value.length > 1 && (
                <span className="absolute top-2 right-2 rounded bg-black/60 px-2 py-0.5 text-xs text-white">{`${at + 1} / ${value.length}`}</span>
              )}
            </div>
            <p className="truncate border-t border-border px-3 py-2 text-sm" title={current.name}>
              {current.name}
            </p>
          </>
        ) : (
          <div className="flex flex-col items-center gap-2 p-6 text-center text-sm text-muted">
            <Upload className="h-8 w-8" aria-hidden />
            <span>{canUpload ? t('media.fieldEmpty') : t('media.fieldEmptyNoUpload')}</span>
          </div>
        )}
        {progress !== null && (
          <div className="h-1.5 bg-background" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(progress * 100)}>
            <div className="h-full bg-primary" style={{ width: `${Math.round(progress * 100)}%` }} />
          </div>
        )}
        <div className="flex flex-col gap-2 border-t border-border p-2 md:flex-row md:flex-wrap md:items-center">
          {value.length > 1 && (
            <>
              <Button variant="outline" title={t('media.previous')} icon={<ArrowLeft className="h-4 w-4" />} disabled={at === 0} onClick={() => setIndex(at - 1)} className="w-full md:w-auto">
                {t('media.previous')}
              </Button>
              <Button variant="outline" title={t('media.next')} icon={<ArrowRight className="h-4 w-4" />} disabled={at === value.length - 1} onClick={() => setIndex(at + 1)} className="w-full md:w-auto">
                {t('media.next')}
              </Button>
            </>
          )}
          {(multiple || !current) && (
            <Button variant="outline" title={t('media.add')} icon={<ImagePlus className="h-4 w-4" />} onClick={() => setPicking(true)} className="w-full md:w-auto">
              {t('media.add')}
            </Button>
          )}
          {!multiple && current && (
            <Button variant="outline" title={t('media.change')} icon={<ImagePlus className="h-4 w-4" />} onClick={() => setPicking(true)} className="w-full md:w-auto">
              {t('media.change')}
            </Button>
          )}
          {current && (
            <>
              <Button variant="outline" title={t('media.edit')} icon={<Pencil className="h-4 w-4" />} onClick={() => setEditing(current)} className="w-full md:w-auto">
                {t('media.edit')}
              </Button>
              <Button
                variant="outline"
                title={t('media.copy')}
                icon={<Copy className="h-4 w-4" />}
                onClick={() => void navigator.clipboard?.writeText(absolute(current.url)).then(() => toast.success(t('media.copied')))}
                className="w-full md:w-auto"
              >
                {t('media.copy')}
              </Button>
              <Button variant="destructive-outline" title={t('rel.remove')} icon={<X className="h-4 w-4" />} onClick={() => void remove(current)} className="w-full md:w-auto">
                {t('rel.remove')}
              </Button>
            </>
          )}
        </div>
      </div>
      {multiple && value.length > 1 && (
        <ol className="grid grid-cols-[repeat(auto-fill,minmax(6.5rem,1fr))] gap-2" aria-label={t('media.order')}>
          {value.map((f, i) => (
            <li
              key={f.id}
              draggable
              onDragStart={(e) => {
                if (readOnly()) return e.preventDefault()
                e.dataTransfer.setData(reorderType, String(i))
                e.dataTransfer.effectAllowed = 'move'
                setDragFrom(i)
              }}
              onDragEnd={() => setDragFrom(null)}
              onDragOver={(e) => {
                if (e.dataTransfer.types.includes(reorderType)) e.preventDefault()
              }}
              onDrop={(e) => {
                const from = Number(e.dataTransfer.getData(reorderType))
                if (!e.dataTransfer.types.includes(reorderType)) return
                e.preventDefault()
                e.stopPropagation()
                move(from, i)
              }}
              className={cn('overflow-hidden rounded-md border bg-surface', i === at ? 'border-primary ring-1 ring-primary' : 'border-border', dragFrom === i && 'opacity-50')}
            >
              <button type="button" title={t('media.show', { name: f.name })} onClick={() => setIndex(i)} className="block aspect-square w-full bg-background">
                <Thumb file={f} />
              </button>
              <span className="flex justify-between gap-1 p-1">
                <button type="button" title={t('rel.up')} aria-label={t('rel.up')} disabled={i === 0} onClick={() => move(i, i - 1)} className={`${small} border-border bg-surface`}>
                  <ArrowLeft className="h-3.5 w-3.5" />
                </button>
                <button type="button" title={t('rel.down')} aria-label={t('rel.down')} disabled={i === value.length - 1} onClick={() => move(i, i + 1)} className={`${small} border-border bg-surface`}>
                  <ArrowRight className="h-3.5 w-3.5" />
                </button>
              </span>
            </li>
          ))}
        </ol>
      )}
      {canUpload && <p className="hidden text-xs text-muted md:block">{t('media.fieldDrop')}</p>}
      {error && <p className="text-xs text-destructive">{error}</p>}
      {picking && (
        <MediaPicker
          multiple={multiple}
          allowedTypes={attr.allowedTypes}
          onClose={() => setPicking(false)}
          onChoose={(files) => {
            add(files)
            setPicking(false)
          }}
        />
      )}
      {editing && (
        <FileDialog
          key={editing.id}
          file={editing}
          onClose={() => setEditing(null)}
          onSaved={(f) => onChange(value.map((x) => (x.id === f.id ? { ...x, ...f } : x)))}
          onDeleted={(id) => onChange(value.filter((x) => x.id !== id))}
        />
      )}
    </fieldset>
  )
}
