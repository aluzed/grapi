import { expect, test } from 'bun:test'
import { fromItems, inlineToMarkdown, markdownToInline, toItems, type Block } from './blocks'

test('inline marks', () => {
  expect(markdownToInline('Hello **bold** and *it* `c` ~~s~~ <u>u</u>')).toEqual([
    { type: 'text', text: 'Hello ' },
    { type: 'text', text: 'bold', bold: true },
    { type: 'text', text: ' and ' },
    { type: 'text', text: 'it', italic: true },
    { type: 'text', text: ' ' },
    { type: 'text', text: 'c', code: true },
    { type: 'text', text: ' ' },
    { type: 'text', text: 's', strikethrough: true },
    { type: 'text', text: ' ' },
    { type: 'text', text: 'u', underline: true },
  ])
})

test('nested marks and links', () => {
  expect(markdownToInline('***both***')).toEqual([{ type: 'text', text: 'both', bold: true, italic: true }])
  expect(markdownToInline('see [the **docs**](https://x.dev)')).toEqual([
    { type: 'text', text: 'see ' },
    { type: 'link', url: 'https://x.dev', children: [{ type: 'text', text: 'the ' }, { type: 'text', text: 'docs', bold: true }] },
  ])
  expect(markdownToInline('')).toEqual([{ type: 'text', text: '' }])
})

test('round trip', () => {
  const blocks: Block[] = [
    { type: 'heading', level: 2, children: [{ type: 'text', text: 'Title' }] },
    { type: 'paragraph', children: [{ type: 'text', text: 'a ' }, { type: 'text', text: 'b', bold: true, italic: true }, { type: 'link', url: '/x', children: [{ type: 'text', text: 'l' }] }] },
    { type: 'list', format: 'ordered', children: [{ type: 'list-item', children: [{ type: 'text', text: 'one' }] }, { type: 'list-item', children: [{ type: 'text', text: 'two', code: true }] }] },
    { type: 'quote', children: [{ type: 'text', text: 'q' }] },
    { type: 'code', language: 'go', children: [{ type: 'text', text: 'x := **1**' }] },
    { type: 'image', image: { url: '/u.png', name: 'u.png' }, children: [{ type: 'text', text: '' }] },
  ]
  expect(fromItems(toItems(blocks))).toEqual(blocks)
  expect(inlineToMarkdown(blocks[1]!.type === 'paragraph' ? blocks[1]!.children : [])).toBe('a ***b***[l](/x)')
})

test('literal characters survive', () => {
  const children = [{ type: 'text' as const, text: '2*3*4 [x] ~a~ <b> back\\slash' }, { type: 'text' as const, text: 'b*', bold: true }]
  expect(markdownToInline(inlineToMarkdown(children))).toEqual(children)
})
