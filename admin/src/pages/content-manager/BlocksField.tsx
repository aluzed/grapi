import { useState } from 'react'
import { ArrowDown, ArrowUp, Check, ImagePlus, Plus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { SelectField, TextField } from '@/components/ui/field'
import type { Item, Kind } from '@/lib/blocks'
import type { MediaFile } from '@/lib/types'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'
import { MediaBrowser } from '../media/MediaBrowser'

const small =
  'inline-flex min-h-9 items-center gap-1 rounded-md border px-2 text-xs font-medium disabled:cursor-not-allowed disabled:bg-zinc-200 disabled:text-zinc-500'

const kinds: Kind[] = ['paragraph', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'ul', 'ol', 'quote', 'code', 'image']

// imageOf keeps the media fields Strapi stores in an image block.
const imageOf = (f: MediaFile) => ({
  name: f.name,
  alternativeText: f.alternativeText,
  url: f.url,
  caption: f.caption,
  width: f.width,
  height: f.height,
  formats: f.formats,
  ext: f.ext,
  mime: f.mime,
  size: f.size,
})

export function BlocksField({ label, value, onChange }: { label: string; value: Item[]; onChange: (v: Item[]) => void }) {
  const { t } = useI18n()
  const confirm = useConfirm()
  const [picking, setPicking] = useState<number | null>(null)
  const [selection, setSelection] = useState<MediaFile[]>([])
  const kindLabel = (k: Kind) =>
    k.startsWith('h') && k.length === 2 ? t('blocks.h', { n: k.slice(1) }) : t(`blocks.${k as 'paragraph' | 'quote' | 'code' | 'ul' | 'ol' | 'image'}`)
  const set = (i: number, patch: Partial<Item>) => onChange(value.map((it, j) => (j === i ? { ...it, ...patch } : it)))
  const move = (i: number, d: number) => {
    const next = [...value]
    const [item] = next.splice(i, 1)
    if (item) next.splice(i + d, 0, item)
    onChange(next)
  }
  return (
    <fieldset className="flex flex-col gap-2 sm:col-span-2">
      <legend className="mb-1 text-sm font-medium">{label}</legend>
      <p className="text-xs text-muted">{t('blocks.help')}</p>
      {value.length === 0 && <p className="text-xs text-muted">{t('blocks.empty')}</p>}
      {value.map((it, i) => (
        <div key={i} className="rounded-md border border-border bg-background/50 p-3">
          <div className="mb-2 flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
            <SelectField label={t('blocks.kind')} value={it.kind} onChange={(e) => set(i, { kind: e.target.value as Kind })} className="md:w-56">
              {kinds.map((k) => (
                <option key={k} value={k}>
                  {kindLabel(k)}
                </option>
              ))}
            </SelectField>
            <span className="flex gap-1">
              <button type="button" title={t('rel.up')} disabled={i === 0} onClick={() => move(i, -1)} className={`${small} border-border bg-surface`}>
                <ArrowUp className="h-3.5 w-3.5" />
                <span className="hidden sm:inline">{t('rel.up')}</span>
              </button>
              <button type="button" title={t('rel.down')} disabled={i === value.length - 1} onClick={() => move(i, 1)} className={`${small} border-border bg-surface`}>
                <ArrowDown className="h-3.5 w-3.5" />
                <span className="hidden sm:inline">{t('rel.down')}</span>
              </button>
              <button type="button" title={t('comp.remove')} onClick={async () => {
                  if (await confirm({ title: t('comp.remove'), message: t('comp.removeConfirm'), confirmLabel: t('comp.remove'), destructive: true })) onChange(value.filter((_, j) => j !== i))
                }} className={`${small} border-destructive/40 bg-surface text-destructive`}>
                <X className="h-3.5 w-3.5" />
                <span className="hidden sm:inline">{t('comp.remove')}</span>
              </button>
            </span>
          </div>
          {it.kind === 'image' ? (
            <div className="flex flex-col gap-2 md:flex-row md:items-center">
              {typeof it.image?.url === 'string' && <img src={it.image.url} alt={String(it.image.alternativeText ?? '')} className="h-24 w-auto rounded border border-border object-contain" />}
              <Button
                variant="outline"
                title={t('blocks.chooseImage')}
                icon={<ImagePlus className="h-4 w-4" />}
                onClick={() => {
                  setSelection([])
                  setPicking(i)
                }}
                className="w-full md:w-auto"
              >
                {t('blocks.chooseImage')}
              </Button>
            </div>
          ) : (
            <>
              {it.kind === 'code' && <TextField label={t('blocks.language')} value={it.language ?? ''} onChange={(e) => set(i, { language: e.target.value })} className="mb-2 md:w-56" />}
              <textarea
                aria-label={kindLabel(it.kind)}
                rows={it.kind === 'paragraph' || it.kind === 'code' || it.kind === 'ul' || it.kind === 'ol' ? 4 : 2}
                value={it.text}
                onChange={(e) => set(i, { text: e.target.value })}
                className={`block w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-primary focus:outline-none ${it.kind === 'code' ? 'font-mono' : ''} ${it.kind.startsWith('h') && it.kind.length === 2 ? 'font-semibold' : ''}`}
              />
            </>
          )}
        </div>
      ))}
      <div>
        <Button variant="outline" title={t('blocks.add')} icon={<Plus className="h-4 w-4" />} onClick={() => onChange([...value, { kind: 'paragraph', text: '' }])} className="w-full md:w-auto">
          {t('blocks.add')}
        </Button>
      </div>
      {picking !== null && (
        <Dialog
          open
          wide
          title={t('blocks.chooseImage')}
          onClose={() => setPicking(null)}
          closeLabel={t('common.cancel')}
          footer={
            <>
              <Button variant="outline" title={t('common.cancel')} onClick={() => setPicking(null)} className="w-full md:w-auto">
                {t('common.cancel')}
              </Button>
              <Button
                title={t('media.choose')}
                icon={<Check className="h-4 w-4" />}
                disabled={selection.length === 0}
                className="w-full md:w-auto"
                onClick={() => {
                  const f = selection[0]
                  if (f) set(picking, { image: imageOf(f) })
                  setPicking(null)
                }}
              >
                {t('media.choose')}
              </Button>
            </>
          }
        >
          <MediaBrowser pick={{ multiple: false, selected: selection, onChange: setSelection, allowedTypes: ['images'] }} />
        </Dialog>
      )}
    </fieldset>
  )
}
