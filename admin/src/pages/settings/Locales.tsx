import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { DialogActions } from '@/components/DialogActions'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { EntityActionsMenu } from '@/components/ui/entity-actions-menu'
import { Checkbox, SelectField, TextField } from '@/components/ui/field'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { api } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import type { Locale } from '@/lib/types'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'

function LocaleDialog({ locale, existing, onClose }: { locale: Locale | null; existing: Locale[]; onClose: () => void }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const iso = useQuery({ queryKey: ['i18n', 'iso'], queryFn: () => api.get<{ code: string; name: string }[]>('/i18n/iso-locales'), enabled: locale === null, staleTime: Infinity })
  const used = new Set(existing.map((l) => l.code))
  const [code, setCode] = useState('')
  const [name, setName] = useState(locale?.name ?? '')
  const [isDefault, setIsDefault] = useState(locale?.isDefault ?? false)
  const save = useMutation({
    mutationFn: () => (locale ? api.put(`/i18n/locales/${locale.id}`, { name, isDefault }) : api.post('/i18n/locales', { code, name: name || undefined, isDefault })),
    onSuccess: () => {
      toast.success(locale ? t('locales.saved') : t('locales.created'))
      void queryClient.invalidateQueries({ queryKey: ['cm', 'locales'] })
      onClose()
    },
    onError: (err) => showError(err, undefined, t('common.error')),
  })
  return (
    <Dialog
      open
      title={locale ? locale.name : t('locales.add')}
      onClose={onClose}
      closeLabel={t('common.cancel')}
      footer={<DialogActions onCancel={onClose} onConfirm={() => save.mutate()} loading={save.isPending} disabled={!locale && !code} />}
    >
      <div className="flex flex-col gap-4">
        {!locale && (
          <SelectField
            label={t('locales.code')}
            value={code}
            onChange={(e) => {
              setCode(e.target.value)
              setName(iso.data?.find((l) => l.code === e.target.value)?.name ?? '')
            }}
          >
            <option value="">-</option>
            {iso.data
              ?.filter((l) => !used.has(l.code))
              .map((l) => (
                <option key={l.code} value={l.code}>
                  {l.name}
                </option>
              ))}
          </SelectField>
        )}
        <TextField label={t('locales.name')} value={name} onChange={(e) => setName(e.target.value)} />
        {!locale?.isDefault && <Checkbox label={t('locales.makeDefault')} checked={isDefault} onChange={(e) => setIsDefault(e.target.checked)} />}
      </div>
    </Dialog>
  )
}

export function Locales() {
  const { t } = useI18n()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const { permissions } = useAuth()
  const [editing, setEditing] = useState<Locale | null | undefined>(undefined)
  const locales = useQuery({ queryKey: ['cm', 'locales'], queryFn: () => api.get<Locale[]>('/content-manager/locales') })
  const remove = useMutation({
    mutationFn: (l: Locale) => api.del(`/i18n/locales/${l.id}`),
    onSuccess: () => {
      toast.success(t('locales.deleted'))
      void queryClient.invalidateQueries({ queryKey: ['cm'] })
    },
    onError: (err) => showError(err, undefined, t('common.error')),
  })
  const list = locales.data ?? []
  const actions = (l: Locale) => {
    const out = []
    if (can(permissions, 'plugin::i18n.locale.update')) out.push({ label: t('locales.edit'), icon: <Pencil className="h-4 w-4" />, onSelect: () => setEditing(l) })
    if (can(permissions, 'plugin::i18n.locale.delete') && !l.isDefault)
      out.push({
        label: t('locales.delete'),
        icon: <Trash2 className="h-4 w-4" />,
        destructive: true,
        onSelect: async () => {
          if (await confirm({ title: t('locales.delete'), message: t('locales.deleteConfirm', { name: l.name }), confirmLabel: t('locales.delete'), destructive: true })) remove.mutate(l)
        },
      })
    return out
  }
  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t('settings.locales')}
        actions={
          can(permissions, 'plugin::i18n.locale.create') && (
            <Button title={t('locales.add')} icon={<Plus className="h-4 w-4" />} onClick={() => setEditing(null)} className="w-full md:w-auto">
              {t('locales.add')}
            </Button>
          )
        }
      />
      <div className="hidden md:block">
        <Table>
          <thead>
            <tr>
              <Th>{t('locales.name')}</Th>
              <Th>{t('locales.code')}</Th>
              <Th>{t('locales.default')}</Th>
              <Th className="w-16">
                <span className="sr-only">{t('cm.actions')}</span>
              </Th>
            </tr>
          </thead>
          <tbody>
            {list.map((l) => (
              <Tr key={l.code}>
                <Td className="font-medium">{l.name}</Td>
                <Td>{l.code}</Td>
                <Td>{l.isDefault ? t('locales.isDefault') : ''}</Td>
                <Td className="text-right">{actions(l).length > 0 && <EntityActionsMenu actions={actions(l)} label={t('cm.actions')} />}</Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </div>
      <ul className="flex flex-col gap-3 md:hidden">
        {list.map((l) => (
          <li key={l.code} className="flex items-start justify-between gap-3 rounded-lg border border-border bg-surface p-4 shadow-sm">
            <div>
              <p className="font-medium">{l.name}</p>
              <p className="text-sm text-muted">
                {l.code}
                {l.isDefault ? ` · ${t('locales.isDefault')}` : ''}
              </p>
            </div>
            {actions(l).length > 0 && <EntityActionsMenu actions={actions(l)} label={t('cm.actions')} />}
          </li>
        ))}
      </ul>
      {editing !== undefined && <LocaleDialog locale={editing} existing={list} onClose={() => setEditing(undefined)} />}
    </div>
  )
}
