import { expect, test } from 'bun:test'
import type { Attribute } from '@/lib/types'
import { splitLayout } from './sections'

const a = (type: string, extra: Partial<Attribute> = {}) => ({ type, ...extra }) as Attribute

test('splitLayout groups the fields and repacks the short ones', () => {
  const attrs = {
    title: a('string'),
    content: a('richtext'),
    slug: a('uid'),
    description: a('text'),
    cover: a('media'),
    gallery: a('media', { multiple: true }),
    category: a('relation'),
    hidden: a('boolean'),
    extra: a('json'),
    seo: a('component'),
    count: a('integer'),
  }
  const layout = [
    [{ name: 'title', size: 6 }, { name: 'content', size: 6 }],
    [{ name: 'slug', size: 6 }, { name: 'description', size: 6 }],
    [{ name: 'cover', size: 6 }, { name: 'gallery', size: 6 }],
    [{ name: 'category', size: 6 }, { name: 'hidden', size: 4 }, { name: 'count', size: 4 }],
    [{ name: 'extra', size: 12 }, { name: 'seo', size: 12 }, { name: 'gone', size: 6 }],
  ]
  const s = splitLayout(layout, attrs)
  expect(s.general).toEqual([[{ name: 'title', size: 6 }, { name: 'slug', size: 6 }], [{ name: 'description', size: 12 }], [{ name: 'count', size: 12 }]])
  expect(s.content).toEqual([[{ name: 'content', size: 12 }]])
  expect(s.cover).toEqual([[{ name: 'cover', size: 12 }]])
  expect(s.media).toEqual([[{ name: 'gallery', size: 12 }]])
  expect(s.relations).toEqual([[{ name: 'category', size: 12 }]])
  expect(s.options).toEqual([[{ name: 'hidden', size: 12 }]])
  expect(s.advanced).toEqual([[{ name: 'extra', size: 12 }]])
  expect(s.structure).toEqual([[{ name: 'seo', size: 12 }]])
})
