export type Kind = 'image' | 'video' | 'audio' | 'file'

export function kindOf(mime: string): Kind {
  for (const k of ['image', 'video', 'audio'] as const) if (mime.startsWith(`${k}/`)) return k
  return 'file'
}

// allowedTypes are plural in Strapi schemas, and "files" stands for every
// mime that is not an image, a video or an audio.
export function accepts(allowedTypes: string[] | undefined, mime: string): boolean {
  if (!allowedTypes?.length) return true
  return allowedTypes.includes(`${kindOf(mime)}s`)
}

export type DropError = 'single' | 'type'

export function checkDrop(files: { type: string }[], opts: { multiple: boolean; allowedTypes?: string[] }): DropError | null {
  if (!opts.multiple && files.length > 1) return 'single'
  if (files.some((f) => !accepts(opts.allowedTypes, f.type || 'application/octet-stream'))) return 'type'
  return null
}

export function reorder<T>(list: T[], from: number, to: number): T[] {
  if (from === to || from < 0 || from >= list.length || to < 0 || to >= list.length) return list
  const next = [...list]
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item as T)
  return next
}

// previewOf is how the media field shows a file; other files, PDF
// included, get their icon, as the admin's CSP has object-src 'none'.
export const previewOf = (mime: string): Kind => kindOf(mime)

// uploadOrigin is plugins.upload.config.publicUrl, which the server puts in
// index.html, or the admin's origin.
export const uploadOrigin = () =>
  document.querySelector<HTMLMetaElement>('meta[name="grapi-upload-url"]')?.content || window.location.origin

// backendUrl is Strapi's prefixFileUrlWithBackendUrl: the editors store a
// file's URL with the uploads' origin when it is relative.
export const backendUrl = (url: string, origin = uploadOrigin()) => (url.startsWith('/') ? `${origin}${url}` : url)
