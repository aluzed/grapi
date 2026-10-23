import { expect, test } from 'bun:test'
import { isoToZoned, zonedToISO } from './tz'

test('zonedToISO', () => {
  expect(zonedToISO('2026-01-15T09:30', 'Europe/Paris')).toBe('2026-01-15T08:30:00Z')
  expect(zonedToISO('2026-07-15T09:30', 'Europe/Paris')).toBe('2026-07-15T07:30:00Z')
  expect(zonedToISO('2026-07-15T09:30', 'America/New_York')).toBe('2026-07-15T13:30:00Z')
  expect(zonedToISO('2026-07-15T09:30', 'UTC')).toBe('2026-07-15T09:30:00Z')
  expect(zonedToISO('2026-10-25T09:00', 'Europe/Paris')).toBe('2026-10-25T08:00:00Z')
  expect(zonedToISO('2026-03-29T09:00', 'Europe/Paris')).toBe('2026-03-29T07:00:00Z')
})

test('isoToZoned', () => {
  expect(isoToZoned('2026-07-15T07:30:00Z', 'Europe/Paris')).toBe('2026-07-15T09:30')
  expect(isoToZoned('2026-01-15T08:30:00Z', 'Asia/Tokyo')).toBe('2026-01-15T17:30')
})
