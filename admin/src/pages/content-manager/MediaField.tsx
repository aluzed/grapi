import { useState } from 'react'
import { ArrowDown, ArrowUp, Check, ImagePlus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import type { Attribute, MediaFile } from '@/lib/types'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { MediaBrowser, Thumb } from '../media/MediaBrowser'

const small =
  'inline-flex min-h-9 items-center gap-1 rounded-md border px-2 text-xs font-medium disabled:cursor-not-allowed disabled:bg-zinc-200 disabled:text-zinc-500'

export function mediaItems(v: unknown): MediaFile[] {
  if (!v) return []
  return (Array.isArray(v) ? v : [v]) as MediaFile[]
}

export function MediaField({ attr, label, value, onChange }: { attr: Attribute & { allowedTypes?: string[] }; label: string; value: MediaFile[]; onChange: (v: MediaFile[]) => void }) {
  const { t } = useI18n()
  const confirm = useConfirm()
  const multiple = Boolean(attr.multiple)
  const [picking, setPicking] = useState(false)
  const [selection, setSelection] = useState<MediaFile[]>([])
  const move = (i: number, d: number) => {
    const next = [...value]
    const [item] = next.splice(i, 1)
    if (item) next.splice(i + d, 0, item)
    onChange(next)
  }
  return (
    <fieldset className="flex flex-col gap-1.5 sm:col-span-2">
      <legend className="mb-1.5 text-sm font-medium">{label}</legend>
      {value.length > 0 && (
        <ul className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {value.map((f, i) => (
            <li key={f.id} className="overflow-hidden rounded-md border border-border bg-surface">
              <span className="block aspect-square bg-background">
                <Thumb file={f} />
              </span>
              <span className="block truncate px-2 py-1 text-xs">{f.name}</span>
              <span className="flex flex-wrap gap-1 p-2 pt-0">
                {multiple && (
                  <>
                    <button type="button" title={t('rel.up')} disabled={i === 0} onClick={() => move(i, -1)} className={`${small} border-border bg-surface`}>
                      <ArrowUp className="h-3.5 w-3.5" />
                    </button>
                    <button type="button" title={t('rel.down')} disabled={i === value.length - 1} onClick={() => move(i, 1)} className={`${small} border-border bg-surface`}>
                      <ArrowDown className="h-3.5 w-3.5" />
                    </button>
                  </>
                )}
                <button type="button" title={t('rel.remove')} onClick={async () => {
                  if (await confirm({ title: t('rel.remove'), message: t('rel.removeConfirm'), confirmLabel: t('rel.remove'), destructive: true })) onChange(value.filter((x) => x.id !== f.id))
                }} className={`${small} border-destructive/40 bg-surface text-destructive`}>
                  <X className="h-3.5 w-3.5" />
                  <span>{t('rel.remove')}</span>
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}
      <div>
        <Button
          variant="outline"
          title={t('media.add')}
          icon={<ImagePlus className="h-4 w-4" />}
          className="w-full md:w-auto"
          onClick={() => {
            setSelection([])
            setPicking(true)
          }}
        >
          {t('media.add')}
        </Button>
      </div>
      {picking && (
        <Dialog
          open
          title={t('media.pick')}
          onClose={() => setPicking(false)}
          closeLabel={t('common.cancel')}
          wide
          footer={
            <>
              <span className="text-sm text-muted md:mr-auto">{t('media.selected', { count: String(selection.length) })}</span>
              <Button variant="outline" title={t('common.cancel')} onClick={() => setPicking(false)} className="w-full md:w-auto">
                {t('common.cancel')}
              </Button>
              <Button
                title={t('media.choose')}
                icon={<Check className="h-4 w-4" />}
                disabled={selection.length === 0}
                className="w-full md:w-auto"
                onClick={() => {
                  const known = new Set(value.map((f) => f.id))
                  onChange(multiple ? [...value, ...selection.filter((f) => !known.has(f.id))] : selection.slice(0, 1))
                  setPicking(false)
                }}
              >
                {t('media.choose')}
              </Button>
            </>
          }
        >
          <MediaBrowser pick={{ multiple, selected: selection, onChange: setSelection, allowedTypes: attr.allowedTypes }} />
        </Dialog>
      )}
    </fieldset>
  )
}
