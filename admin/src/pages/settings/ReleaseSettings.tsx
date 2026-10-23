import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { SelectField } from '@/components/ui/field'
import { api } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import { timeZones } from '@/lib/tz'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'

export type ReleaseSettingsData = { defaultTimezone: string | null }

export function useReleaseSettings(enabled = true) {
  return useQuery({ queryKey: ['releases', 'settings'], queryFn: () => api.get<ReleaseSettingsData>('/releases/settings'), enabled, staleTime: 60_000 })
}

export function ReleaseSettings() {
  const { t } = useI18n()
  const { permissions } = useAuth()
  const queryClient = useQueryClient()
  const settings = useReleaseSettings()
  const [timezone, setTimezone] = useState('')
  useEffect(() => setTimezone(settings.data?.defaultTimezone ?? ''), [settings.data])
  const editable = can(permissions, 'admin::releases.settings.update')
  const save = useMutation({
    mutationFn: () => api.put('/releases/settings', { defaultTimezone: timezone || null }),
    onSuccess: () => {
      toast.success(t('releases.settingsSaved'))
      void queryClient.invalidateQueries({ queryKey: ['releases', 'settings'] })
    },
    onError: (e) => showError(e, undefined, t('common.error')),
  })
  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t('settings.releases')}
        subtitle={t('releases.settingsHelp')}
        actions={
          editable && (
            <Button title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={save.isPending} onClick={() => save.mutate()} className="w-full md:w-auto">
              {t('common.save')}
            </Button>
          )
        }
      />
      <Card>
        <SelectField label={t('releases.defaultTimezone')} value={timezone} disabled={!editable} onChange={(e) => setTimezone(e.target.value)}>
          <option value="">{t('releases.browserTimezone')}</option>
          {timeZones().map((z) => (
            <option key={z} value={z}>
              {z}
            </option>
          ))}
        </SelectField>
      </Card>
    </div>
  )
}
