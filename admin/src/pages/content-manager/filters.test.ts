import { expect, test } from 'bun:test'
import type { Model } from '@/lib/types'
import { operators, parseFilters, toQuery } from './filters'

const model = {
  uid: 'api::post.post',
  attributes: { title: { type: 'string' }, views: { type: 'integer' }, featured: { type: 'boolean' }, writer: { type: 'relation', relation: 'manyToOne', target: 'api::writer.writer' } },
} as unknown as Model
const writer = { uid: 'api::writer.writer', mainField: 'name' } as unknown as Model

test('operators by type', () => {
  expect(operators({ type: 'integer' })).toContain('$gte')
  expect(operators({ type: 'string' })).toContain('$containsi')
  expect(operators({ type: 'boolean' })).toEqual(['$eq', '$null', '$notNull'])
})

test('toQuery', () => {
  expect(toQuery([], model, [])).toBeNull()
  expect(toQuery([{ field: 'views', op: '$gt', value: '10' }], model, [])).toEqual({ views: { $gt: 10 } })
  expect(
    toQuery(
      [
        { field: 'featured', op: '$eq', value: 'true' },
        { field: 'writer', op: '$containsi', value: 'urs' },
        { field: 'title', op: '$null', value: '' },
      ],
      model,
      [writer],
    ),
  ).toEqual({ $and: [{ featured: { $eq: true } }, { writer: { name: { $containsi: 'urs' } } }, { title: { $null: true } }] })
})

test('parseFilters ignores garbage', () => {
  expect(parseFilters('nope')).toEqual([])
  expect(parseFilters('[{"field":"a","op":"$eq","value":"1"},{"x":1}]')).toEqual([{ field: 'a', op: '$eq', value: '1' }])
})
