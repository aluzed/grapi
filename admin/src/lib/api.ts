import { apiBase } from './paths'

export type FieldError = { path: string[]; message: string }

export class ApiError extends Error {
  status: number
  name: string
  details: { errors?: FieldError[] } & Record<string, unknown>

  constructor(status: number, name: string, message: string, details: ApiError['details']) {
    super(message)
    this.status = status
    this.name = name
    this.details = details
  }

  fieldErrors(): FieldError[] {
    return this.details.errors ?? []
  }
}

let accessToken: string | null = null
let refreshing: Promise<boolean> | null = null
let onSessionLost: () => void = () => {}

export function setAccessToken(token: string | null) {
  accessToken = token
}

export function onUnauthorized(fn: () => void) {
  onSessionLost = fn
}

async function send(method: string, path: string, body?: unknown, withAuth = true): Promise<Response> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (withAuth && accessToken) headers.Authorization = `Bearer ${accessToken}`
  return fetch(`${apiBase()}${path}`, {
    method,
    headers,
    credentials: 'same-origin',
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

async function parse<T>(res: Response): Promise<T> {
  const json = await res.json().catch(() => null)
  if (!res.ok) {
    const e = json?.error ?? {}
    throw new ApiError(res.status, e.name ?? 'Error', e.message ?? res.statusText, e.details ?? {})
  }
  return json?.data as T
}

// refresh exchanges the httpOnly refresh cookie for a new access token. One
// call at a time: concurrent 401s wait for the same rotation, as a second
// rotation with the old cookie would be taken for a replay.
export function refresh(): Promise<boolean> {
  refreshing ??= (async () => {
    try {
      const res = await send('POST', '/auth/access-token', undefined, false)
      if (!res.ok) {
        accessToken = null
        return false
      }
      const data = await parse<{ token: string }>(res)
      accessToken = data.token
      return true
    } catch {
      return false
    } finally {
      refreshing = null
    }
  })()
  return refreshing
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let res = await send(method, path, body)
  if (res.status === 401 && accessToken !== null) {
    if (await refresh()) {
      res = await send(method, path, body)
    } else {
      onSessionLost()
    }
  }
  return parse<T>(res)
}

export async function publicRequest<T>(method: string, path: string, body?: unknown): Promise<T> {
  return parse<T>(await send(method, path, body, false))
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body ?? {}),
  del: <T>(path: string) => request<T>('DELETE', path),
}
