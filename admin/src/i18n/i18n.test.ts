import { expect, test } from 'bun:test'
import en from './en.json'
import fr from './fr.json'
import { format } from './index'

test('fr has the keys of en', () => {
  expect(Object.keys(fr).sort()).toEqual(Object.keys(en).sort())
})

test('format', () => {
  expect(format('Hello {name}', { name: 'Ada' })).toBe('Hello Ada')
  expect(format('Hello {name}')).toBe('Hello {name}')
})
