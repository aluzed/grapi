import { forwardRef, useId, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

const control =
  'block w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-foreground placeholder:text-muted focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20 aria-[invalid=true]:border-destructive'

type FieldProps = { label: string; error?: string; hint?: ReactNode }

export const TextField = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & FieldProps>(function TextField(
  { label, error, hint, className, id, ...props },
  ref,
) {
  const auto = useId()
  const inputId = id ?? auto
  return (
    <div className={cn('flex flex-col gap-1.5', className)}>
      <label htmlFor={inputId} className="text-sm font-medium">
        {label}
      </label>
      <input ref={ref} id={inputId} aria-invalid={error ? true : undefined} aria-describedby={error ? `${inputId}-error` : undefined} className={control} {...props} />
      {hint && !error && <p className="text-xs text-muted">{hint}</p>}
      {error && (
        <p id={`${inputId}-error`} className="text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  )
})

export const SelectField = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement> & FieldProps>(function SelectField(
  { label, error, hint, className, id, children, ...props },
  ref,
) {
  const auto = useId()
  const selectId = id ?? auto
  return (
    <div className={cn('flex flex-col gap-1.5', className)}>
      <label htmlFor={selectId} className="text-sm font-medium">
        {label}
      </label>
      <select ref={ref} id={selectId} aria-invalid={error ? true : undefined} className={control} {...props}>
        {children}
      </select>
      {hint && !error && <p className="text-xs text-muted">{hint}</p>}
      {error && <p className="text-xs text-destructive">{error}</p>}
    </div>
  )
})

// Switch is a boolean that reads as an on/off setting, its label on the left.
export const Switch = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { label: string; description?: ReactNode }>(function Switch(
  { label, description, className, ...props },
  ref,
) {
  return (
    <label className={cn('flex min-h-10 cursor-pointer items-center justify-between gap-3 text-sm has-[:disabled]:cursor-not-allowed', className)}>
      <span className="min-w-0">
        <span className="block font-medium">{label}</span>
        {description && <span className="block text-xs text-muted">{description}</span>}
      </span>
      <input ref={ref} type="checkbox" role="switch" className="peer sr-only" {...props} />
      <span
        aria-hidden
        className="relative inline-flex h-6 w-11 shrink-0 rounded-full bg-zinc-300 transition-colors after:absolute after:top-0.5 after:left-0.5 after:h-5 after:w-5 after:rounded-full after:bg-white after:shadow after:transition-transform peer-checked:bg-primary peer-checked:after:translate-x-5 peer-focus-visible:ring-2 peer-focus-visible:ring-primary/40 peer-disabled:opacity-50"
      />
    </label>
  )
})

export const Checkbox = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { label: string }>(function Checkbox(
  { label, className, ...props },
  ref,
) {
  return (
    <label className={cn('flex min-h-10 cursor-pointer items-center gap-2 text-sm', className)}>
      <input ref={ref} type="checkbox" className="h-4 w-4 rounded border-border accent-primary" {...props} />
      {label}
    </label>
  )
})
