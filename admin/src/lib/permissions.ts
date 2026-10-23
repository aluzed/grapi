import type { Permission } from './types'

// can: some permission grants action, on subject when one is given. Row
// level conditions are the server's business.
export function can(perms: Permission[], action: string, subject?: string): boolean {
  return perms.some((p) => p.action === action && (subject === undefined || p.subject === subject || p.subject === null))
}

export function canAny(perms: Permission[], prefix: string): boolean {
  return perms.some((p) => p.action.startsWith(prefix))
}
