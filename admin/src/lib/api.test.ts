import { afterEach, expect, test } from 'bun:test'
import { api, onUnauthorized, setAccessToken } from './api'

const realFetch = globalThis.fetch

const g = globalThis as { document?: unknown; window?: unknown }
const realDocument = g.document
const realWindow = g.window

afterEach(() => {
  globalThis.fetch = realFetch
  setAccessToken(null)
  // Other test files import modules that touch the real document.
  g.document = realDocument
  g.window = realWindow
  if (realDocument === undefined) delete g.document
  if (realWindow === undefined) delete g.window
})

// A server whose access tokens expire: "old" is refused, the refresh hands
// out "new", counted to check that concurrent 401s share one rotation.
function fakeServer() {
  const calls = { refresh: 0, retried: 0 }
  globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input)
    const auth = new Headers(init?.headers).get('Authorization')
    if (url.endsWith('/auth/access-token')) {
      calls.refresh++
      await new Promise((r) => setTimeout(r, 20))
      return new Response(JSON.stringify({ data: { token: 'new' } }), { status: 200 })
    }
    if (auth === 'Bearer old') return new Response(JSON.stringify({ error: { name: 'UnauthorizedError', message: 'expired' } }), { status: 401 })
    calls.retried++
    return new Response(JSON.stringify({ data: { ok: url } }), { status: 200 })
  }) as typeof fetch
  return calls
}

test('an expired access token is renewed once and the requests replayed', async () => {
  g.document = { querySelector: () => null }
  g.window = { location: { origin: 'http://x' } }
  const calls = fakeServer()
  setAccessToken('old')
  const results = await Promise.all([api.get<{ ok: string }>('/a'), api.get<{ ok: string }>('/b'), api.put<{ ok: string }>('/c', { x: 1 })])
  expect(results.map((r) => r.ok.split('/').pop())).toEqual(['a', 'b', 'c'])
  expect(calls.refresh).toBe(1)
  expect(calls.retried).toBe(3)
})

test('a failed refresh ends the session', async () => {
  g.document = { querySelector: () => null }
  g.window = { location: { origin: 'http://x' } }
  globalThis.fetch = (async () => new Response(JSON.stringify({ error: { name: 'UnauthorizedError', message: 'no' } }), { status: 401 })) as unknown as typeof fetch
  let lost = 0
  onUnauthorized(() => lost++)
  setAccessToken('old')
  await expect(api.get('/a')).rejects.toThrow()
  expect(lost).toBe(1)
})
