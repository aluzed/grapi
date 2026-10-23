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
import { ApiTokens } from '@/pages/settings/ApiTokens'
import { Email } from '@/pages/settings/Email'
import { Locales } from '@/pages/settings/Locales'
import { Overview } from '@/pages/settings/Overview'
import { RoleEditor } from '@/pages/settings/RoleEditor'
import { Roles } from '@/pages/settings/Roles'
import { Settings, sections } from '@/pages/settings/Settings'

const settingsAllowed = (to: string) => sections.find((s) => s.to === to)!.allowed
import { UPRoleEditor, UPRoles } from '@/pages/settings/up/UPRoles'
import { UPAdvanced, UPEmails, UPProviders } from '@/pages/settings/up/UPSettings'
import { Users } from '@/pages/settings/Users'
import { Webhooks } from '@/pages/settings/Webhooks'
import { ContentTypeBuilder } from '@/pages/ctb/ContentTypeBuilder'
import { MediaLibrary } from '@/pages/media/MediaLibrary'
import { RequireAnonymous, RequireAuth, RequirePermission } from './guards'
import { Layout } from './Layout'
import { navItems } from './nav'

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
            { path: 'locales', element: <RequirePermission allowed={settingsAllowed('/settings/locales')}><Locales /></RequirePermission> },
          ],
        },
        { path: '*', element: <NotFound /> },
      ],
    },
  ],
  { basename: routerBasename() },
)
