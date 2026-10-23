import { expect, test } from 'bun:test'
import { can, canAny } from './permissions'
import type { Permission } from './types'

const perms: Permission[] = [
  { action: 'plugin::content-manager.explorer.read', subject: 'api::post.post', properties: {}, conditions: [] },
  { action: 'plugin::upload.read', subject: null, properties: {}, conditions: [] },
]

test('can', () => {
  expect(can(perms, 'plugin::content-manager.explorer.read')).toBe(true)
  expect(can(perms, 'plugin::content-manager.explorer.read', 'api::post.post')).toBe(true)
  expect(can(perms, 'plugin::content-manager.explorer.read', 'api::page.page')).toBe(false)
  expect(can(perms, 'plugin::upload.read', 'anything')).toBe(true)
  expect(can(perms, 'admin::webhooks.read')).toBe(false)
})

test('canAny', () => {
  expect(canAny(perms, 'plugin::content-manager.')).toBe(true)
  expect(canAny(perms, 'admin::')).toBe(false)
})
