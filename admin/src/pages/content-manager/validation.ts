import type { FieldError } from '@/lib/api'
import type { Attribute } from '@/lib/types'

type ValidationError = { message: string; details?: { errors?: FieldError[] } }

export function localizeValidationError(error: ValidationError, attributes: Record<string, Attribute>, requiredMessage: string) {
  let localized = false
  const fields = (error.details?.errors ?? []).map((field) => {
    const name = field.path[0]
    if (field.path.length !== 1 || !name || !attributes[name]?.required) return field

    // The API keeps Strapi's validation messages; translate only known missing-value errors in the editor.
    const missing =
      (field.value == null && field.message === `${name} must be defined.`) ||
      (field.value === '' && field.message === `${name} cannot be empty`) ||
      (field.value === null && field.message.startsWith(`${name} must be a \``) && field.message.endsWith('` type, but the final value was: `null`.'))
    if (!missing) return field
    localized = true
    return { ...field, message: requiredMessage }
  })
  return { fields, message: localized ? fields.map((field) => field.message).join(', ') : error.message, localized }
}
