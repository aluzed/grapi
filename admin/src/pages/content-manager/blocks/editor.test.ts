import { expect, test } from 'bun:test'
import { Editor, Transforms, type Descendant } from 'slate'
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
  moveBlock,
  normalizeValue,
  selectBlock,
  toggleList,
  toggleMark,
  type BlocksEditor,
} from './editor'

const noop = { preventDefault() {} }

// json is what the form stores: undefined fields drop out.
const json = (v: unknown): unknown => JSON.parse(JSON.stringify(v))

function start(value: unknown[] = [emptyParagraph()]): BlocksEditor {
  const e = createBlocksEditor()
  e.children = value as Descendant[]
  Transforms.select(e, Editor.end(e, []))
  return e
}

// type mimics the keyboard: a space first checks for a Markdown snippet.
function type(e: BlocksEditor, s: string) {
  for (const ch of s) {
    let prevented = false
    if (ch === ' ') checkSnippet(e, { preventDefault: () => (prevented = true) })
    if (!prevented) Editor.insertText(e, ch)
  }
}

const text = (t: string, marks: Record<string, boolean> = {}) => ({ type: 'text', text: t, ...marks })

test('a heading from the menu, then Enter gives a paragraph', () => {
  const e = start()
  type(e, 'Title')
  selectBlock(e, 'heading-one')
  handleEnter(e, false)
  type(e, 'Body')
  expect(json(e.children)).toEqual([
    { type: 'heading', level: 1, children: [text('Title')] },
    { type: 'paragraph', children: [text('Body')] },
  ])
})

test('Markdown snippets convert the block', () => {
  const e = start()
  type(e, '## Sub')
  expect(json(e.children)).toEqual([{ type: 'heading', level: 2, children: [text('Sub')] }])
  expect(currentBlockKey(e)).toBe('heading-two')
  const q = start()
  type(q, '> Quoted')
  expect(json(q.children)).toEqual([{ type: 'quote', children: [text('Quoted')] }])
  const c = start()
  type(c, '``` x')
  expect(json(c.children)).toEqual([{ type: 'code', language: 'plaintext', children: [text('x')] }])
})

test('a list nested on three levels with Tab', () => {
  const e = start()
  type(e, '- one')
  handleEnter(e, false)
  type(e, 'two')
  handleTab(e, noop)
  handleEnter(e, false)
  type(e, 'three')
  handleTab(e, noop)
  expect(json(e.children)).toEqual([
    {
      type: 'list',
      format: 'unordered',
      children: [
        { type: 'list-item', children: [text('one')] },
        {
          type: 'list',
          format: 'unordered',
          indentLevel: 1,
          children: [
            { type: 'list-item', children: [text('two')] },
            { type: 'list', format: 'unordered', indentLevel: 2, children: [{ type: 'list-item', children: [text('three')] }] },
          ],
        },
      ],
    },
  ])
})

test('Enter on an empty nested item goes up a level, then out of the list', () => {
  const e = start()
  type(e, '1. a')
  handleEnter(e, false)
  type(e, 'b')
  handleTab(e, noop)
  handleEnter(e, false)
  handleEnter(e, false)
  type(e, 'c')
  handleEnter(e, false)
  handleEnter(e, false)
  type(e, 'after')
  expect(json(e.children)).toEqual([
    {
      type: 'list',
      format: 'ordered',
      children: [
        { type: 'list-item', children: [text('a')] },
        { type: 'list', format: 'ordered', indentLevel: 1, children: [{ type: 'list-item', children: [text('b')] }] },
        { type: 'list-item', children: [text('c')] },
      ],
    },
    { type: 'paragraph', children: [text('after')] },
  ])
})

test('toggling a list switches its format, then turns the item back into a paragraph', () => {
  const e = start()
  type(e, 'item')
  toggleList(e, 'unordered')
  expect(json(e.children)).toEqual([{ type: 'list', format: 'unordered', children: [{ type: 'list-item', children: [text('item')] }] }])
  toggleList(e, 'ordered')
  expect((e.children[0] as { format?: unknown }).format).toBe('ordered')
  toggleList(e, 'ordered')
  expect(json(e.children)).toEqual([{ type: 'paragraph', children: [text('item')] }])
})

test('a link from the toolbar, then edited', () => {
  const e = start()
  type(e, 'see ')
  addLinkFromToolbar(e)
  expect(e.lastInsertedLinkPath).toEqual([0, 1])
  editLink(e, [0, 1], { url: 'https://strapi.io', text: 'Strapi', rel: '', target: '' })
  expect(json(e.children)).toEqual([{ type: 'paragraph', children: [text('see '), { type: 'link', url: 'https://strapi.io', rel: '', target: '', children: [text('Strapi')] }, text('')] }])
})

test('a link over a selection', () => {
  const e = start([{ type: 'paragraph', children: [text('go there now')] }])
  Transforms.select(e, { anchor: { path: [0, 0], offset: 3 }, focus: { path: [0, 0], offset: 8 } })
  addLinkFromToolbar(e)
  expect(json(e.children)).toEqual([{ type: 'paragraph', children: [text('go '), { type: 'link', url: '', children: [text('there')] }, text(' now')] }])
})

test('marks', () => {
  const e = start()
  type(e, 'a ')
  toggleMark(e, 'bold')
  type(e, 'b')
  toggleMark(e, 'bold')
  type(e, ' c')
  expect(json(e.children)).toEqual([{ type: 'paragraph', children: [text('a '), text('b', { bold: true }), text(' c')] }])
})

test('images replace the current block, Strapi fields only', () => {
  const e = start([{ type: 'paragraph', children: [text('x')] }, emptyParagraph()])
  const file = { id: 3, documentId: 'd', name: 'cat.png', alternativeText: null, url: '/uploads/cat.png', width: 10, height: 8, mime: 'image/png', ext: '.png', size: 1.2, formats: null, folder: null }
  insertImages(e, [imageOf(file)])
  expect(json(e.children[1])).toEqual({
    type: 'image',
    image: { name: 'cat.png', alternativeText: 'cat.png', url: '/uploads/cat.png', width: 10, height: 8, formats: null, ext: '.png', mime: 'image/png', size: 1.2 },
    children: [text('')],
  })
  handleBackspace(e, noop)
  expect(json(e.children)).toEqual([{ type: 'paragraph', children: [text('x')] }])
})

test('code: Enter adds a line, twice leaves the block', () => {
  const e = start()
  type(e, '``` a')
  handleEnter(e, false)
  type(e, 'b')
  handleEnter(e, false)
  handleEnter(e, false)
  type(e, 'next')
  expect(json(e.children)).toEqual([
    { type: 'code', language: 'plaintext', children: [text('a\nb')] },
    { type: 'paragraph', children: [text('next')] },
  ])
})

test('moving blocks and the empty value', () => {
  const e = start([{ type: 'paragraph', children: [text('1')] }, { type: 'paragraph', children: [text('2')] }])
  Transforms.select(e, Editor.start(e, [1]))
  expect(moveBlock(e, -1)).toBe(true)
  expect(e.children.map((b) => Editor.string(e, [e.children.indexOf(b)]))).toEqual(['2', '1'])
  expect(moveBlock(e, -1)).toBe(false)
  expect(normalizeValue(e, [emptyParagraph()])).toBeNull()
})

test('a document is left untouched until it is edited', () => {
  const doc = [{ type: 'paragraph', children: [{ type: 'text', text: 'kept', bold: true }, { type: 'link', url: '/x', children: [{ type: 'text', text: 'l' }] }, { type: 'text', text: '' }] }]
  const e = start(JSON.parse(JSON.stringify(doc)))
  expect(json(e.children)).toEqual(doc as Descendant[])
})
