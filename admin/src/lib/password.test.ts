import { expect, test } from 'bun:test'
import { passwordSchema } from './password'

const schema = passwordSchema((k) => k)

test('password policy', () => {
  expect(schema.safeParse('Passw0rd').success).toBe(true)
  expect(schema.safeParse('password').success).toBe(false)
  expect(schema.safeParse('PASSW0RD').success).toBe(false)
  expect(schema.safeParse('Password').success).toBe(false)
  expect(schema.safeParse('Pa0').success).toBe(false)
  expect(schema.safeParse('Pa0' + 'é'.repeat(35)).success).toBe(false)
})
