import { routerBasename } from './paths'

// The admin runs embedded (?embed=1) in the dialog that opens a related
// document from a relation field: no navigation around the page, and a
// message to the parent window when the document is saved.

export const isEmbedded = (search: string) => new URLSearchParams(search).get('embed') === '1'

export function withEmbed(path: string): string {
  return `${path}${path.includes('?') ? '&' : '?'}embed=1`
}

export const appUrl = (path: string) => `${routerBasename()}${path}`

export type SavedMessage = { type: 'grapi:saved'; uid: string; documentId: string; label: string }

export function notifySaved(msg: Omit<SavedMessage, 'type'>) {
  if (window.parent !== window) window.parent.postMessage({ type: 'grapi:saved', ...msg } satisfies SavedMessage, window.location.origin)
}

export function isSavedMessage(e: MessageEvent, from: Window | null | undefined): e is MessageEvent<SavedMessage> {
  return e.origin === window.location.origin && e.source === from && (e.data as SavedMessage | null)?.type === 'grapi:saved'
}
