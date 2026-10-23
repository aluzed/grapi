import type { ReactNode } from 'react'
import { Loader2 } from 'lucide-react'
import { Navigate, useLocation } from 'react-router'
import { Forbidden } from '@/pages/errors/Forbidden'
import { useAuth } from '@/providers/AuthProvider'
import type { Information, Permission } from '@/lib/types'

function Splash() {
  return (
    <div className="flex min-h-full items-center justify-center" role="status">
      <Loader2 className="h-6 w-6 animate-spin text-muted" aria-hidden />
    </div>
  )
}

export function RequireAuth({ children }: { children: ReactNode }) {
  const { status, hasAdmin } = useAuth()
  const location = useLocation()
  if (status === 'loading') return <Splash />
  if (status === 'anonymous') return <Navigate to={hasAdmin ? '/auth/login' : '/auth/register-admin'} replace state={{ from: location.pathname }} />
  return children
}

// RequireAnonymous keeps a logged in user out of the auth pages, and sends
// every visitor of an admin without administrator to the first admin form.
export function RequireAnonymous({ children, firstAdmin = false }: { children: ReactNode; firstAdmin?: boolean }) {
  const { status, hasAdmin } = useAuth()
  const location = useLocation()
  if (status === 'loading') return <Splash />
  if (status === 'authenticated') {
    const from = (location.state as { from?: string } | null)?.from
    return <Navigate to={from && !from.startsWith('/auth') ? from : '/'} replace />
  }
  if (!hasAdmin && !firstAdmin) return <Navigate to="/auth/register-admin" replace />
  if (hasAdmin && firstAdmin) return <Navigate to="/auth/login" replace />
  return children
}

export function RequirePermission({ allowed, children }: { allowed: (p: Permission[], info: Information | null) => boolean; children: ReactNode }) {
  const { permissions, information } = useAuth()
  return allowed(permissions, information) ? children : <Forbidden />
}
