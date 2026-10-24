import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Bold, Code, Eye, Image, Italic, Link, List, ListOrdered, Maximize2, Minimize2, Pencil, Strikethrough, TextQuote, Underline } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/cn'
import { backendUrl } from '@/lib/media'
import { block, canIndent, enter, heading, indent, inline, insertFiles, list, type Heading, type State } from '@/lib/markdown'
import type { MediaFile } from '@/lib/types'
import { useI18n } from '@/providers/I18nProvider'
import { MediaPicker } from '../media/MediaPicker'

// apply writes the change through the browser's editing commands, which
// keeps the native undo history, and falls back to a plain value change.
function apply(ta: HTMLTextAreaElement, next: State, onChange: (v: string) => void) {
  const prev = ta.value
  let p = 0
  while (p < prev.length && p < next.text.length && prev[p] === next.text[p]) p++
  let s = 0
  while (s < prev.length - p && s < next.text.length - p && prev[prev.length - 1 - s] === next.text[next.text.length - 1 - s]) s++
  const middle = next.text.slice(p, next.text.length - s)
  ta.focus()
  ta.setSelectionRange(p, prev.length - s)
  const done = middle ? document.execCommand('insertText', false, middle) : document.execCommand('delete')
  if (!done || ta.value !== next.text) onChange(next.text)
  requestAnimationFrame(() => ta.setSelectionRange(next.start, next.end))
}

const state = (ta: HTMLTextAreaElement): State => ({ text: ta.value, start: ta.selectionStart, end: ta.selectionEnd })

function ToolButton({ label, icon, onClick, pressed }: { label: string; icon: ReactNode; onClick: () => void; pressed?: boolean }) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      aria-pressed={pressed}
      onMouseDown={(e) => e.preventDefault()}
      onClick={onClick}
      className="inline-flex min-h-9 min-w-9 items-center justify-center gap-1.5 rounded-md border border-border bg-surface px-2 text-sm hover:bg-background disabled:cursor-not-allowed disabled:bg-zinc-200 disabled:text-zinc-500 aria-pressed:border-primary aria-pressed:text-primary"
    >
      {icon}
      <span className="hidden 2xl:inline">{label}</span>
    </button>
  )
}

export function MarkdownField({
  name,
  label,
  value,
  error,
  maxLength,
  onChange,
}: {
  name: string
  label: string
  value: string
  error?: string
  maxLength?: number
  onChange: (v: string) => void
}) {
  const { t } = useI18n()
  const ta = useRef<HTMLTextAreaElement>(null)
  const [preview, setPreview] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [picking, setPicking] = useState(false)
  // The renderer and its plugins load with the first preview, not with the admin.
  const [render, setRender] = useState<((text: string) => string) | null>(null)
  useEffect(() => {
    if ((preview || expanded) && !render) void import('@/lib/markdown-render').then((m) => setRender(() => m.renderMarkdown))
  }, [preview, expanded, render])
  const html = useMemo(() => (render && (preview || expanded) ? render(value) : ''), [render, preview, expanded, value])

  // The editor grows with its text, up to a height where it scrolls.
  useLayoutEffect(() => {
    const el = ta.current
    if (!el || expanded) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(Math.max(el.scrollHeight + 2, 160), Math.round(window.innerHeight * 0.6))}px`
  }, [value, expanded, preview])

  const expandButton = useRef<HTMLButtonElement>(null)
  const wasExpanded = useRef(false)
  useEffect(() => {
    if (expanded) ta.current?.focus()
    else if (wasExpanded.current) expandButton.current?.focus()
    wasExpanded.current = expanded
  }, [expanded])

  useEffect(() => {
    if (!expanded) return
    const onKey = (e: KeyboardEvent) => {
      // Escape in the media picker closes the picker only.
      if (e.key === 'Escape' && !document.querySelector('dialog[open]')) {
        e.preventDefault()
        setExpanded(false)
      }
    }
    document.addEventListener('keydown', onKey)
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = ''
    }
  }, [expanded])

  const run = (op: (s: State) => State) => {
    const act = () => {
      const el = ta.current
      // A read-only field sits in a disabled fieldset; el.disabled misses that.
      if (el && !el.matches(':disabled')) apply(el, op(state(el)), onChange)
    }
    if (!preview) return act()
    // The textarea is not mounted in preview mode: act once it is back.
    setPreview(false)
    requestAnimationFrame(act)
  }

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    const el = e.currentTarget
    if (e.nativeEvent.isComposing) return
    if (e.key === 'Enter' && !e.shiftKey && !e.altKey && !e.metaKey && !e.ctrlKey) {
      e.preventDefault()
      apply(el, enter(state(el)), onChange)
    } else if (e.key === 'Tab' && !e.altKey && !e.metaKey && !e.ctrlKey && canIndent(state(el))) {
      e.preventDefault()
      apply(el, indent(state(el), e.shiftKey), onChange)
    } else if ((e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey) {
      const action = ({ b: 'Bold', i: 'Italic', u: 'Underline' } as const)[e.key.toLowerCase() as 'b' | 'i' | 'u']
      if (action) {
        e.preventDefault()
        apply(el, inline(action, state(el)), onChange)
      }
    }
  }

  const toolbar = (
    <div role="toolbar" aria-label={t('md.toolbar')} className="flex flex-wrap items-center gap-1 border-b border-border bg-background/60 p-1.5">
      <select
        aria-label={t('md.headings')}
        title={t('md.headings')}
        value=""
        onChange={(e) => {
          const h = e.target.value as Heading
          if (h) run((s) => heading(h, s))
        }}
        className="min-h-9 rounded-md border border-border bg-surface px-2 text-sm"
      >
        <option value="">{t('md.headings')}</option>
        {(['h1', 'h2', 'h3', 'h4', 'h5', 'h6'] as const).map((h) => (
          <option key={h} value={h}>
            {t('md.heading', { n: h.slice(1) })}
          </option>
        ))}
      </select>
      <ToolButton label={t('md.bold')} icon={<Bold className="h-4 w-4" />} onClick={() => run((s) => inline('Bold', s))} />
      <ToolButton label={t('md.italic')} icon={<Italic className="h-4 w-4" />} onClick={() => run((s) => inline('Italic', s))} />
      <ToolButton label={t('md.underline')} icon={<Underline className="h-4 w-4" />} onClick={() => run((s) => inline('Underline', s))} />
      <ToolButton label={t('md.strike')} icon={<Strikethrough className="h-4 w-4" />} onClick={() => run((s) => inline('Strikethrough', s))} />
      <ToolButton label={t('md.bullet')} icon={<List className="h-4 w-4" />} onClick={() => run((s) => list('BulletList', s))} />
      <ToolButton label={t('md.numbered')} icon={<ListOrdered className="h-4 w-4" />} onClick={() => run((s) => list('NumberList', s))} />
      <ToolButton label={t('md.code')} icon={<Code className="h-4 w-4" />} onClick={() => run((s) => block('Code', s))} />
      <ToolButton label={t('md.image')} icon={<Image className="h-4 w-4" />} onClick={() => setPicking(true)} />
      <ToolButton label={t('md.link')} icon={<Link className="h-4 w-4" />} onClick={() => run((s) => inline('Link', s))} />
      <ToolButton label={t('md.quote')} icon={<TextQuote className="h-4 w-4" />} onClick={() => run((s) => block('Quote', s))} />
    </div>
  )

  const editor = (
    <textarea
      ref={ta}
      id={`md-${name}`}
      name={name}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      onKeyDown={onKeyDown}
      aria-invalid={error ? true : undefined}
      spellCheck
      className={cn('block w-full resize-y bg-surface px-3 py-2 font-mono text-sm leading-relaxed focus:outline-none', expanded && 'h-full resize-none')}
    />
  )

  return (
    <div className="@container flex min-w-0 flex-col gap-1.5 sm:col-span-2">
      <label htmlFor={`md-${name}`} className="text-sm font-medium">
        {label}
      </label>
      <div className={cn('overflow-hidden rounded-md border bg-surface focus-within:border-primary', error ? 'border-destructive' : 'border-border')}>
        {!expanded && toolbar}
        {!expanded && (preview ? <div className="md-preview min-h-40 px-4 py-3 text-sm" dangerouslySetInnerHTML={{ __html: html || `<p class="text-muted">${t('md.empty')}</p>` }} /> : editor)}
        {expanded && <p className="px-3 py-6 text-center text-sm text-muted">{t('md.editingExpanded')}</p>}
        <div className="flex flex-col gap-2 border-t border-border p-1.5 @md:flex-row @md:items-center @md:justify-between">
          <div className="flex flex-col gap-2 @md:flex-row">
            <Button
              variant="outline"
              title={preview ? t('md.edit') : t('md.preview')}
              icon={preview ? <Pencil className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              disabled={expanded}
              onClick={() => setPreview(!preview)}
              className="w-full @md:w-auto"
            >
              {preview ? t('md.edit') : t('md.preview')}
            </Button>
            <Button ref={expandButton} variant="outline" title={t('md.expand')} icon={<Maximize2 className="h-4 w-4" />} onClick={() => setExpanded(true)} className="w-full @md:w-auto">
              {t('md.expand')}
            </Button>
          </div>
          {maxLength ? (
            <span className={cn('text-center text-xs', value.length > maxLength ? 'text-destructive' : 'text-muted')}>{t('md.count', { n: String(value.length), max: String(maxLength) })}</span>
          ) : null}
        </div>
      </div>
      {error && <p className="text-xs text-destructive">{error}</p>}
      {expanded && (
        <div role="dialog" aria-modal="true" aria-label={label} className="fixed inset-0 z-50 flex flex-col bg-surface">
          <div className="flex flex-col gap-2 border-b border-border px-4 py-3 md:flex-row md:items-center md:justify-between">
            <h2 className="text-base font-semibold">{label}</h2>
            <Button variant="outline" title={t('md.collapse')} icon={<Minimize2 className="h-4 w-4" />} onClick={() => setExpanded(false)} className="w-full md:w-auto">
              {t('md.collapse')}
            </Button>
          </div>
          {toolbar}
          <div className="grid min-h-0 flex-1 grid-cols-1 grid-rows-2 md:grid-cols-2 md:grid-rows-1">
            <div className="min-h-0 border-b border-border md:border-r md:border-b-0">{editor}</div>
            <div className="md-preview min-h-0 overflow-y-auto px-4 py-3 text-sm" aria-label={t('md.preview')} dangerouslySetInnerHTML={{ __html: html }} />
          </div>
        </div>
      )}
      {picking && (
        <MediaPicker
          multiple
          onClose={() => setPicking(false)}
          onChoose={(files: MediaFile[]) => {
            setPicking(false)
            run((s) => insertFiles(files.map((f) => ({ alt: f.alternativeText || f.name, url: backendUrl(f.url), mime: f.mime })), s))
          }}
        />
      )}
    </div>
  )
}
