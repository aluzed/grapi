import { expect, test } from 'bun:test'
import type { Attribute } from '@/lib/types'
import { fromInput, toInput, type FieldContext } from './fields'

process.env.TZ = 'Europe/Paris'

const ctx = { models: [], components: new Map() } as unknown as FieldContext
const cases: [Attribute['type'], unknown][] = [
  ['string', 'Hello'],
  ['text', 'Line 1\nLine 2'],
  ['richtext', '# Title'],
  ['email', 'ada@example.com'],
  ['uid', 'hello-world'],
  ['enumeration', 'draft'],
  ['integer', 42],
  ['biginteger', '9007199254740993'],
  ['float', 3.25],
  ['decimal', 19.99],
  ['boolean', true],
  ['date', '2026-10-23'],
  ['time', '12:34:56.000'],
  ['time', '08:15:00.000'],
  ['datetime', '2026-10-23T15:35:45.385Z'],
  ['datetime', '2026-01-05T09:00:00.000Z'],
  ['timestamp', '2026-07-01T22:30:00.000Z'],
  ['json', { a: [1, 'two'], b: null }],
  ['blocks', [{ type: 'paragraph', children: [{ type: 'text', text: 'Hi' }] }]],
]

test.each(cases)('%s keeps its value through the form', (type, value) => {
  const attr = { type } as Attribute
  expect(fromInput(attr, toInput(attr, value, ctx), ctx)).toEqual(value)
})

test('a datetime shows in local time', () => {
  expect(toInput({ type: 'datetime' } as Attribute, '2026-10-23T15:35:00.000Z', ctx)).toBe('2026-10-23T17:35')
})

test('a timestamp read as milliseconds is sent back as the same instant', () => {
  const attr = { type: 'timestamp' } as Attribute
  expect(toInput(attr, '1782945000000', ctx)).toBe('2026-07-02T00:30')
  expect(fromInput(attr, toInput(attr, '1782945000000', ctx), ctx)).toBe('2026-07-01T22:30:00.000Z')
})
