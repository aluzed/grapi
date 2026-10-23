import type { ReactNode } from 'react'
import { Boxes, FileText, House, Image, Settings } from 'lucide-react'
import { can, canAny } from '@/lib/permissions'
import type { Information, Permission } from '@/lib/types'
import type { MessageKey } from '@/i18n'

export type NavItem = { to: string; label: MessageKey; icon: ReactNode; allowed: (p: Permission[], info: Information | null) => boolean }

const icon = 'h-4 w-4 shrink-0'

export const navItems: NavItem[] = [
  { to: '/', label: 'nav.home', icon: <House className={icon} />, allowed: () => true },
  { to: '/content-manager', label: 'nav.contentManager', icon: <FileText className={icon} />, allowed: (p) => canAny(p, 'plugin::content-manager.explorer.') },
  {
    to: '/content-type-builder',
    label: 'nav.contentTypeBuilder',
    icon: <Boxes className={icon} />,
    allowed: (p) => can(p, 'plugin::content-type-builder.read'),
  },
  { to: '/media-library', label: 'nav.mediaLibrary', icon: <Image className={icon} />, allowed: (p) => can(p, 'plugin::upload.read') },
  {
    to: '/settings',
    label: 'nav.settings',
    icon: <Settings className={icon} />,
    allowed: () => true,
  },
]
