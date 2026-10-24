// Editing actions of the richtext field. Each one reproduces, on a plain
// text and selection, what Strapi's Markdown editor writes for the same
// action, so that a document edited in either admin stores the same text.

export type State = { text: string; start: number; end: number }

export type InlineAction = 'Bold' | 'Italic' | 'Underline' | 'Strikethrough' | 'Link'
export type BlockAction = 'Code' | 'Quote'
export type ListAction = 'BulletList' | 'NumberList'
export type Heading = 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6'

function wrap(action: InlineAction | BlockAction, s: string): string {
  switch (action) {
    case 'Strikethrough':
      return `~~${s}~~`
    case 'Bold':
      return `**${s}**`
    case 'Italic':
      return `_${s}_`
    case 'Underline':
      return `<u>${s}</u>`
    case 'Code':
      return `\`\`\`\n${s}\n\`\`\``
    case 'Link':
      return `[${s}](link)`
    case 'Quote':
      return `>${s}`
  }
}

const lineStart = (text: string, pos: number) => text.lastIndexOf('\n', pos - 1) + 1

function lineEnd(text: string, pos: number): number {
  const i = text.indexOf('\n', pos)
  return i === -1 ? text.length : i
}

export function inline(action: InlineAction, { text, start, end }: State): State {
  const selected = text.slice(start, end)
  const out = wrap(action, selected || action)
  const next = text.slice(0, start) + out + text.slice(end)
  const after = start + out.length
  // The link target is a placeholder: select it, ready to be typed over.
  if (action === 'Link') return selected ? { text: next, start: after - 5, end: after - 1 } : { text: next, start: start + 1, end: start + 1 + action.length }
  if (selected) return { text: next, start: after, end: after }
  const left = out.indexOf(action)
  return { text: next, start: start + left, end: start + left + action.length }
}

// block puts a code block or a quote on a new line after the current one,
// out of the selected text (removed from its place) or a placeholder.
export function block(action: BlockAction, { text, start, end }: State): State {
  const selected = text.slice(start, end)
  const t = text.slice(0, start) + text.slice(end)
  const le = lineEnd(t, start)
  const rest = le < t.length ? t.slice(le + 1) : null
  const out = wrap(action, selected || action)
  const head = `${t.slice(0, le)}\n`
  const next = head + out + (rest === null ? '' : `\n${rest}`)
  const at = head.length
  if (!selected) {
    const word = action === 'Code' ? 4 : 1
    return { text: next, start: at + word, end: at + word + action.length }
  }
  const caret = action === 'Code' ? at + 4 + selected.length : at + out.length
  return { text: next, start: caret, end: caret }
}

const listItem = /^[*-]\s|^\d+\.\s/

export function list(action: ListAction, { text, start, end }: State): State {
  const prefix = action === 'BulletList' ? '- ' : '1. '
  const first = lineStart(text, start)
  if (start === end) {
    const le = lineEnd(text, start)
    const next = text.slice(0, first) + prefix + text.slice(first)
    return { text: next, start: le + prefix.length, end: le + prefix.length }
  }
  const last = lineEnd(text, end)
  const lines = text.slice(first, last).split('\n')
  const remove = listItem.test(lines[0] ?? '')
  const changed = lines.map((l, i) => {
    if (remove) return listItem.test(l) ? l.slice(prefix.length) : l
    return (action === 'BulletList' ? '- ' : `${i + 1}. `) + l
  })
  const body = changed.join('\n')
  return { text: text.slice(0, first) + body + text.slice(last), start: first, end: first + body.length }
}

export function heading(level: Heading, { text, start }: State): State {
  const ls = lineStart(text, start)
  const le = lineEnd(text, start)
  const line = `${'#'.repeat(Number(level.slice(1)))} ${text.slice(ls, le).replace(/#{1,6}\s/g, '').trim()}`
  return { text: text.slice(0, ls) + line + text.slice(le), start: ls + line.length, end: ls + line.length }
}

export type Asset = { alt: string; url: string; mime: string }

// insertFiles writes one image (or link, for other files) per line at the
// end of the current line, starting a new line unless the caret is at the
// start of it.
export function insertFiles(files: Asset[], { text, end }: State): State {
  const col = end - lineStart(text, end)
  let t = text
  let pos = lineEnd(text, end)
  files.forEach((f, i) => {
    pos = lineEnd(t, pos)
    if (i > 0 || col !== 0) {
      t = `${t.slice(0, pos)}\n${t.slice(pos)}`
      pos += 1
    }
    const md = f.mime.includes('image') ? `![${f.alt}](${f.url})` : `[${f.alt}](${f.url})`
    t = t.slice(0, pos) + md + t.slice(pos)
    pos += md.length
  })
  return { text: t, start: pos, end: pos }
}

const listRE = /^(\s*)(>[> ]*|[*+-] \[[x ]\]\s|[*+-]\s|(\d+)([.)]))(\s*)/
const emptyListRE = /^(\s*)(>[> ]*|[*+-] \[[x ]\]|[*+-]|(\d+)[.)])(\s*)$/
const unorderedListRE = /[*+-]\s/

// enter continues a list or a quote on the next line, ends it on an empty
// item, and keeps the indentation otherwise. Numbered items that follow are
// renumbered.
export function enter({ text, start, end }: State): State {
  const ls = lineStart(text, start)
  const le = lineEnd(text, start)
  const line = text.slice(ls, le)
  const ch = start - ls
  const match = listRE.exec(line)
  if (start !== end || !match || /^\s*$/.test(line.slice(0, ch))) {
    const indent = /^\s*/.exec(line)?.[0] ?? ''
    const ins = `\n${indent}`
    return { text: text.slice(0, start) + ins + text.slice(end), start: start + ins.length, end: start + ins.length }
  }
  if (emptyListRE.test(line)) {
    const t = text.slice(0, ls) + text.slice(Math.min(ls + ch + 1, le))
    const next = `${t.slice(0, ls)}\n${t.slice(ls)}`
    return { text: next, start: ls + 1, end: ls + 1 }
  }
  const indent = match[1] ?? ''
  const after = match[5] ?? ''
  const marker = match[2] ?? ''
  const numbered = !(unorderedListRE.test(marker) || marker.includes('>'))
  const bullet = numbered ? `${Number.parseInt(match[3] ?? '0', 10) + 1}${match[4] ?? ''}` : marker.replace('x', ' ')
  const ins = `\n${indent}${bullet}${after}`
  // As in Strapi, the following items are renumbered from the current one
  // before the new item goes in; renumbering only touches later lines.
  const base = numbered ? renumber(text, lineIndex(text, start)) : text
  const caret = start + ins.length
  return { text: base.slice(0, start) + ins + base.slice(start), start: caret, end: caret }
}

const lineIndex = (text: string, pos: number) => text.slice(0, pos).split('\n').length - 1

function renumber(text: string, startLine: number): string {
  const lines = text.split('\n')
  const startItem = listRE.exec(lines[startLine] ?? '')
  if (!startItem) return text
  const startIndent = startItem[1] ?? ''
  let lookAhead = 0
  let skip = 0
  for (;;) {
    lookAhead += 1
    const n = startLine + lookAhead
    const nextLine = lines[n]
    const item = nextLine === undefined ? null : listRE.exec(nextLine)
    if (!item || nextLine === undefined) break
    const nextIndent = item[1] ?? ''
    const want = Number.parseInt(startItem[3] ?? '', 10) + lookAhead - skip
    const has = Number.parseInt(item[3] ?? '', 10)
    if (startIndent === nextIndent && !Number.isNaN(has)) {
      let num = has
      if (want === has) num = has + 1
      if (want > has) num = want + 1
      lines[n] = nextLine.replace(listRE, `${nextIndent}${num}${item[4] ?? ''}${item[5] ?? ''}`)
    } else {
      if (startIndent.length > nextIndent.length) break
      if (startIndent.length < nextIndent.length && lookAhead === 1) break
      skip += 1
    }
  }
  return lines.join('\n')
}

// Tab indents list items and fenced code only, so that it still leaves the
// field anywhere else.
export function canIndent({ text, start }: State): boolean {
  const ls = lineStart(text, start)
  if (listRE.test(text.slice(ls, lineEnd(text, start)))) return true
  const fences = text.slice(0, ls).match(/^```/gm)
  return (fences?.length ?? 0) % 2 === 1
}

export function indent({ text, start, end }: State, outdent: boolean): State {
  const first = lineStart(text, start)
  const last = lineEnd(text, end)
  const lines = text.slice(first, last).split('\n')
  let delta0 = 0
  const changed = lines.map((l, i) => {
    if (!outdent) {
      if (i === 0) delta0 = 2
      return `  ${l}`
    }
    const cut = /^ {1,2}/.exec(l)?.[0].length ?? 0
    if (i === 0) delta0 = -cut
    return l.slice(cut)
  })
  const body = changed.join('\n')
  const total = body.length - (last - first)
  return { text: text.slice(0, first) + body + text.slice(last), start: Math.max(first, start + delta0), end: end + total }
}
