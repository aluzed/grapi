// Strapi's blocks (the JSON of the "blocks" attribute) and an editable form
// of them: one item per block, inline formatting written as light Markdown.

export type TextNode = { type: 'text'; text: string; bold?: boolean; italic?: boolean; underline?: boolean; strikethrough?: boolean; code?: boolean }
export type LinkNode = { type: 'link'; url: string; children: TextNode[] }
export type Inline = TextNode | LinkNode

export type Block =
  | { type: 'paragraph'; children: Inline[] }
  | { type: 'heading'; level: 1 | 2 | 3 | 4 | 5 | 6; children: Inline[] }
  | { type: 'quote'; children: Inline[] }
  | { type: 'code'; language?: string; children: TextNode[] }
  | { type: 'list'; format: 'ordered' | 'unordered'; children: { type: 'list-item'; children: Inline[] }[] }
  | { type: 'image'; image: Record<string, unknown>; children: TextNode[] }

export type Kind = 'paragraph' | 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6' | 'quote' | 'code' | 'ul' | 'ol' | 'image'

// Item is a block as the editor holds it: text is Markdown for inline
// blocks, one line per entry for lists, raw for code.
export type Item = { kind: Kind; text: string; language?: string; image?: Record<string, unknown> }

type Marks = Omit<TextNode, 'type' | 'text'>

// Characters that open a mark are escaped with a backslash in the text,
// and stand for themselves once parsed.
const special = ['\\', '*', '`', '[', ']', '~', '<']
const placeholder = (c: string) => String.fromCharCode(0xe000 + special.indexOf(c))
const escapeText = (s: string) => s.replace(/[\\*`[\]~<]/g, (c) => `\\${c}`)
const hide = (s: string) => s.replace(/\\([\\*`[\]~<])/g, (_, c: string) => placeholder(c))
const restore = (s: string) => s.replace(/[\ue000-\ue006]/g, (c) => special[c.charCodeAt(0) - 0xe000]!)

const wrap = (raw: string, m: Marks) => {
  if (raw === '') return raw
  if (m.code) return '`' + raw.replace(/`/g, "'") + '`'
  let out = escapeText(raw)
  if (m.underline) out = `<u>${out}</u>`
  if (m.strikethrough) out = `~~${out}~~`
  if (m.italic) out = `*${out}*`
  if (m.bold) out = `**${out}**`
  return out
}

export function inlineToMarkdown(children: Inline[]): string {
  return children
    .map((n) => (n.type === 'link' ? `[${inlineToMarkdown(n.children)}](${n.url})` : wrap(n.text, n)))
    .join('')
}

const patterns: { re: RegExp; mark?: keyof Marks; also?: keyof Marks; link?: true }[] = [
  { re: /`([^`]+)`/, mark: 'code' },
  { re: /\[([^\]]*)\]\(([^)\s]*)\)/, link: true },
  { re: /\*\*\*(.+?)\*\*\*/, mark: 'bold', also: 'italic' },
  { re: /\*\*(.+?)\*\*/, mark: 'bold' },
  { re: /~~(.+?)~~/, mark: 'strikethrough' },
  { re: /<u>(.+?)<\/u>/, mark: 'underline' },
  { re: /\*(.+?)\*/, mark: 'italic' },
]

function parse(s: string, marks: Marks): Inline[] {
  if (s === '') return []
  let best: { index: number; length: number; p: (typeof patterns)[number]; m: RegExpExecArray } | null = null
  for (const p of patterns) {
    const m = p.re.exec(s)
    if (m && (best === null || m.index < best.index)) best = { index: m.index, length: m[0].length, p, m }
  }
  if (!best) return [{ type: 'text', text: s, ...marks }]
  const before: Inline[] = best.index > 0 ? [{ type: 'text', text: s.slice(0, best.index), ...marks }] : []
  const after = parse(s.slice(best.index + best.length), marks)
  let middle: Inline[]
  if (best.p.link) {
    const children = parse(best.m[1] ?? '', marks).filter((n): n is TextNode => n.type === 'text')
    middle = [{ type: 'link', url: best.m[2] ?? '', children: children.length ? children : [{ type: 'text', text: '' }] }]
  } else if (best.p.mark === 'code') {
    middle = [{ type: 'text', text: best.m[1] ?? '', ...marks, code: true }]
  } else {
    middle = parse(best.m[1] ?? '', { ...marks, [best.p.mark!]: true, ...(best.p.also ? { [best.p.also]: true } : {}) })
  }
  return [...before, ...middle, ...after]
}

// markdownToInline never returns an empty list: Strapi expects at least
// one text node per block.
export function markdownToInline(s: string): Inline[] {
  const fix = (n: Inline): Inline => (n.type === 'text' ? { ...n, text: restore(n.text) } : { ...n, url: restore(n.url), children: n.children.map((c) => ({ ...c, text: restore(c.text) })) })
  const out = parse(hide(s), {}).map(fix)
  return out.length ? out : [{ type: 'text', text: '' }]
}

const plain = (children: Inline[]): string => children.map((n) => (n.type === 'text' ? n.text : plain(n.children))).join('')

export function toItems(blocks: unknown): Item[] {
  if (!Array.isArray(blocks)) return []
  return (blocks as Block[]).map((b): Item => {
    switch (b.type) {
      case 'heading':
        return { kind: `h${b.level}` as Kind, text: inlineToMarkdown(b.children) }
      case 'quote':
        return { kind: 'quote', text: inlineToMarkdown(b.children) }
      case 'code':
        return { kind: 'code', text: plain(b.children), language: b.language }
      case 'list':
        return { kind: b.format === 'ordered' ? 'ol' : 'ul', text: b.children.map((li) => inlineToMarkdown(li.children)).join('\n') }
      case 'image':
        return { kind: 'image', text: '', image: b.image }
      default:
        return { kind: 'paragraph', text: inlineToMarkdown(b.children ?? []) }
    }
  })
}

export function fromItems(items: Item[]): Block[] {
  return items.map((it): Block => {
    switch (it.kind) {
      case 'h1':
      case 'h2':
      case 'h3':
      case 'h4':
      case 'h5':
      case 'h6':
        return { type: 'heading', level: Number(it.kind.slice(1)) as 1, children: markdownToInline(it.text) }
      case 'quote':
        return { type: 'quote', children: markdownToInline(it.text) }
      case 'code':
        return { type: 'code', ...(it.language ? { language: it.language } : {}), children: [{ type: 'text', text: it.text }] }
      case 'ul':
      case 'ol':
        return {
          type: 'list',
          format: it.kind === 'ol' ? 'ordered' : 'unordered',
          children: (it.text.split('\n').length ? it.text.split('\n') : ['']).map((line) => ({ type: 'list-item', children: markdownToInline(line) })),
        }
      case 'image':
        return { type: 'image', image: it.image ?? {}, children: [{ type: 'text', text: '' }] }
      default:
        return { type: 'paragraph', children: markdownToInline(it.text) }
    }
  })
}
