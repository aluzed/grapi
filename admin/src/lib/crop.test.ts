import { expect, test } from 'bun:test'
import { cropRect } from './crop'

test('cropRect', () => {
  expect(cropRect({ x: 0.25, y: 0.5, w: 0.5, h: 0.25 }, 800, 400)).toEqual({ x: 200, y: 200, width: 400, height: 100 })
  expect(cropRect({ x: 0.9, y: 0.9, w: 0.5, h: 0.5 }, 100, 100)).toEqual({ x: 90, y: 90, width: 10, height: 10 })
  expect(cropRect({ x: 0, y: 0, w: 0, h: 0 }, 100, 100)).toEqual({ x: 0, y: 0, width: 1, height: 1 })
})
