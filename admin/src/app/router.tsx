import { lazy, Suspense, type ComponentType } from 'react'
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
import { ConfigureView } from '@/pages/content-manager/ConfigureView'
import { ContentManager, FirstModel } from '@/pages/content-manager/ContentManager'
import { EditView } from '@/pages/content-manager/EditView'
import { ListView } from '@/pages/content-manager/ListView'
import { Overview } from '@/pages/settings/Overview'
import { Settings, sections } from '@/pages/settings/Settings'

const settingsAllowed = (to: string) => sections.find((s) => s.to === to)!.allowed
import { RequireAnonymous, RequireAuth, RequirePermission } from './guards'
import { Layout } from './Layout'
import { navItems } from './nav'

function lazyPage(load: () => Promise<{ default: ComponentType }>) {
  const Page = lazy(load)
  return function LazyPage() {
    return (
      <Suspense fallback={<p className="text-sm text-muted">…</p>}>
        <Page />
      </Suspense>
    )
  }
}

// The pages away from the content manager load when first opened.
const ApiTokens = lazyPage(() => import('@/pages/settings/ApiTokens').then((m) => ({ default: m.ApiTokens })))
const AuditLogs = lazyPage(() => import('@/pages/settings/AuditLogs').then((m) => ({ default: m.AuditLogs })))
const Email = lazyPage(() => import('@/pages/settings/Email').then((m) => ({ default: m.Email })))
const MediaSettings = lazyPage(() => import('@/pages/settings/MediaSettings').then((m) => ({ default: m.MediaSettings })))
const ReleaseSettings = lazyPage(() => import('@/pages/settings/ReleaseSettings').then((m) => ({ default: m.ReleaseSettings })))
const Locales = lazyPage(() => import('@/pages/settings/Locales').then((m) => ({ default: m.Locales })))
const RoleEditor = lazyPage(() => import('@/pages/settings/RoleEditor').then((m) => ({ default: m.RoleEditor })))
const Roles = lazyPage(() => import('@/pages/settings/Roles').then((m) => ({ default: m.Roles })))
const Users = lazyPage(() => import('@/pages/settings/Users').then((m) => ({ default: m.Users })))
const Webhooks = lazyPage(() => import('@/pages/settings/Webhooks').then((m) => ({ default: m.Webhooks })))
const ContentTypeBuilder = lazyPage(() => import('@/pages/ctb/ContentTypeBuilder').then((m) => ({ default: m.ContentTypeBuilder })))
const MediaLibrary = lazyPage(() => import('@/pages/media/MediaLibrary').then((m) => ({ default: m.MediaLibrary })))
const ReleaseDetail = lazyPage(() => import('@/pages/releases/ReleaseDetail').then((m) => ({ default: m.ReleaseDetail })))
const Releases = lazyPage(() => import('@/pages/releases/Releases').then((m) => ({ default: m.Releases })))
const UPRoleEditor = lazyPage(() => import('@/pages/settings/up/UPRoles').then((m) => ({ default: m.UPRoleEditor })))
const UPRoles = lazyPage(() => import('@/pages/settings/up/UPRoles').then((m) => ({ default: m.UPRoles })))
const UPAdvanced = lazyPage(() => import('@/pages/settings/up/UPSettings').then((m) => ({ default: m.UPAdvanced })))
const UPEmails = lazyPage(() => import('@/pages/settings/up/UPSettings').then((m) => ({ default: m.UPEmails })))
const UPProviders = lazyPage(() => import('@/pages/settings/up/UPSettings').then((m) => ({ default: m.UPProviders })))

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
        {
          path: 'content-manager',
          element: (
            <RequirePermission allowed={navItems.find((i) => i.to === '/content-manager')!.allowed}>
              <ContentManager />
            </RequirePermission>
          ),
          children: [
            { index: true, element: <FirstModel /> },
            { path: 'collection-types/:uid', element: <ListView /> },
            { path: 'collection-types/:uid/configurations', element: <ConfigureView /> },
            { path: 'single-types/:uid/configurations', element: <ConfigureView /> },
            { path: 'collection-types/:uid/create', element: <EditView /> },
            { path: 'collection-types/:uid/:documentId', element: <EditView /> },
            { path: 'single-types/:uid', element: <EditView /> },
          ],
        },
        { path: 'content-type-builder', element: <RequirePermission allowed={navItems.find((i) => i.to === '/content-type-builder')!.allowed}><ContentTypeBuilder /></RequirePermission> },
        { path: 'media-library', element: <RequirePermission allowed={navItems.find((i) => i.to === '/media-library')!.allowed}><MediaLibrary /></RequirePermission> },
        { path: 'releases', element: <RequirePermission allowed={navItems.find((i) => i.to === '/releases')!.allowed}><Releases /></RequirePermission> },
        { path: 'releases/:id', element: <RequirePermission allowed={navItems.find((i) => i.to === '/releases')!.allowed}><ReleaseDetail /></RequirePermission> },
        {
          path: 'settings',
          element: <Settings />,
          children: [
            { index: true, element: <Overview /> },
            { path: 'users', element: <RequirePermission allowed={settingsAllowed('/settings/users')}><Users /></RequirePermission> },
            { path: 'roles', element: <RequirePermission allowed={settingsAllowed('/settings/roles')}><Roles /></RequirePermission> },
            { path: 'roles/:id', element: <RequirePermission allowed={settingsAllowed('/settings/roles')}><RoleEditor /></RequirePermission> },
            { path: 'api-tokens', element: <RequirePermission allowed={settingsAllowed('/settings/api-tokens')}><ApiTokens /></RequirePermission> },
            { path: 'webhooks', element: <RequirePermission allowed={settingsAllowed('/settings/webhooks')}><Webhooks /></RequirePermission> },
            { path: 'users-permissions/roles', element: <RequirePermission allowed={settingsAllowed('/settings/users-permissions/roles')}><UPRoles /></RequirePermission> },
            { path: 'users-permissions/roles/:id', element: <RequirePermission allowed={settingsAllowed('/settings/users-permissions/roles')}><UPRoleEditor /></RequirePermission> },
            { path: 'users-permissions/advanced', element: <RequirePermission allowed={settingsAllowed('/settings/users-permissions/advanced')}><UPAdvanced /></RequirePermission> },
            { path: 'users-permissions/email-templates', element: <RequirePermission allowed={settingsAllowed('/settings/users-permissions/email-templates')}><UPEmails /></RequirePermission> },
            { path: 'users-permissions/providers', element: <RequirePermission allowed={settingsAllowed('/settings/users-permissions/providers')}><UPProviders /></RequirePermission> },
            { path: 'email', element: <RequirePermission allowed={settingsAllowed('/settings/email')}><Email /></RequirePermission> },
            { path: 'releases', element: <RequirePermission allowed={settingsAllowed('/settings/releases')}><ReleaseSettings /></RequirePermission> },
            { path: 'media-library', element: <RequirePermission allowed={settingsAllowed('/settings/media-library')}><MediaSettings /></RequirePermission> },
            { path: 'audit-logs', element: <RequirePermission allowed={settingsAllowed('/settings/audit-logs')}><AuditLogs /></RequirePermission> },
            { path: 'locales', element: <RequirePermission allowed={settingsAllowed('/settings/locales')}><Locales /></RequirePermission> },
          ],
        },
        { path: '*', element: <NotFound /> },
      ],
    },
  ],
  { basename: routerBasename() },
)
