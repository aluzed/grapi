import type { Attribute } from '@/lib/types'
import { isEditable } from './fields'

// The server's rule: the localized option, any relation or a uid. Any other
// field is shared by every locale of the document.
export function isLocalizedAttribute(attr: Attribute): boolean {
  return attr.pluginOptions?.i18n?.localized === true || attr.type === 'relation' || attr.type === 'uid'
}

// The fields "fill from another locale" copies: the localized ones the user
// may change.
export function fillableFields(attrs: Record<string, Attribute>, fields: string[], writable: (name: string) => boolean): string[] {
  return fields.filter((name) => {
    const attr = attrs[name]
    return attr !== undefined && isEditable(attr) && attr.type !== 'password' && isLocalizedAttribute(attr) && writable(name)
  })
}
