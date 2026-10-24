import { expect, test } from 'bun:test'
import { accepts, checkDrop, kindOf, previewOf, reorder } from './media'

test('kindOf', () => {
  expect(kindOf('image/png')).toBe('image')
  expect(kindOf('video/mp4')).toBe('video')
  expect(kindOf('audio/mpeg')).toBe('audio')
  expect(kindOf('application/pdf')).toBe('file')
})

test('accepts follows allowedTypes', () => {
  expect(accepts(undefined, 'video/mp4')).toBe(true)
  expect(accepts([], 'video/mp4')).toBe(true)
  expect(accepts(['images'], 'image/webp')).toBe(true)
  expect(accepts(['images'], 'video/mp4')).toBe(false)
  expect(accepts(['files'], 'application/pdf')).toBe(true)
  expect(accepts(['files'], 'image/png')).toBe(false)
})

test('checkDrop', () => {
  const png = { type: 'image/png' }
  expect(checkDrop([png, png], { multiple: false })).toBe('single')
  expect(checkDrop([png, png, png], { multiple: true })).toBeNull()
  expect(checkDrop([png, { type: 'video/mp4' }], { multiple: true, allowedTypes: ['images'] })).toBe('type')
  expect(checkDrop([{ type: '' }], { multiple: true, allowedTypes: ['files'] })).toBeNull()
})

test('reorder', () => {
  expect(reorder([1, 2, 3], 0, 2)).toEqual([2, 3, 1])
  expect(reorder([1, 2, 3], 2, 0)).toEqual([3, 1, 2])
  expect(reorder([1, 2, 3], 1, 1)).toEqual([1, 2, 3])
  expect(reorder([1, 2, 3], 1, 5)).toEqual([1, 2, 3])
})

test('previewOf', () => {
  expect(previewOf('application/pdf')).toBe('pdf')
  expect(previewOf('image/gif')).toBe('image')
  expect(previewOf('text/csv')).toBe('file')
})
