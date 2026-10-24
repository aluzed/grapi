import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { Bold, Code, GripVertical, Italic, Link as LinkIcon, List, ListOrdered, Maximize2, Minimize2, Save, Strikethrough, Trash2, Underline } from 'lucide-react'
import { Editor, Element as SlateElement, Node, Path, Transforms, type Descendant } from 'slate'
import { Editable, ReactEditor, Slate, useFocused, useSelected, useSlate, useSlateStatic, withReact, type RenderElementProps, type RenderLeafProps } from 'slate-react'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { SelectField, TextField } from '@/components/ui/field'
import { cn } from '@/lib/cn'
import { backendUrl } from '@/lib/media'
import type { MediaFile } from '@/lib/types'
import { useI18n } from '@/providers/I18nProvider'
import { MediaPicker } from '../media/MediaPicker'
import {
  addLinkFromToolbar,
  checkSnippet,
  createBlocksEditor,
  currentBlockKey,
  editLink,
  emptyParagraph,
  handleBackspace,
  handleEnter,
  handleTab,
  imageOf,
  insertImages,
  isLinkActive,
  isListActive,
  isMarkActive,
  markForKey,
  marksDisabled,
  moveBlock,
  normalizeValue,
  removeLink,
  selectBlock,
  spansBlocks,
  toggleList,
  toggleMark,
  type BlockElement,
  type BlockKey,
  type BlocksEditor,
  type Modifier,
} from './blocks/editor'

const codeLanguages: [string, string][] = [
  ['asm', 'Assembly'], ['bash', 'Bash'], ['c', 'C'], ['clojure', 'Clojure'], ['cobol', 'COBOL'], ['cpp', 'C++'], ['csharp', 'C#'], ['css', 'CSS'], ['dart', 'Dart'], ['dockerfile', 'Dockerfile'],
  ['elixir', 'Elixir'], ['erlang', 'Erlang'], ['fortran', 'Fortran'], ['fsharp', 'F#'], ['go', 'Go'], ['graphql', 'GraphQL'], ['groovy', 'Groovy'], ['haskell', 'Haskell'], ['haxe', 'Haxe'], ['html', 'HTML'],
  ['ini', 'INI'], ['java', 'Java'], ['javascript', 'JavaScript'], ['jsx', 'JavaScript (React)'], ['json', 'JSON'], ['julia', 'Julia'], ['kotlin', 'Kotlin'], ['latex', 'LaTeX'], ['lua', 'Lua'], ['markdown', 'Markdown'],
  ['matlab', 'MATLAB'], ['makefile', 'Makefile'], ['objectivec', 'Objective-C'], ['perl', 'Perl'], ['php', 'PHP'], ['plaintext', 'Plain text'], ['powershell', 'PowerShell'], ['python', 'Python'], ['r', 'R'], ['ruby', 'Ruby'],
  ['rust', 'Rust'], ['sas', 'SAS'], ['scala', 'Scala'], ['scheme', 'Scheme'], ['shell', 'Shell'], ['sql', 'SQL'], ['stata', 'Stata'], ['swift', 'Swift'], ['typescript', 'TypeScript'], ['tsx', 'TypeScript (React)'],
  ['vbnet', 'VB.NET'], ['xml', 'XML'], ['yaml', 'YAML'],
]

const blockKeys: BlockKey[] = ['paragraph', 'heading-one', 'heading-two', 'heading-three', 'heading-four', 'heading-five', 'heading-six', 'list-unordered', 'list-ordered', 'image', 'quote', 'code']

const orderedStyles = ['decimal', 'lower-alpha', 'upper-roman']
const unorderedStyles = ['disc', 'circle', 'square']

function ToolButton({ label, icon, onClick, pressed, disabled }: { label: string; icon: ReactNode; onClick: () => void; pressed?: boolean; disabled?: boolean }) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      aria-pressed={pressed}
      disabled={disabled}
      onMouseDown={(e) => e.preventDefault()}
      onClick={onClick}
      className="inline-flex min-h-9 min-w-9 items-center justify-center gap-1.5 rounded-md border border-border bg-surface px-2 text-sm hover:bg-background disabled:cursor-not-allowed disabled:bg-zinc-200 disabled:text-zinc-500 aria-pressed:border-primary aria-pressed:bg-primary/10 aria-pressed:text-primary"
    >
      {icon}
      <span className="hidden 2xl:inline">{label}</span>
    </button>
  )
}

function Toolbar({ readOnly, onPickImage, expanded, onToggleExpand }: { readOnly: boolean; onPickImage: () => void; expanded: boolean; onToggleExpand: () => void }) {
  const { t } = useI18n()
  const editor = useSlate()
  const noMarks = readOnly || marksDisabled(editor)
  const oneBlock = readOnly || spansBlocks(editor)
  const icons: Record<Modifier, ReactNode> = {
    bold: <Bold className="h-4 w-4" />,
    italic: <Italic className="h-4 w-4" />,
    underline: <Underline className="h-4 w-4" />,
    strikethrough: <Strikethrough className="h-4 w-4" />,
    code: <Code className="h-4 w-4" />,
  }
  return (
    <div role="toolbar" aria-label={t('md.toolbar')} className="flex flex-wrap items-center gap-1 border-b border-border bg-background/60 p-1.5">
      <select
        aria-label={t('blocks.kind')}
        title={t('blocks.kind')}
        disabled={readOnly}
        value={currentBlockKey(editor)}
        onChange={(e) => {
          if (selectBlock(editor, e.target.value as BlockKey) === 'image') onPickImage()
          ReactEditor.focus(editor)
        }}
        className="min-h-9 rounded-md border border-border bg-surface px-2 text-sm"
      >
        {blockKeys.map((k) => (
          <option key={k} value={k}>
            {t(`blocks.key.${k}`)}
          </option>
        ))}
      </select>
      {(Object.keys(icons) as Modifier[]).map((m) => (
        <ToolButton key={m} label={t(`blocks.mark.${m}`)} icon={icons[m]} pressed={isMarkActive(editor, m)} disabled={noMarks} onClick={() => toggleMark(editor, m)} />
      ))}
      <ToolButton label={t('md.link')} icon={<LinkIcon className="h-4 w-4" />} pressed={isLinkActive(editor)} disabled={noMarks || oneBlock} onClick={() => addLinkFromToolbar(editor)} />
      <ToolButton label={t('md.bullet')} icon={<List className="h-4 w-4" />} pressed={isListActive(editor, 'unordered')} disabled={oneBlock} onClick={() => toggleList(editor, 'unordered')} />
      <ToolButton label={t('md.numbered')} icon={<ListOrdered className="h-4 w-4" />} pressed={isListActive(editor, 'ordered')} disabled={oneBlock} onClick={() => toggleList(editor, 'ordered')} />
      <span className="ml-auto">
        <ToolButton label={expanded ? t('md.collapse') : t('md.expand')} icon={expanded ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />} onClick={onToggleExpand} />
      </span>
    </div>
  )
}

function CodeBlock({ attributes, children, element }: RenderElementProps) {
  const { t } = useI18n()
  const editor = useSlateStatic()
  const selected = useSelected()
  const focused = useFocused()
  const [choosing, setChoosing] = useState(false)
  return (
    <div className="relative" {...attributes}>
      <pre className="overflow-auto rounded-md bg-zinc-100 px-4 py-3 font-mono text-sm">
        <code>{children}</code>
      </pre>
      {((selected && focused) || choosing) && (
        <div contentEditable={false} className="mt-1 flex justify-end">
          <select
            aria-label={t('blocks.language')}
            title={t('blocks.language')}
            value={String(element.language || 'plaintext')}
            onFocus={() => setChoosing(true)}
            onBlur={() => setChoosing(false)}
            onChange={(e) => {
              Transforms.setNodes(editor, { language: e.target.value }, { at: ReactEditor.findPath(editor, element) })
              setChoosing(false)
              ReactEditor.focus(editor)
            }}
            className="min-h-9 rounded-md border border-border bg-surface px-2 text-sm"
          >
            {codeLanguages.map(([v, l]) => (
              <option key={v} value={v}>
                {l}
              </option>
            ))}
          </select>
        </div>
      )}
    </div>
  )
}

function ImageBlock({ attributes, children, element }: RenderElementProps) {
  const selected = useSelected()
  const focused = useFocused()
  const image = (element.image ?? {}) as { url?: string; alternativeText?: string; width?: number; height?: number }
  return (
    <div {...attributes}>
      {children}
      <div contentEditable={false} className={cn('flex justify-center rounded-md bg-zinc-100 transition-shadow', selected && focused && 'ring-3 ring-primary')}>
        <img src={image.url} alt={image.alternativeText ?? ''} width={image.width} height={image.height} className="h-auto max-h-[456px] max-w-full object-contain" />
      </div>
    </div>
  )
}

function renderBlock(props: RenderElementProps, onOpenLink: (p: Path) => void, editor: BlocksEditor) {
  const { attributes, children, element } = props
  switch (element.type) {
    case 'heading': {
      const size = ['text-3xl', 'text-2xl', 'text-xl', 'text-lg', 'text-base', 'text-sm'][Number(element.level) - 1] ?? 'text-base'
      const Tag = `h${Number(element.level) || 1}` as 'h1'
      return (
        <Tag {...attributes} className={cn(size, 'font-semibold leading-snug')}>
          {children}
        </Tag>
      )
    }
    case 'quote':
      return (
        <blockquote {...attributes} className="border-l-4 border-border px-4 py-2 text-muted">
          {children}
        </blockquote>
      )
    case 'code':
      return <CodeBlock {...props} />
    case 'image':
      return <ImageBlock {...props} />
    case 'list': {
      const styles = element.format === 'ordered' ? orderedStyles : unorderedStyles
      const style = styles[Number(element.indentLevel ?? 0) % styles.length]
      const Tag = element.format === 'ordered' ? 'ol' : 'ul'
      return (
        <Tag {...attributes} style={{ listStyleType: style }} className="flex flex-col gap-2 ps-2 [&_li]:ms-3">
          {children}
        </Tag>
      )
    }
    case 'list-item':
      return <li {...attributes}>{children}</li>
    case 'link':
      return (
        <a
          {...attributes}
          href={String(element.url ?? '')}
          onClick={(e) => {
            e.preventDefault()
            onOpenLink(ReactEditor.findPath(editor, element))
          }}
          className="cursor-pointer text-primary underline"
        >
          {children}
        </a>
      )
  }
  return <p {...attributes}>{children}</p>
}

function Leaf({ attributes, children, leaf }: RenderLeafProps) {
  let out = children
  if (leaf.bold) out = <strong>{out}</strong>
  if (leaf.italic) out = <em>{out}</em>
  if (leaf.underline) out = <u>{out}</u>
  if (leaf.strikethrough) out = <s>{out}</s>
  if (leaf.code) out = <code className="rounded bg-zinc-100 px-1.5 font-mono text-[0.9em]">{out}</code>
  return <span {...attributes}>{out}</span>
}

function LinkDialog({ editor, path, onClose }: { editor: BlocksEditor; path: Path; onClose: () => void }) {
  const { t } = useI18n()
  const node = Editor.node(editor, path)[0] as BlockElement
  const [text, setText] = useState(Editor.string(editor, path))
  const [url, setUrl] = useState(String(node.url ?? ''))
  const [rel, setRel] = useState(String(node.rel ?? ''))
  const [target, setTarget] = useState(String(node.target ?? ''))
  // showModal focuses the close button: move the focus to the URL once open.
  const urlInput = useRef<HTMLInputElement>(null)
  useEffect(() => {
    const id = requestAnimationFrame(() => urlInput.current?.focus())
    return () => cancelAnimationFrame(id)
  }, [])
  let valid = true
  try {
    new URL(url.startsWith('/') ? `https://example.com${url}` : url)
  } catch {
    valid = false
  }
  const close = () => {
    // A link left without a URL is not kept, as in Strapi.
    if (!node.url) removeLink(editor, path)
    onClose()
  }
  return (
    <Dialog
      open
      title={t('blocks.editLink')}
      onClose={close}
      closeLabel={t('common.cancel')}
      footer={
        <>
          <Button variant="destructive-outline" title={t('blocks.removeLink')} icon={<Trash2 className="h-4 w-4" />} onClick={() => (removeLink(editor, path), onClose())} className="w-full md:w-auto">
            {t('blocks.removeLink')}
          </Button>
          <Button variant="outline" title={t('common.cancel')} onClick={close} className="w-full md:w-auto">
            {t('common.cancel')}
          </Button>
          <Button title={t('common.save')} icon={<Save className="h-4 w-4" />} disabled={!url || !valid} onClick={() => {
              editLink(editor, path, { url, text, rel, target })
              // Go on typing after the link, not inside it.
              const end = Editor.end(editor, path)
              Transforms.select(editor, Editor.after(editor, end) ?? end)
              onClose()
            }} className="w-full md:w-auto">
            {t('common.save')}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <TextField label={t('blocks.linkText')} value={text} onChange={(e) => setText(e.target.value)} />
        <TextField ref={urlInput} label={t('blocks.linkUrl')} value={url} placeholder="https://" error={url && !valid ? t('blocks.linkInvalid') : undefined} onChange={(e) => setUrl(e.target.value)} />
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <TextField label={t('blocks.linkRel')} value={rel} placeholder="noopener noreferrer" onChange={(e) => setRel(e.target.value)} />
          <SelectField label={t('blocks.linkTarget')} value={target} onChange={(e) => setTarget(e.target.value)}>
            <option value="">{t('blocks.linkTargetSame')}</option>
            <option value="_blank">{t('blocks.linkTargetBlank')}</option>
          </SelectField>
        </div>
      </div>
    </Dialog>
  )
}

// DragHandle moves a top level block by drag and drop; the keyboard does
// the same with Ctrl/Cmd+Shift+Up/Down.
function DragHandle({ index, type, onDrop }: { index: number; type: string; onDrop: (from: number, to: number) => void }) {
  const { t } = useI18n()
  return (
    <span
      contentEditable={false}
      draggable
      title={t('blocks.drag')}
      aria-hidden
      onDragStart={(e) => {
        e.dataTransfer.setData(type, String(index))
        e.dataTransfer.effectAllowed = 'move'
      }}
      onDragOver={(e) => e.dataTransfer.types.includes(type) && e.preventDefault()}
      onDrop={(e) => {
        e.preventDefault()
        onDrop(Number(e.dataTransfer.getData(type)), index)
      }}
      className="absolute top-1 -left-7 hidden cursor-grab rounded p-0.5 text-muted select-none group-hover/block:block hover:bg-background md:block md:opacity-0 md:group-hover/block:opacity-100"
    >
      <GripVertical className="h-4 w-4" />
    </span>
  )
}

export function BlocksField({ name, label, value, error, onChange }: { name: string; label: string; value: Descendant[] | null; error?: string; onChange: (v: Descendant[] | null) => void }) {
  const { t } = useI18n()
  const [editor] = useState(() => createBlocksEditor((e) => withReact(e) as BlocksEditor))
  const root = useRef<HTMLDivElement>(null)
  // Each editor has its own drag type: a block dropped from another one does nothing.
  const dragType = `application/x-grapi-block-${useId().replace(/[^a-z0-9]/gi, '')}`
  const [readOnly, setReadOnly] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [picking, setPicking] = useState(false)
  const [linkAt, setLinkAt] = useState<Path | null>(null)
  const [live, setLive] = useState('')

  // A field made read only sits in a disabled fieldset.
  useEffect(() => setReadOnly(Boolean(root.current?.matches(':disabled') || root.current?.closest('fieldset:disabled'))), [])

  // Slate is not controlled: a value set from outside (a restored version,
  // a locale filled in) remounts it. Changes made in the editor do not.
  const fromEditor = useRef(0)
  const seen = useRef(0)
  const [key, setKey] = useState(0)
  useEffect(() => {
    seen.current += 1
    if (seen.current !== fromEditor.current) {
      setKey((k) => k + 1)
      fromEditor.current = seen.current
    }
  }, [value])

  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const flush = useCallback(() => {
    if (!timer.current) return
    clearTimeout(timer.current)
    timer.current = null
    fromEditor.current += 1
    onChange(normalizeValue(editor, editor.children))
  }, [editor, onChange])
  useEffect(() => () => flush(), [flush])

  const handleChange = () => {
    if (!editor.operations.some((op) => op.type !== 'set_selection')) return
    if (editor.lastInsertedLinkPath) {
      setLinkAt(editor.lastInsertedLinkPath)
      editor.lastInsertedLinkPath = null
    }
    // Writing the whole document to the form on each key is slow: wait for a pause.
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      timer.current = null
      fromEditor.current += 1
      onChange(normalizeValue(editor, editor.children))
    }, 300)
  }

  const moveTo = (from: number, to: number) => {
    if (from === to || from < 0 || to < 0) return
    Transforms.moveNodes(editor, { at: [from], to: [to] })
    setLive(t('blocks.moved', { from: String(from + 1), to: String(to + 1), count: String(editor.children.length) }))
  }

  const renderElement = useCallback(
    (props: RenderElementProps) => {
      const el = renderBlock(props, setLinkAt, editor)
      const path = ReactEditor.findPath(editor, props.element)
      if (path.length !== 1 || readOnly) return el
      return (
        <div className="group/block relative">
          <DragHandle index={path[0]!} type={dragType} onDrop={moveTo} />
          {el}
        </div>
      )
    },
    // moveTo and setLinkAt only use refs and stable setters.
    [editor, readOnly, dragType],
  )

  const onKeyDown = (e: React.KeyboardEvent) => {
    switch (e.key) {
      case 'Enter':
        e.preventDefault()
        return handleEnter(editor, e.shiftKey)
      case 'Backspace':
        return handleBackspace(editor, e)
      case 'Tab':
        return handleTab(editor, e)
      case 'Escape':
        if (expanded) return setExpanded(false)
        return ReactEditor.blur(editor)
    }
    if (e.metaKey || e.ctrlKey) {
      const mark = markForKey(e)
      if (mark) {
        e.preventDefault()
        if (!marksDisabled(editor)) toggleMark(editor, mark)
        return
      }
      if (e.shiftKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
        const from = editor.selection?.anchor.path[0] ?? 0
        if (moveBlock(editor, e.key === 'ArrowUp' ? -1 : 1)) {
          e.preventDefault()
          setLive(t('blocks.moved', { from: String(from + 1), to: String(editor.selection!.anchor.path[0]! + 1), count: String(editor.children.length) }))
        }
      }
      return
    }
    if (e.key === ' ' && checkSnippet(editor, e) === 'image') setPicking(true)
  }

  return (
    <div ref={root} className="flex min-w-0 flex-col gap-1.5 sm:col-span-2">
      <span id={`blocks-${name}-label`} className="text-sm font-medium">
        {label}
      </span>
      <p id={`blocks-${name}-help`} className="sr-only">
        {t('blocks.dndHelp')}
      </p>
      <p aria-live="assertive" className="sr-only">
        {live}
      </p>
      <Slate key={key} editor={editor} initialValue={value?.length ? value : [emptyParagraph()]} onChange={handleChange}>
        <div className={cn('flex flex-col overflow-hidden border bg-surface', expanded ? 'fixed inset-0 z-50' : 'rounded-md', error ? 'border-destructive' : 'border-border focus-within:border-primary')}>
          {expanded && (
            <div className="border-b border-border px-4 py-3">
              <h2 className="text-base font-semibold">{label}</h2>
            </div>
          )}
          <Toolbar readOnly={readOnly} onPickImage={() => setPicking(true)} expanded={expanded} onToggleExpand={() => setExpanded(!expanded)} />
          <div className={cn('overflow-auto py-4 pr-4 pl-4 md:pl-10', expanded ? 'flex-1' : 'max-h-[32rem] min-h-40')}>
            <Editable
              aria-labelledby={`blocks-${name}-label`}
              aria-describedby={`blocks-${name}-help`}
              readOnly={readOnly}
              renderElement={renderElement}
              renderLeaf={(p) => <Leaf {...p} />}
              onKeyDown={onKeyDown}
              onBlur={flush}
              onDrop={() => true}
              onDragStart={() => true}
              placeholder={t('blocks.placeholder')}
              className="flex min-h-32 flex-col gap-3 text-sm leading-relaxed focus:outline-none"
            />
          </div>
        </div>
        {picking && (
          <MediaPicker
            multiple
            allowedTypes={['images']}
            onClose={() => setPicking(false)}
            onChoose={(files: MediaFile[]) => {
              setPicking(false)
              insertImages(
                editor,
                files.map((f) => imageOf({ ...f, url: backendUrl(f.url) } as unknown as Record<string, unknown>)),
              )
              ReactEditor.focus(editor)
            }}
          />
        )}
        {linkAt && Node.has(editor, linkAt) && SlateElement.isElement(Node.get(editor, linkAt)) && (
          <LinkDialog
            key={linkAt.join('.')}
            editor={editor}
            path={linkAt}
            onClose={() => {
              setLinkAt(null)
              ReactEditor.focus(editor)
            }}
          />
        )}
      </Slate>
      {error && <p className="text-xs text-destructive">{error}</p>}
    </div>
  )
}
