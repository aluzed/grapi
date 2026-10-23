import { afterEach, expect, test } from 'bun:test'
import { setAccessToken, upload } from './api'

const g = globalThis as { XMLHttpRequest?: unknown; document?: unknown; window?: unknown }
const real = { xhr: g.XMLHttpRequest, document: g.document, window: g.window }

function stubBrowser() {
  g.XMLHttpRequest = FakeXHR
  g.document = { querySelector: () => null }
  g.window = { location: { origin: 'http://localhost' } }
}

afterEach(() => {
  setAccessToken(null)
  for (const k of ['XMLHttpRequest', 'document', 'window'] as const) {
    const v = k === 'XMLHttpRequest' ? real.xhr : real[k]
    if (v === undefined) delete g[k]
    else g[k] = v
  }
})

// A fake XMLHttpRequest that reports two progress steps, then answers,
// unless aborted first.
class FakeXHR {
  static last: FakeXHR
  headers: Record<string, string> = {}
  upload: { onprogress?: (e: { lengthComputable: boolean; loaded: number; total: number }) => void } = {}
  onload?: () => void
  onerror?: () => void
  onabort?: () => void
  status = 0
  statusText = ''
  responseText = ''
  withCredentials = false
  aborted = false
  constructor() {
    FakeXHR.last = this
  }
  open() {}
  setRequestHeader(k: string, v: string) {
    this.headers[k] = v
  }
  send() {
    setTimeout(() => this.upload.onprogress?.({ lengthComputable: true, loaded: 50, total: 100 }), 1)
    setTimeout(() => {
      if (this.aborted) return
      this.upload.onprogress?.({ lengthComputable: true, loaded: 100, total: 100 })
      this.status = 201
      this.responseText = JSON.stringify([{ id: 1 }])
      this.onload?.()
    }, 10)
  }
  abort() {
    this.aborted = true
    this.onabort?.()
  }
}

test('upload reports its progress', async () => {
  stubBrowser()
  setAccessToken('tok')
  const seen: number[] = []
  const res = await upload<{ id: number }[]>('/upload', new FormData(), { onProgress: (p) => seen.push(p) })
  expect(res).toEqual([{ id: 1 }])
  expect(seen).toEqual([0.5, 1])
  expect(FakeXHR.last.headers.Authorization).toBe('Bearer tok')
})

test('upload can be cancelled', async () => {
  stubBrowser()
  const ctrl = new AbortController()
  const pending = upload('/upload', new FormData(), { signal: ctrl.signal })
  setTimeout(() => ctrl.abort(), 3)
  await expect(pending).rejects.toMatchObject({ name: 'AbortError' })
})
