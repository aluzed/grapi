import type { FieldValues, Path, UseFormSetError } from 'react-hook-form'
import { toast } from 'sonner'
import { ApiError } from './api'

// showError maps a validation error of the API onto the form fields, and
// toasts anything else.
export function showError<T extends FieldValues>(err: unknown, setError?: UseFormSetError<T>, fallback = 'Error') {
  if (err instanceof ApiError) {
    const fields = err.fieldErrors()
    if (setError && fields.length > 0) {
      for (const f of fields) {
        const name = f.path[f.path.length - 1]
        if (name) setError(name as Path<T>, { message: f.message })
      }
      return
    }
    toast.error(fields.length > 0 ? fields.map((f) => `${f.path.join('.')}: ${f.message}`).join('\n') : err.message)
    return
  }
  toast.error(fallback)
}
