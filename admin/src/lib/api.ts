import { apiBase } from './paths'

export type FieldError = { path: string[]; message: string; value?: unknown }

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

// readBody returns the whole JSON envelope, or throws the API error it holds.
async function readBody(res: Response): Promise<{ data?: unknown; meta?: unknown } | null> {
  const json = await res.json().catch(() => null)
  if (!res.ok) {
    const e = json?.error ?? {}
    throw new ApiError(res.status, e.name ?? 'Error', e.message ?? res.statusText, e.details ?? {})
  }
  return json
}

async function parse<T>(res: Response): Promise<T> {
  return (await readBody(res))?.data as T
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

// withRefresh sends, and on a 401 sends again once the access token is
// rotated.
async function withRefresh(send: () => Promise<Response>): Promise<Response> {
  const res = await send()
  if (res.status !== 401 || accessToken === null) return res
  if (await refresh()) return send()
  onSessionLost()
  return res
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  return parse<T>(await withRefresh(() => send(method, path, body)))
}

// requestWithMeta returns data and meta (paginated lists).
export async function requestWithMeta<T, M>(path: string): Promise<{ data: T; meta: M }> {
  const json = await readBody(await withRefresh(() => send('GET', path)))
  return { data: json?.data as T, meta: json?.meta as M }
}

// upload posts a multipart form, with the same refresh-on-401 as request.
export type UploadOptions = { onProgress?: (fraction: number) => void; signal?: AbortSignal }

// xhrPost is fetch for an upload: only XMLHttpRequest reports the progress
// of a request body.
function xhrPost(url: string, form: FormData, { onProgress, signal }: UploadOptions): Promise<Response> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', url)
    xhr.withCredentials = true
    if (accessToken) xhr.setRequestHeader('Authorization', `Bearer ${accessToken}`)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress?.(e.loaded / e.total)
    }
    xhr.onload = () => resolve(new Response(xhr.responseText, { status: xhr.status, statusText: xhr.statusText }))
    xhr.onerror = () => reject(new TypeError('Network error'))
    xhr.onabort = () => reject(new DOMException('Upload cancelled', 'AbortError'))
    if (signal?.aborted) return xhr.onabort?.(new ProgressEvent('abort'))
    signal?.addEventListener('abort', () => xhr.abort(), { once: true })
    xhr.send(form)
  })
}

export async function upload<T>(path: string, form: FormData, options?: UploadOptions): Promise<T> {
  const post = () =>
    options
      ? xhrPost(`${apiBase()}${path}`, form, options)
      : fetch(`${apiBase()}${path}`, { method: 'POST', credentials: 'same-origin', headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : {}, body: form })
  return (await readBody(await withRefresh(post))) as T
}

// download saves an authenticated GET as a file.
export async function download(path: string, filename: string) {
  const res = await withRefresh(() => send('GET', path))
  if (!res.ok) throw new ApiError(res.status, 'Error', res.statusText, {})
  const url = URL.createObjectURL(await res.blob())
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
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
