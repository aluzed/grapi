import type { ComponentProps, ReactNode } from 'react'
import { Loader2 } from 'lucide-react'
import { cn } from '@/lib/cn'

// No ghost or link variant: an action always looks like a button. Grey is
// reserved to the disabled state.
type Variant = 'primary' | 'outline' | 'destructive' | 'destructive-outline'

const variants: Record<Variant, string> = {
  primary: 'bg-primary text-primary-foreground hover:bg-primary-hover border border-primary',
  outline: 'bg-surface text-foreground border border-border hover:bg-background',
  destructive: 'bg-destructive text-white hover:bg-destructive-hover border border-destructive',
  'destructive-outline': 'bg-surface text-destructive border border-destructive/40 hover:bg-destructive/5',
}

export type ButtonProps = ComponentProps<'button'> & {
  variant?: Variant
  icon?: ReactNode
  loading?: boolean
  // title is required: every action carries a tooltip.
  title: string
}

export function Button({ variant = 'primary', icon, loading, className, children, disabled, type = 'button', ...props }: ButtonProps) {
  return (
    <button
      type={type}
      disabled={disabled || loading}
      className={cn(
        'inline-flex min-h-10 items-center justify-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors',
        'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary',
        'disabled:cursor-not-allowed disabled:border-border disabled:bg-zinc-200 disabled:text-zinc-500',
        variants[variant],
        className,
      )}
      {...props}
    >
      {loading ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden /> : icon}
      {children}
    </button>
  )
}
