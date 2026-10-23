import { createBrowserRouter } from 'react-router'
import { routerBasename } from '@/lib/paths'
import { ForgotPassword } from '@/pages/auth/ForgotPassword'
import { Login } from '@/pages/auth/Login'
import { Register } from '@/pages/auth/Register'
import { RegisterAdmin } from '@/pages/auth/RegisterAdmin'
import { ResetPassword } from '@/pages/auth/ResetPassword'
import { NotFound } from '@/pages/errors/Forbidden'
import { Home } from '@/pages/home/Home'
import { Profile } from '@/pages/profile/Profile'
import { Soon } from '@/pages/Soon'
import { RequireAnonymous, RequireAuth, RequirePermission } from './guards'
import { Layout } from './Layout'
import { navItems } from './nav'

const section = (to: string) => {
  const item = navItems.find((i) => i.to === to)
  if (!item) throw new Error(`no nav item ${to}`)
  return {
    path: `${to.slice(1)}/*`,
    element: (
      <RequirePermission allowed={item.allowed}>
        <Soon title={item.label} />
      </RequirePermission>
    ),
  }
}

export const router = createBrowserRouter(
  [
    { path: '/auth/register-admin', element: <RequireAnonymous firstAdmin><RegisterAdmin /></RequireAnonymous> },
    { path: '/auth/login', element: <RequireAnonymous><Login /></RequireAnonymous> },
    { path: '/auth/forgot-password', element: <RequireAnonymous><ForgotPassword /></RequireAnonymous> },
    { path: '/auth/reset-password', element: <RequireAnonymous><ResetPassword /></RequireAnonymous> },
    { path: '/auth/register', element: <RequireAnonymous><Register /></RequireAnonymous> },
    {
      path: '/',
      element: <RequireAuth><Layout /></RequireAuth>,
      children: [
        { index: true, element: <Home /> },
        { path: 'me', element: <Profile /> },
        section('/content-manager'),
        section('/content-type-builder'),
        section('/media-library'),
        section('/settings'),
        { path: '*', element: <NotFound /> },
      ],
    },
  ],
  { basename: routerBasename() },
)
