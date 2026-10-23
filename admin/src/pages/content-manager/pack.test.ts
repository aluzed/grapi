import { expect, test } from 'bun:test'
import { pack } from './ConfigureView'

test('pack fills rows of 12', () => {
  expect(pack([{ name: 'a', size: 6 }, { name: 'b', size: 4 }, { name: 'c', size: 4 }, { name: 'd', size: 12 }, { name: 'e', size: 8 }])).toEqual([
    [{ name: 'a', size: 6 }, { name: 'b', size: 4 }],
    [{ name: 'c', size: 4 }],
    [{ name: 'd', size: 12 }],
    [{ name: 'e', size: 8 }],
  ])
})
