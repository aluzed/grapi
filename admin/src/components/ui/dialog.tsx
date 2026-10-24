import { useEffect, useRef, type ReactNode } from 'react'
import { X } from 'lucide-react'

type DialogProps = { open: boolean; title: string; onClose: () => void; children: ReactNode; footer?: ReactNode; closeLabel: string; wide?: boolean }

export function Dialog({ open, title, onClose, children, footer, closeLabel, wide }: DialogProps) {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const d = ref.current
    if (!d) return
    if (open && !d.open) d.showModal()
    if (!open && d.open) d.close()
  }, [open])
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onCancel={onClose}
      className={`m-auto ${wide ? 'h-dvh max-h-dvh w-full max-w-full rounded-none sm:h-auto sm:max-h-[85vh] sm:w-[calc(100%-2rem)] sm:max-w-5xl sm:rounded-lg' : 'w-[calc(100%-2rem)] max-w-lg rounded-lg'} border border-border bg-surface p-0 text-foreground shadow-xl backdrop:bg-black/40`}
    >
      <div className={`flex flex-col ${wide ? 'h-full max-h-dvh sm:max-h-[85vh]' : 'max-h-[85vh]'}`}>
        <div className="flex items-start justify-between gap-4 border-b border-border px-4 py-3 sm:px-6">
          <h2 className="text-base font-semibold">{title}</h2>
          <button type="button" onClick={onClose} title={closeLabel} aria-label={closeLabel} className="rounded p-1 text-muted hover:text-foreground">
            <X className="h-5 w-5" />
          </button>
        </div>
        <div className="overflow-y-auto px-4 py-4 sm:px-6">{children}</div>
        {footer && <div className="flex flex-col gap-2 border-t border-border px-4 py-3 sm:px-6 md:flex-row md:items-center md:justify-end">{footer}</div>}
      </div>
    </dialog>
  )
}
