import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { fieldLabel } from '@/lib/humanize'
import type { Locale, Model } from '@/lib/types'

export const cmBase = '/content-manager/collection-types'

export function useModels() {
  return useQuery({ queryKey: ['cm', 'models'], queryFn: () => api.get<Model[]>('/content-manager/models'), staleTime: 60_000 })
}

export function useLocales(enabled: boolean) {
  return useQuery({ queryKey: ['cm', 'locales'], queryFn: () => api.get<Locale[]>('/content-manager/locales'), enabled, staleTime: 60_000 })
}

export const entryPath = (uid: string, documentId?: string) => `${cmBase}/${encodeURIComponent(uid)}${documentId ? `/${encodeURIComponent(documentId)}` : ''}`

export function label(model: Model, field: string): string {
  return fieldLabel(field, model.labels[field])
}
