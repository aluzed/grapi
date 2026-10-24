import type { Attribute } from '@/lib/types'
import { pack } from './ConfigureView'

type Cell = { name: string; size: number }

// The edit view groups the fields by kind: short fields on a grid, long
// texts and structures one per row, and what qualifies the entry (relations,
// a single media, switches) beside the form.
export type Section = 'general' | 'content' | 'structure' | 'media' | 'advanced' | 'relations' | 'cover' | 'options'

export function sectionOf(attr: Attribute): Section {
  switch (attr.type) {
    case 'richtext':
    case 'blocks':
      return 'content'
    case 'component':
    case 'dynamiczone':
      return 'structure'
    case 'media':
      return attr.multiple ? 'media' : 'cover'
    case 'json':
      return 'advanced'
    case 'relation':
      return 'relations'
    case 'boolean':
      return 'options'
  }
  return 'general'
}

// splitLayout keeps the configured order and widths: the general fields are
// packed again once the others are out of their rows, the last field of a row
// widens to its end, a text takes a full row, every other field takes its own.
export function splitLayout(layout: Cell[][], attrs: Record<string, Attribute>): Record<Section, Cell[][]> {
  const out: Record<Section, Cell[][]> = { general: [], content: [], structure: [], media: [], advanced: [], relations: [], cover: [], options: [] }
  const general: Cell[] = []
  for (const cell of layout.flat()) {
    const attr = attrs[cell.name]
    if (!attr) continue
    const section = sectionOf(attr)
    if (section === 'general') general.push(attr.type === 'text' ? { name: cell.name, size: 12 } : cell)
    else out[section].push([{ name: cell.name, size: 12 }])
  }
  out.general = pack(general).map((row) => {
    const used = row.reduce((n, c) => n + c.size, 0)
    return row.map((c, i) => (i === row.length - 1 ? { ...c, size: c.size + 12 - used } : c))
  })
  return out
}
