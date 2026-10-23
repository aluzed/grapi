import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Settings } from 'lucide-react'
import { cn } from '@/lib/cn'

export type EntityAction = { label: string; icon?: ReactNode; onSelect: () => void; destructive?: boolean }

// The row actions of a table, behind a bordered gear button.
export function EntityActionsMenu({ actions, label }: { actions: EntityAction[]; label: string }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [open])
  return (
    <div ref={ref} className="relative inline-block">
      <button
        type="button"
        title={label}
        aria-label={label}
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="inline-flex h-9 w-9 items-center justify-center rounded-md border border-border bg-surface hover:bg-background"
      >
        <Settings className="h-4 w-4" />
      </button>
      {open && (
        <ul role="menu" className="absolute right-0 z-20 mt-1 min-w-48 rounded-md border border-border bg-surface py-1 shadow-lg">
          {actions.map((a) => (
            <li key={a.label}>
              <button
                type="button"
                role="menuitem"
                title={a.label}
                onClick={() => {
                  setOpen(false)
                  a.onSelect()
                }}
                className={cn('flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-background', a.destructive && 'text-destructive')}
              >
                {a.icon}
                {a.label}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
