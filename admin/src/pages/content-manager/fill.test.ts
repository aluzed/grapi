import { expect, test } from 'bun:test'
import type { Attribute } from '@/lib/types'
import { fillableFields } from './fill'

test('fill copies the localized fields the user may change', () => {
  const attrs = {
    title: { type: 'string', pluginOptions: { i18n: { localized: true } } },
    views: { type: 'integer', pluginOptions: { i18n: { localized: false } } },
    code: { type: 'string' },
    slug: { type: 'uid' },
    tags: { type: 'relation', relation: 'manyToMany', target: 'api::tag.tag' },
    note: { type: 'string', pluginOptions: { i18n: { localized: true } } },
  } as unknown as Record<string, Attribute>
  expect(fillableFields(attrs, ['title', 'views', 'code', 'slug', 'tags', 'note', 'missing'], (name) => name !== 'note')).toEqual(['title', 'slug', 'tags'])
})
