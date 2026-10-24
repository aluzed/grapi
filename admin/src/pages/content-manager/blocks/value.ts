import type { Descendant } from 'slate'

// blocksItems is the form value of a blocks attribute: Strapi's JSON as is.
export const blocksItems = (v: unknown): Descendant[] | null => (Array.isArray(v) && v.length ? (v as Descendant[]) : null)
