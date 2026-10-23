import { expect, test } from 'bun:test'
import { kebab, pluralize } from './model'

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
