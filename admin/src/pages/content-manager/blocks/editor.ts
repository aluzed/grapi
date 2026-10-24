// The behaviour of Strapi's blocks editor on a Slate editor: the same
// conversions, keys and plugins, so that an action gives the JSON Strapi
// would store. Kept free of React so that it runs in unit tests.
import { createEditor, Editor, Element, Node, Path, Point, Range, Text, Transforms, type BaseEditor, type Descendant, type NodeEntry } from 'slate'
import { withHistory, type HistoryEditor } from 'slate-history'
import type { ReactEditor } from 'slate-react'

export type Marks = { bold?: boolean; italic?: boolean; underline?: boolean; strikethrough?: boolean; code?: boolean }
export type TextNode = { type: 'text'; text: string } & Marks
export type BlockElement = { type: string; children: Descendant[]; [key: string]: unknown }

export type BlocksEditor = BaseEditor & ReactEditor & HistoryEditor & { lastInsertedLinkPath: Path | null; shouldSaveLinkPath: boolean }

declare module 'slate' {
  interface CustomTypes {
    Editor: BlocksEditor
    Element: BlockElement
    Text: TextNode
  }
}

export const emptyParagraph = (): BlockElement => ({ type: 'paragraph', children: [{ type: 'text', text: '' }] })

const isText = (node: unknown): node is TextNode => Node.isNode(node) && !Editor.isEditor(node) && (node as TextNode).type === 'text'
const isElementOf = (type: string) => (node: Node) => !Editor.isEditor(node) && Element.isElement(node) && node.type === type
const isList = isElementOf('list')
const isBlock = (node: Node) => !Editor.isEditor(node) && Element.isElement(node) && node.type !== 'text' && node.type !== 'link'

/* Plugins ------------------------------------------------------------------ */

// Slate creates text nodes without a type; Strapi's schema wants one.
function withStrapiSchema(editor: BlocksEditor): BlocksEditor {
  const { normalizeNode } = editor
  editor.normalizeNode = (entry) => {
    const [node, path] = entry
    if (!Element.isElement(node) && !isText(node)) {
      Transforms.setNodes(editor, { type: 'text' } as Partial<Node>, { at: path })
      return
    }
    normalizeNode(entry)
  }
  return editor
}

function withLinks(editor: BlocksEditor): BlocksEditor {
  const { isInline, apply, insertText, insertData } = editor
  editor.isInline = (element) => element.type === 'link' || isInline(element)
  editor.lastInsertedLinkPath = null
  editor.shouldSaveLinkPath = false
  // The path of the last link inserted from the toolbar opens its editor.
  editor.apply = (op) => {
    if (op.type === 'insert_node') {
      if (!Editor.isEditor(op.node) && Element.isElement(op.node) && op.node.type === 'link' && editor.shouldSaveLinkPath) editor.lastInsertedLinkPath = op.path
    } else if (op.type === 'move_node') {
      if (Path.hasPrevious(op.path) && editor.lastInsertedLinkPath && editor.shouldSaveLinkPath) editor.lastInsertedLinkPath = Path.transform(editor.lastInsertedLinkPath, op)
    }
    apply(op)
  }
  // A space typed at the end of a link goes after it.
  editor.insertText = (text) => {
    if (editor.selection && Range.isCollapsed(editor.selection) && text === ' ') {
      const links = Array.from(Editor.nodes(editor, { at: editor.selection, match: isElementOf('link') }))
      const first = links[0]
      if (first && Point.equals(editor.selection.anchor, Editor.end(editor, first[1]))) {
        Transforms.insertNodes(editor, { text: ' ', type: 'text' }, { at: Path.next(first[1]), select: true })
        return
      }
    }
    insertText(text)
  }
  editor.insertData = (data) => {
    const pasted = data.getData('text/plain')
    if (pasted) {
      try {
        new URL(pasted)
        editor.shouldSaveLinkPath = false
        insertLink(editor, { url: pasted })
        return
      } catch {
        // Not a URL: paste as text.
      }
    }
    insertData(data)
  }
  return editor
}

function withImages(editor: BlocksEditor): BlocksEditor {
  const { isVoid } = editor
  editor.isVoid = (element) => element.type === 'image' || isVoid(element)
  return editor
}

// Pasting in a code block keeps the text as is, line breaks included.
function withCode(editor: BlocksEditor): BlocksEditor {
  const { insertData } = editor
  editor.insertData = (data) => {
    const pasted = data.getData('text/plain')
    if (pasted && editor.selection && Editor.above(editor, { match: isElementOf('code') })) {
      Transforms.insertText(editor, pasted)
      return
    }
    insertData(data)
  }
  return editor
}

// createBlocksEditor builds the editor without React; withReact is applied
// on top by the field, as Strapi does (history, schema, react, then blocks).
export function createBlocksEditor(withReact: (e: BlocksEditor) => BlocksEditor = (e) => e): BlocksEditor {
  let e = createEditor() as BlocksEditor
  e = withHistory(e) as BlocksEditor
  e = withStrapiSchema(e)
  e = withReact(e)
  for (const plugin of [withLinks, withImages, withCode]) e = plugin(e)
  return e
}

/* Conversions -------------------------------------------------------------- */

const attributesToClear = (element: BlockElement) => {
  const { children: _c, type: _t, ...extra } = element
  return Object.fromEntries(Object.keys(extra).map((k) => [k, null]))
}

function baseHandleConvert(editor: BlocksEditor, attributes: Record<string, unknown>): Path | undefined {
  const [, lastNodePath] = Editor.last(editor, [])
  // A block converted inside a list leaves the list, which is split around it.
  Transforms.unwrapNodes(editor, { match: isList, split: true, at: editor.selection ?? lastNodePath })
  const [, updatedLastNodePath] = Editor.last(editor, [])
  const entry = Editor.above(editor, { match: isBlock, at: editor.selection ?? updatedLastNodePath })
  if (!entry || Editor.isEditor(entry[0])) return
  const [element, path] = entry as NodeEntry<BlockElement>
  Transforms.setNodes(editor, { ...attributesToClear(element), ...attributes } as Partial<Node>, { at: path })
  return path
}

function convertToList(editor: BlocksEditor, format: 'ordered' | 'unordered') {
  const path = baseHandleConvert(editor, { type: 'list-item' })
  if (!path) return
  Transforms.wrapNodes(editor, { type: 'list', format, children: [] }, { at: path })
}

/* Keys --------------------------------------------------------------------- */

const modifierNames = ['bold', 'italic', 'underline', 'strikethrough', 'code'] as const

// The first Enter adds a line break, the second one on an empty last line
// leaves the block for a new paragraph.
function pressEnterTwiceToExit(editor: BlocksEditor) {
  const entry = Editor.above(editor, { match: (n) => !Editor.isEditor(n) && Element.isElement(n) && !['link', 'text'].includes(n.type) })
  if (!entry || !editor.selection) return
  const [node, path] = entry as NodeEntry<BlockElement>
  const isNodeEnd = Editor.isEnd(editor, editor.selection.anchor, path)
  const last = node.children.at(-1)
  if (isNodeEnd && isText(last) && last.text.endsWith('\n')) {
    Transforms.delete(editor, { distance: 1, unit: 'character', reverse: true })
    Transforms.insertNodes(editor, emptyParagraph())
    return
  }
  Transforms.insertText(editor, '\n')
  if (isNodeEnd) for (const m of modifierNames) Editor.removeMark(editor, m)
}

function paragraphEnter(editor: BlocksEditor) {
  if (!editor.selection) return
  const start = editor.selection.anchor.path
  Transforms.splitNodes(editor, { always: true })
  const parent = Editor.above(editor, { match: (n) => !Editor.isEditor(n) && Element.isElement(n) && n.type !== 'text' })
  if (!parent) return
  const isNodeEnd = Editor.isEnd(editor, editor.selection.anchor, parent[1])
  // The new block is recreated as a paragraph, so that no attribute of the
  // split block (a heading's level) carries over; its marks do, if it has text.
  const [fragment] = Editor.parent(editor, editor.selection.anchor.path) as NodeEntry<BlockElement>
  Transforms.removeNodes(editor)
  const top = start[0]!
  const hasNext = editor.children.length - top > 1
  Transforms.insertNodes(editor, { type: 'paragraph', children: isNodeEnd ? [{ type: 'text', text: '' }] : fragment.children }, { at: hasNext ? [top + 1] : [editor.children.length] })
  Transforms.select(editor, Editor.start(editor, [top + 1]))
}

function replaceListWithEmptyBlock(editor: BlocksEditor, listPath: Path) {
  Transforms.removeNodes(editor, { at: listPath })
  if (listPath[0] === 0) {
    Transforms.insertNodes(editor, emptyParagraph(), { at: listPath })
    Transforms.select(editor, listPath)
  }
}

const isEmptyItem = (item: BlockElement) => item.children.length === 1 && isText(item.children[0]) && item.children[0].text === ''

function listEnter(editor: BlocksEditor) {
  const itemEntry = Editor.above(editor, { match: isElementOf('list-item') })
  if (!itemEntry || !editor.selection) return
  const [item, itemPath] = itemEntry as NodeEntry<BlockElement>
  const [list, listPath] = Editor.parent(editor, itemPath) as NodeEntry<BlockElement>
  const listEmpty = list.children.length === 1 && isEmptyItem(item)
  const itemEmpty = isEmptyItem(item)
  const atChildStart = editor.selection.focus.offset === 0 && editor.selection.focus.path.at(-1) === 0
  if (listEmpty) {
    replaceListWithEmptyBlock(editor, listPath)
  } else if (atChildStart && !itemEmpty) {
    const current = Editor.above(editor, { at: editor.selection.anchor })
    Transforms.insertNodes(editor, { type: 'list-item', children: [{ type: 'text', text: '' }] })
    if (current) {
      const p = current[1]
      const next = [...p.slice(0, -1), p[p.length - 1]! + 1]
      Transforms.select(editor, { anchor: { path: next.concat(0), offset: 0 }, focus: { path: next.concat(0), offset: 0 } })
    }
  } else if (itemEmpty) {
    const level = Number(list.indentLevel ?? 0)
    if (level > 0) {
      const parentList = Editor.above(editor, { match: (n) => isList(n) && Number((n as BlockElement).indentLevel ?? 0) === level - 1 })
      if (parentList) {
        const to = itemPath.slice(0, -1)
        if (to.length > 0) to[to.length - 1]! += 1
        Transforms.moveNodes(editor, { at: itemPath, to })
        return
      }
    }
    Transforms.removeNodes(editor, { at: itemPath })
    const created = Path.next(listPath)
    Transforms.insertNodes(editor, emptyParagraph(), { at: created })
    Transforms.select(editor, created)
  } else if (Editor.isEnd(editor, editor.selection.anchor, itemPath)) {
    Transforms.insertNodes(editor, { type: 'list-item', children: [{ type: 'text', text: '' }] })
  } else {
    Transforms.splitNodes(editor)
  }
}

function listBackspace(editor: BlocksEditor, event: { preventDefault(): void }) {
  if (!editor.selection) return
  const [item, itemPath] = Editor.parent(editor, editor.selection.anchor) as NodeEntry<BlockElement>
  const [list, listPath] = Editor.parent(editor, itemPath) as NodeEntry<BlockElement>
  const listEmpty = list.children.length === 1 && isText(item.children[0]) && item.children[0].text === ''
  const itemEmpty = isEmptyItem(item)
  const atChildStart = editor.selection.focus.offset === 0 && editor.selection.focus.path.at(-2) === 0
  if (listEmpty) {
    if (!Editor.above(editor, { at: listPath, match: isList })) {
      event.preventDefault()
      replaceListWithEmptyBlock(editor, listPath)
    }
  } else if (atChildStart) {
    Transforms.liftNodes(editor, { match: isElementOf('list-item') })
    Transforms.setNodes(editor, { type: 'paragraph' })
  } else if (itemEmpty) {
    const prev = Editor.previous(editor, { at: itemPath })
    const next = Editor.next(editor, { at: itemPath })
    if (prev && next) {
      event.preventDefault()
      Transforms.removeNodes(editor, { at: itemPath })
      const [p] = prev
      const [n] = next
      if (isList(p) && isList(n) && (p as BlockElement).format === (n as BlockElement).format && (p as BlockElement).indentLevel === (n as BlockElement).indentLevel) {
        Transforms.mergeNodes(editor, { at: itemPath })
      }
    }
  }
}

// Tab nests a list item one level deeper, under the previous item.
function listTab(editor: BlocksEditor) {
  const itemEntry = Editor.above(editor, { match: isElementOf('list-item') })
  if (!itemEntry || !editor.selection) return
  const [item, itemPath] = itemEntry as NodeEntry<BlockElement>
  const [list] = Editor.parent(editor, itemPath) as NodeEntry<BlockElement>
  if (item === list.children[0]) return
  const index = list.children.findIndex((c) => c === item)
  const previous = list.children[index - 1] as BlockElement | undefined
  if (previous?.type === 'list') {
    Transforms.moveNodes(editor, { at: itemPath, to: Path.previous(itemPath).concat(previous.children.length) })
    return
  }
  if (isList(list)) Transforms.wrapNodes(editor, { type: 'list', format: list.format, indentLevel: Number(list.indentLevel ?? 0) + 1, children: [] })
}

function imageBackspace(editor: BlocksEditor) {
  if (editor.children.length === 1) Transforms.setNodes(editor, { type: 'paragraph', image: null, children: [{ type: 'text', text: '' }] } as Partial<Node>)
  else Transforms.removeNodes(editor)
}

/* Blocks ------------------------------------------------------------------- */

export type BlockKey =
  | 'paragraph'
  | 'heading-one'
  | 'heading-two'
  | 'heading-three'
  | 'heading-four'
  | 'heading-five'
  | 'heading-six'
  | 'list-ordered'
  | 'list-unordered'
  | 'image'
  | 'quote'
  | 'code'

type BlockDef = {
  match: (n: BlockElement) => boolean
  convert?: (editor: BlocksEditor) => void
  enter?: (editor: BlocksEditor) => void
  backspace?: (editor: BlocksEditor, event: { preventDefault(): void }) => void
  tab?: (editor: BlocksEditor) => void
  snippets?: string[]
}

const headingKeys = ['heading-one', 'heading-two', 'heading-three', 'heading-four', 'heading-five', 'heading-six'] as const

export const blocks: Record<BlockKey, BlockDef> = {
  paragraph: { match: (n) => n.type === 'paragraph', convert: (e) => void baseHandleConvert(e, { type: 'paragraph' }), enter: paragraphEnter },
  ...(Object.fromEntries(
    headingKeys.map((k, i) => [k, { match: (n: BlockElement) => n.type === 'heading' && n.level === i + 1, convert: (e: BlocksEditor) => void baseHandleConvert(e, { type: 'heading', level: i + 1 }), snippets: ['#'.repeat(i + 1)] }]),
  ) as Record<(typeof headingKeys)[number], BlockDef>),
  'list-ordered': { match: (n) => n.type === 'list' && n.format === 'ordered', convert: (e) => convertToList(e, 'ordered'), enter: listEnter, backspace: listBackspace, tab: listTab, snippets: ['1.'] },
  'list-unordered': { match: (n) => n.type === 'list' && n.format === 'unordered', convert: (e) => convertToList(e, 'unordered'), enter: listEnter, backspace: listBackspace, tab: listTab, snippets: ['-', '*', '+'] },
  image: { match: (n) => n.type === 'image', backspace: imageBackspace, enter: (e) => Transforms.insertNodes(e, emptyParagraph()), snippets: ['!['] },
  quote: { match: (n) => n.type === 'quote', convert: (e) => void baseHandleConvert(e, { type: 'quote' }), enter: pressEnterTwiceToExit, snippets: ['>'] },
  code: { match: (n) => n.type === 'code', convert: (e) => void baseHandleConvert(e, { type: 'code', language: 'plaintext' }), enter: pressEnterTwiceToExit, snippets: ['```'] },
}

const topBlock = (editor: BlocksEditor) => {
  if (!editor.selection) return undefined
  const node = editor.children[editor.selection.anchor.path[0]!] as BlockElement | undefined
  return node ? (Object.values(blocks).find((b) => b.match(node)) ?? null) : undefined
}

export function handleEnter(editor: BlocksEditor, shift: boolean) {
  if (!editor.selection) return
  const node = editor.children[editor.selection.anchor.path[0]!] as BlockElement | undefined
  const block = topBlock(editor)
  if (!node || !block) return
  if (shift && node.type !== 'image') {
    Transforms.insertText(editor, '\n')
    return
  }
  ;(block.enter ?? paragraphEnter)(editor)
}

export function handleBackspace(editor: BlocksEditor, event: { preventDefault(): void }) {
  topBlock(editor)?.backspace?.(editor, event)
}

export function handleTab(editor: BlocksEditor, event: { preventDefault(): void }) {
  const block = topBlock(editor)
  if (block?.tab) {
    event.preventDefault()
    block.tab(editor)
  }
}

// checkSnippet turns "# " and the like, typed at the start of a block, into
// the matching block. It returns the image block's key when "![" asks for
// the media picker, which the field opens.
export function checkSnippet(editor: BlocksEditor, event: { preventDefault(): void }): BlockKey | null {
  if (!editor.selection) return null
  const [node, path] = Editor.node(editor, editor.selection.anchor.path)
  if (!isText(node) || path.at(-1) !== 0) return null
  const entry = (Object.entries(blocks) as [BlockKey, BlockDef][]).find(([, b]) => b.snippets?.includes(node.text))
  if (!entry) return null
  const [key, block] = entry
  if (!block.convert && key !== 'image') return null
  event.preventDefault()
  Transforms.delete(editor, { distance: node.text.length, unit: 'character', reverse: true })
  if (key === 'image') return 'image'
  block.convert!(editor)
  return key
}

/* Toolbar ------------------------------------------------------------------ */

export function selectBlock(editor: BlocksEditor, key: BlockKey): 'image' | null {
  const empty = editor.children.length === 1 && Editor.isEmpty(editor, editor.children[0] as BlockElement)
  if (!editor.selection && !empty) Transforms.insertNodes(editor, { type: 'quote', children: [{ type: 'text', text: '' }] }, { select: true })
  else if (!editor.selection && empty) Transforms.select(editor, Editor.start(editor, [0, 0]))
  const listEntry = Editor.above(editor, { match: isList })
  if (listEntry && (key === 'list-ordered' || key === 'list-unordered')) {
    const [list, path] = listEntry as NodeEntry<BlockElement>
    const format = key === 'list-ordered' ? 'ordered' : 'unordered'
    if (list.format !== format) Transforms.setNodes(editor, { format }, { at: path })
    return null
  }
  if (key === 'image') return 'image'
  blocks[key].convert?.(editor)
  return null
}

// currentBlockKey is what the block menu shows for the selection.
export function currentBlockKey(editor: BlocksEditor): BlockKey {
  if (!editor.selection) return 'paragraph'
  const top = editor.children[editor.selection.anchor.path[0]!] as BlockElement | undefined
  if (!top) return 'paragraph'
  if (top.type === 'list') {
    const inner = Editor.above(editor, { match: isList, at: editor.selection.anchor })
    return (inner?.[0] as BlockElement | undefined)?.format === 'ordered' ? 'list-ordered' : 'list-unordered'
  }
  return ((Object.entries(blocks) as [BlockKey, BlockDef][]).find(([, b]) => b.match(top))?.[0] ?? 'paragraph') as BlockKey
}

export function toggleList(editor: BlocksEditor, format: 'ordered' | 'unordered') {
  let entry: NodeEntry | undefined
  if (editor.selection) entry = Editor.above(editor, { match: isList })
  else {
    const [, last] = Editor.last(editor, [])
    entry = Editor.above(editor, { match: isList, at: last })
  }
  if (!entry) return convertToList(editor, format)
  const [list, path] = entry as NodeEntry<BlockElement>
  if (list.format !== format) Transforms.setNodes(editor, { format }, { at: path })
  else baseHandleConvert(editor, { type: 'paragraph' })
}

export function isListActive(editor: BlocksEditor, format: 'ordered' | 'unordered'): boolean {
  if (!editor.selection) return false
  const entry = Editor.above(editor, { match: isList, at: editor.selection.anchor })
  return (entry?.[0] as BlockElement | undefined)?.format === format
}

// spansBlocks is true when the selection crosses blocks: lists and links
// apply to one block at a time, as in Strapi.
export function spansBlocks(editor: BlocksEditor): boolean {
  if (!editor.selection) return false
  const match = (n: Node) => !Editor.isEditor(n) && Element.isElement(n) && n.type !== 'text'
  const a = Editor.above(editor, { at: editor.selection.anchor, match })
  const f = Editor.above(editor, { at: editor.selection.focus, match })
  return Boolean(a && f && a[0] !== f[0])
}

// Marks do not apply in code and image blocks.
export function marksDisabled(editor: BlocksEditor): boolean {
  if (!editor.selection) return false
  const top = editor.children[editor.selection.anchor.path[0]!] as BlockElement | undefined
  return !top || top.type === 'image' || top.type === 'code'
}

export type Modifier = (typeof modifierNames)[number]
export const modifiers = modifierNames

export function isMarkActive(editor: BlocksEditor, name: Modifier): boolean {
  const { selection } = editor
  if (!selection || Range.isCollapsed(selection)) return Boolean(Editor.marks(editor)?.[name])
  // On a range, every selected text that is not only spaces must carry the mark.
  const range = Editor.unhangRange(editor, selection)
  let some = false
  let all = true
  for (const [node, path] of Editor.nodes(editor, { at: range, match: Text.isText, mode: 'all' })) {
    const inter = Range.intersection(range, Editor.range(editor, path))
    if (!inter) continue
    const s = Math.min(inter.anchor.offset, inter.focus.offset)
    const e = Math.max(inter.anchor.offset, inter.focus.offset)
    if (node.text.slice(s, e).trim().length === 0) continue
    some = true
    all = all && Boolean(node[name])
  }
  return some && all
}

export function toggleMark(editor: BlocksEditor, name: Modifier) {
  if (!editor.selection) Transforms.select(editor, Editor.end(editor, []))
  if (isMarkActive(editor, name)) Editor.removeMark(editor, name)
  else Editor.addMark(editor, name, true)
}

// markForKey gives the modifier of a Ctrl/Cmd shortcut, Strapi's keys.
export function markForKey(e: { key: string; shiftKey: boolean }): Modifier | null {
  if (e.key === 'b') return 'bold'
  if (e.key === 'i') return 'italic'
  if (e.key === 'u') return 'underline'
  if (e.key === 'S' && e.shiftKey) return 'strikethrough'
  if (e.key === 'e') return 'code'
  return null
}

/* Links -------------------------------------------------------------------- */

export function isLinkActive(editor: BlocksEditor): boolean {
  if (!editor.selection) return false
  const [match] = Array.from(Editor.nodes(editor, { at: Editor.unhangRange(editor, editor.selection), match: (n) => Element.isElement(n) && n.type === 'link' }))
  return Boolean(match)
}

export function insertLink(editor: BlocksEditor, { url }: { url: string }) {
  if (!editor.selection) return
  for (const [, path] of Array.from(Editor.nodes(editor, { at: editor.selection, match: isElementOf('link') }))) Transforms.unwrapNodes(editor, { at: path })
  if (Range.isCollapsed(editor.selection)) {
    Transforms.insertNodes(editor, { type: 'link', url: url ?? '', children: [{ type: 'text', text: url }], rel: '', target: '' })
  } else {
    Transforms.wrapNodes(editor, { type: 'link', url: url ?? '', children: [] }, { split: true })
  }
}

export function addLinkFromToolbar(editor: BlocksEditor) {
  editor.shouldSaveLinkPath = true
  insertLink(editor, { url: '' })
}

export function editLink(editor: BlocksEditor, at: Path, link: { url: string; text: string; rel: string; target: string }) {
  Transforms.setNodes(editor, { url: link.url, rel: link.rel, target: link.target }, { at })
  if (link.text !== '' && link.text !== Editor.string(editor, at)) {
    for (const [, child] of Array.from(Node.children(editor, at, { reverse: true }))) Transforms.removeNodes(editor, { at: child })
    Transforms.insertNodes(editor, [{ type: 'text', text: link.text }], { at: at.concat(0) })
  }
}

export function removeLink(editor: BlocksEditor, at?: Path) {
  Transforms.unwrapNodes(editor, { at, match: isElementOf('link') })
}

/* Images ------------------------------------------------------------------- */

const imageFields = ['name', 'alternativeText', 'url', 'caption', 'width', 'height', 'formats', 'hash', 'ext', 'mime', 'size', 'previewUrl', 'provider', 'provider_metadata', 'createdAt', 'updatedAt'] as const

// imageOf keeps the fields Strapi stores in an image block, the
// alternative text falling back to the file name.
export function imageOf(file: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = Object.fromEntries(imageFields.map((k) => [k, file[k]]))
  out.alternativeText = out.alternativeText || out.name
  return out
}

// insertImages replaces the current block with one image block per file.
export function insertImages(editor: BlocksEditor, images: Record<string, unknown>[]) {
  if (!editor.selection) Transforms.select(editor, Editor.end(editor, []))
  Transforms.unwrapNodes(editor, { match: isList, split: true })
  const entry = Editor.above(editor, { match: (n) => !Editor.isEditor(n) && Element.isElement(n) && !['text', 'link'].includes(n.type) })
  if (!entry) return
  const [, at] = entry
  Transforms.removeNodes(editor)
  Transforms.insertNodes(
    editor,
    images.map((image) => ({ type: 'image', image, children: [{ type: 'text' as const, text: '' }] })),
    { at },
  )
  Transforms.select(editor, at)
}

/* Moves -------------------------------------------------------------------- */

// moveBlock moves the selection's top block one place up or down.
export function moveBlock(editor: BlocksEditor, direction: -1 | 1): boolean {
  if (!editor.selection) return false
  const from = Range.start(editor.selection).path[0]!
  const to = from + direction
  if (to < 0 || to >= editor.children.length) return false
  Transforms.moveNodes(editor, { at: [from], to: [to] })
  return true
}

// isEmptyValue mirrors Strapi: an editor holding only an empty block saves null.
export function normalizeValue(editor: BlocksEditor, value: Descendant[]): Descendant[] | null {
  return value.length === 1 && Editor.isEmpty(editor, value[0] as BlockElement) ? null : value
}
