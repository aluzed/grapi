import { expect, test } from 'bun:test'
import type { FieldError } from '@/lib/api'
import en from '@/i18n/en.json'
import fr from '@/i18n/fr.json'
import { localizeValidationError } from './validation'

const attributes = { title: { type: 'string', required: true } }
const nullTitle: FieldError = {
  path: ['title'],
  message: 'title must be a `string` type, but the final value was: `null`.',
  value: null,
}

test.each([en, fr])('localizes an empty required title without changing the API error', (messages) => {
  const error = { message: nullTitle.message, details: { errors: [nullTitle] } }
  const result = localizeValidationError(error, attributes, messages['validation.required'])
  expect(result.fields[0]?.message).toBe(messages['validation.required'])
  expect(result.message).toBe(messages['validation.required'])
  expect(result.localized).toBe(true)
  expect(error.details.errors[0]).toBe(nullTitle)
  expect(nullTitle.message).toContain('`null`')
})

test('recognizes absent and empty required values', () => {
  for (const field of [
    { path: ['title'], message: 'title must be defined.' },
    { path: ['title'], message: 'title cannot be empty', value: '' },
  ]) {
    const result = localizeValidationError({ message: field.message, details: { errors: [field] } }, attributes, fr['validation.required'])
    expect(result.message).toBe(fr['validation.required'])
  }
})

test('preserves unrecognized errors, optional fields and nested paths', () => {
  for (const field of [
    { ...nullTitle, message: 'Custom validation failed' },
    { ...nullTitle, path: ['optionalTitle'] },
    { ...nullTitle, path: ['section', 'title'] },
    { ...nullTitle, message: 'title must be at least 5 characters', value: 'abc' },
    { ...nullTitle, message: 'title must be a `string` type, but the final value was: `42`.', value: 42 },
  ]) {
    const error = { message: 'Original server summary', details: { errors: [field] } }
    const result = localizeValidationError(error, attributes, fr['validation.required'])
    expect(result.fields[0]).toBe(field)
    expect(result.message).toBe(error.message)
    expect(result.localized).toBe(false)
  }
})

test('keeps other field errors visible when one required field is localized', () => {
  const other = { path: ['summary'], message: 'summary must be at most 100 characters', value: 'too long' }
  const result = localizeValidationError({ message: '2 errors occurred', details: { errors: [nullTitle, other] } }, attributes, fr['validation.required'])
  expect(result.fields[1]).toBe(other)
  expect(result.message).toBe(`${fr['validation.required']}, ${other.message}`)
})

test('preserves errors without field details', () => {
  expect(localizeValidationError({ message: 'Forbidden' }, attributes, fr['validation.required'])).toEqual({ fields: [], message: 'Forbidden', localized: false })
})
