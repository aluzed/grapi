import { expect, test } from 'bun:test'
import { fieldDeletionImpacts, kebab, pluralize, typeDeletionImpacts, type SchemaFile } from './model'

test('kebab', () => {
  expect(kebab('Blog Post')).toBe('blog-post')
  expect(kebab('Événement Spécial!')).toBe('evenement-special')
  expect(kebab('myField')).toBe('my-field')
})

test('pluralize', () => {
  expect(pluralize('book')).toBe('books')
  expect(pluralize('category')).toBe('categories')
  expect(pluralize('class')).toBe('classes')
})

const file = (uid: string, attributes: Record<string, unknown>[]) =>
  ({ uid, info: { displayName: uid.split('.').pop() }, options: {}, pluginOptions: {}, attributes, editable: true }) as unknown as SchemaFile

test('deleting a type lists the fields that go with it', () => {
  const all = [
    file('api::tag.tag', [{ name: 'name', type: 'string' }]),
    file('api::post.post', [
      { name: 'tags', type: 'relation', relation: 'manyToMany', target: 'api::tag.tag' },
      { name: 'seo', type: 'component', component: 'shared.seo' },
      { name: 'body', type: 'dynamiczone', components: ['shared.seo', 'blocks.hero'] },
    ]),
  ]
  expect(typeDeletionImpacts('api::tag.tag', all)).toEqual([{ owner: 'post', field: 'tags', kind: 'relation' }])
  expect(typeDeletionImpacts('shared.seo', all)).toEqual([
    { owner: 'post', field: 'seo', kind: 'component' },
    { owner: 'post', field: 'body', kind: 'zone' },
  ])
})

test('deleting one side of a relation lists the other', () => {
  const tags = { name: 'tags', type: 'relation', relation: 'manyToMany', target: 'api::tag.tag', inversedBy: 'posts' }
  const all = [file('api::post.post', [tags]), file('api::tag.tag', [{ name: 'posts', type: 'relation', target: 'api::post.post', mappedBy: 'tags' }])]
  expect(fieldDeletionImpacts('api::post.post', tags as never, all)).toEqual([{ owner: 'tag', field: 'posts', kind: 'inverse' }])
  expect(fieldDeletionImpacts('api::post.post', { name: 'title', type: 'string' }, all)).toEqual([])
})
