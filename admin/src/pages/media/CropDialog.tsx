import { useRef, useState, type PointerEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Copy, Crop } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { upload } from '@/lib/api'
import { showError } from '@/lib/form'
import { cropRect, type Box } from '@/lib/crop'
import type { MediaFile } from '@/lib/types'
import { useI18n } from '@/providers/I18nProvider'

// The browser crops, as in Strapi: the cropped image replaces the file or
// is uploaded as a copy, and the server regenerates the formats.
async function cropped(file: MediaFile, img: HTMLImageElement, box: Box): Promise<File> {
  const r = cropRect(box, img.naturalWidth, img.naturalHeight)
  const canvas = document.createElement('canvas')
  canvas.width = r.width
  canvas.height = r.height
  canvas.getContext('2d')!.drawImage(img, r.x, r.y, r.width, r.height, 0, 0, r.width, r.height)
  const type = ['image/jpeg', 'image/png', 'image/webp'].includes(file.mime) ? file.mime : 'image/png'
  const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, type, 0.92))
  if (!blob) throw new Error('crop')
  return new File([blob], file.name, { type })
}

export function CropDialog({ file, folder, onClose, onDone }: { file: MediaFile; folder: number | null; onClose: () => void; onDone: () => void }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const img = useRef<HTMLImageElement>(null)
  const [box, setBox] = useState<Box | null>(null)
  const start = useRef<{ x: number; y: number } | null>(null)
  const point = (e: PointerEvent) => {
    const r = e.currentTarget.getBoundingClientRect()
    return { x: Math.min(Math.max((e.clientX - r.left) / r.width, 0), 1), y: Math.min(Math.max((e.clientY - r.top) / r.height, 0), 1) }
  }
  const send = useMutation({
    mutationFn: async (copy: boolean) => {
      const out = await cropped(file, img.current!, box!)
      const form = new FormData()
      form.append('files', out)
      return upload(copy ? `/upload?folder=${folder ?? 'root'}` : `/upload?id=${file.id}`, form)
    },
    onSuccess: (_, copy) => {
      toast.success(copy ? t('media.croppedCopy') : t('media.cropped'))
      void queryClient.invalidateQueries({ queryKey: ['media'] })
      onDone()
    },
    onError: (e) => showError(e, undefined, t('media.cropFailed')),
  })
  const ready = box !== null && box.w > 0.01 && box.h > 0.01
  return (
    <Dialog
      open
      wide
      title={t('media.crop')}
      onClose={onClose}
      closeLabel={t('common.cancel')}
      footer={
        <>
          <Button variant="outline" title={t('common.cancel')} onClick={onClose} className="w-full md:w-auto">
            {t('common.cancel')}
          </Button>
          <Button variant="outline" title={t('media.cropCopy')} icon={<Copy className="h-4 w-4" />} disabled={!ready} loading={send.isPending && send.variables} onClick={() => send.mutate(true)} className="w-full md:w-auto">
            {t('media.cropCopy')}
          </Button>
          <Button title={t('media.cropReplace')} icon={<Crop className="h-4 w-4" />} disabled={!ready} loading={send.isPending && !send.variables} onClick={() => send.mutate(false)} className="w-full md:w-auto">
            {t('media.cropReplace')}
          </Button>
        </>
      }
    >
      <p className="mb-3 text-sm text-muted">{t('media.cropHelp')}</p>
      <div
        className="relative mx-auto w-fit touch-none select-none"
        onPointerDown={(e) => {
          e.currentTarget.setPointerCapture(e.pointerId)
          start.current = point(e)
          setBox({ ...start.current, w: 0, h: 0 })
        }}
        onPointerMove={(e) => {
          if (!start.current) return
          const p = point(e)
          setBox({ x: Math.min(start.current.x, p.x), y: Math.min(start.current.y, p.y), w: Math.abs(p.x - start.current.x), h: Math.abs(p.y - start.current.y) })
        }}
        onPointerUp={() => {
          start.current = null
        }}
      >
        <img ref={img} src={file.url} crossOrigin="anonymous" alt={file.alternativeText ?? file.name} draggable={false} className="block max-h-[60vh] max-w-full" />
        {box && (
          <div
            className="pointer-events-none absolute border-2 border-white shadow-[0_0_0_9999px_rgba(0,0,0,0.45)]"
            style={{ left: `${box.x * 100}%`, top: `${box.y * 100}%`, width: `${box.w * 100}%`, height: `${box.h * 100}%` }}
          />
        )}
      </div>
    </Dialog>
  )
}
