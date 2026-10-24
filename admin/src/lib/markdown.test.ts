import { expect, test } from 'bun:test'
import { block, canIndent, enter, heading, indent, inline, insertFiles, list, type State } from './markdown'

// at builds a state from a text where [ and ] mark the selection, or | the caret.
function at(marked: string): State {
  if (marked.includes('|')) {
    const i = marked.indexOf('|')
    return { text: marked.replace('|', ''), start: i, end: i }
  }
  const s = marked.indexOf('[')
  const e = marked.indexOf(']') - 1
  return { text: marked.replace('[', '').replace(']', ''), start: s, end: e }
}

function show({ text, start, end }: State): string {
  if (start === end) return `${text.slice(0, start)}|${text.slice(start)}`
  return `${text.slice(0, start)}[${text.slice(start, end)}]${text.slice(end)}`
}

test('inline actions on a selection', () => {
  expect(show(inline('Bold', at('a [word] b')))).toBe('a **word**| b')
  expect(show(inline('Italic', at('a [word] b')))).toBe('a _word_| b')
  expect(show(inline('Underline', at('a [word] b')))).toBe('a <u>word</u>| b')
  expect(show(inline('Strikethrough', at('a [word] b')))).toBe('a ~~word~~| b')
  expect(show(inline('Link', at('a [word] b')))).toBe('a [word]([link]) b')
})

test('inline actions without a selection insert a selected placeholder', () => {
  expect(show(inline('Bold', at('a |b')))).toBe('a **[Bold]**b')
  expect(show(inline('Italic', at('|')))).toBe('_[Italic]_')
  expect(show(inline('Underline', at('|')))).toBe('<u>[Underline]</u>')
  expect(show(inline('Strikethrough', at('|')))).toBe('~~[Strikethrough]~~')
  expect(show(inline('Link', at('|')))).toBe('[[Link]](link)')
})

test('a repeated action wraps again, as in Strapi', () => {
  expect(inline('Bold', at('[**a**]')).text).toBe('****a****')
})

test('code and quote go on a new line after the current one', () => {
  expect(show(block('Code', at('a[b]c\ndef')))).toBe('ac\n```\nb|\n```\ndef')
  expect(show(block('Code', at('a[b]c')))).toBe('ac\n```\nb|\n```')
  expect(show(block('Quote', at('a[b]c\ndef')))).toBe('ac\n>b|\ndef')
  expect(show(block('Code', at('abc|\ndef')))).toBe('abc\n```\n[Code]\n```\ndef')
  expect(show(block('Quote', at('abc|')))).toBe('abc\n>[Quote]')
  expect(block('Code', at('|')).text).toBe('\n```\nCode\n```')
  expect(block('Code', at('abc|\n')).text).toBe('abc\n```\nCode\n```\n')
})

test('lists', () => {
  expect(show(list('BulletList', at('one|')))).toBe('- one|')
  expect(show(list('NumberList', at('x\no|ne')))).toBe('x\n1. one|')
  expect(list('NumberList', at('[a\nb\nc]')).text).toBe('1. a\n2. b\n3. c')
  expect(list('BulletList', at('[a\nb]')).text).toBe('- a\n- b')
  expect(list('BulletList', at('[- a\nb\n- c]')).text).toBe('a\nb\nc')
  // Strapi removes the length of its own marker, whatever the line holds.
  expect(list('NumberList', at('[1. a\n2. b]')).text).toBe('a\nb')
})

test('headings replace the current one', () => {
  expect(show(heading('h2', at('ti|tle')))).toBe('## title|')
  expect(show(heading('h1', at('### ti|tle')))).toBe('# title|')
  expect(heading('h3', at('a\n|b\nc')).text).toBe('a\n### b\nc')
})

test('insertFiles', () => {
  const img = { alt: 'cat', url: '/uploads/cat.png', mime: 'image/png' }
  const pdf = { alt: 'doc', url: '/uploads/doc.pdf', mime: 'application/pdf' }
  expect(insertFiles([img], at('|')).text).toBe('![cat](/uploads/cat.png)')
  expect(insertFiles([img], at('ab|c')).text).toBe('abc\n![cat](/uploads/cat.png)')
  expect(insertFiles([img, pdf], at('|')).text).toBe('![cat](/uploads/cat.png)\n[doc](/uploads/doc.pdf)')
  // At the start of a line with text, Strapi appends to the end of the line.
  expect(insertFiles([img], at('|abc\nnext')).text).toBe('abc![cat](/uploads/cat.png)\nnext')
})

test('enter continues lists and quotes', () => {
  expect(show(enter(at('- a|')))).toBe('- a\n- |')
  expect(show(enter(at('  * a|')))).toBe('  * a\n  * |')
  expect(show(enter(at('- [x] done|')))).toBe('- [x] done\n- [ ] |')
  expect(show(enter(at('>quote|')))).toBe('>quote\n>|')
  expect(show(enter(at('- |')))).toBe('\n|')
  expect(show(enter(at('plain|')))).toBe('plain\n|')
  expect(show(enter(at('  code|')))).toBe('  code\n  |')
})

test('enter renumbers the items that follow', () => {
  expect(show(enter(at('1. a|\n2. b\n3. c')))).toBe('1. a\n2. |\n3. b\n4. c')
})

test('Tab indents list items and code only', () => {
  expect(canIndent(at('- a|'))).toBe(true)
  expect(canIndent(at('```\nco|de\n```'))).toBe(true)
  expect(canIndent(at('```\ncode\n```\nte|xt'))).toBe(false)
  expect(canIndent(at('te|xt'))).toBe(false)
  expect(show(indent(at('- a|'), false))).toBe('  - a|')
  expect(show(indent(at('  - a|'), true))).toBe('- a|')
  expect(indent(at('[- a\n- b]'), false).text).toBe('  - a\n  - b')
})
