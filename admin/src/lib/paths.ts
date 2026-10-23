// The server injects <base href="<admin.url>/">; the dev server has none.
export function adminBase(): string {
  const base = document.querySelector('base')?.getAttribute('href')
  const path = base ? new URL(base, window.location.origin).pathname : '/admin/'
  return path.endsWith('/') ? path : `${path}/`
}

export const apiBase = () => `${adminBase()}api/v1`
export const routerBasename = () => adminBase().replace(/\/$/, '')
