import { useAuth } from '@/providers/AuthProvider'

// Logo is the project's logo when one is set in the settings (the menu's,
// or the login page's), grapi's otherwise.
export function Logo({ variant = 'menu' }: { variant?: 'menu' | 'auth' }) {
  const { logos } = useAuth()
  const custom = variant === 'auth' ? logos.authLogo : logos.menuLogo
  if (custom) return <img src={custom} alt="Logo" className="h-8 max-w-40 object-contain" />
  return (
    <span className="inline-flex items-center gap-2 text-lg font-bold tracking-tight">
      <span className="inline-flex h-8 w-8 items-center justify-center rounded-md bg-primary text-sm text-primary-foreground">g</span>
      grapi
    </span>
  )
}
