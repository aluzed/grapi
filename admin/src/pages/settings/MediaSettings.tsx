import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Checkbox, TextField } from '@/components/ui/field'
import { api } from '@/lib/api'
import { showError } from '@/lib/form'
import { setUploadOrigin } from '@/lib/media'
import type { MessageKey } from '@/i18n'
import { useI18n } from '@/providers/I18nProvider'

type UploadSettings = {
  sizeOptimization: boolean
  responsiveDimensions: boolean
  autoOrientation: boolean
  publicUrl: string
  publicUrlDefault?: string
}

const keys = ['responsiveDimensions', 'sizeOptimization', 'autoOrientation'] as const

export function MediaSettings() {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const settings = useQuery({ queryKey: ['media', 'settings'], queryFn: () => api.get<UploadSettings>('/upload/settings') })
  const [value, setValue] = useState<UploadSettings | null>(null)
  useEffect(() => {
    if (settings.data) setValue(settings.data)
  }, [settings.data])
  const save = useMutation({
    mutationFn: () => api.put<UploadSettings>('/upload/settings', value),
    onSuccess: (saved) => {
      setUploadOrigin(saved.publicUrl || saved.publicUrlDefault || '')
      toast.success(t('mediaSettings.saved'))
      void queryClient.invalidateQueries({ queryKey: ['media', 'settings'] })
    },
    onError: (e) => showError(e, undefined, t('common.error')),
  })
  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t('settings.media')}
        subtitle={t('mediaSettings.help')}
        actions={
          <Button title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={save.isPending} disabled={!value} onClick={() => save.mutate()} className="w-full md:w-auto">
            {t('common.save')}
          </Button>
        }
      />
      <Card className="flex flex-col gap-4">
        {value &&
          keys.map((k) => (
            <div key={k} className="flex flex-col gap-1">
              <Checkbox label={t(`mediaSettings.${k}` as MessageKey)} checked={value[k]} onChange={(e) => setValue({ ...value, [k]: e.target.checked })} />
              <p className="pl-6 text-xs text-muted">{t(`mediaSettings.${k}Help` as MessageKey)}</p>
            </div>
          ))}
        {value && (
          <TextField
            label={t('mediaSettings.publicUrl')}
            type="url"
            inputMode="url"
            value={value.publicUrl}
            placeholder={value.publicUrlDefault || window.location.origin}
            hint={t('mediaSettings.publicUrlHelp')}
            onChange={(e) => setValue({ ...value, publicUrl: e.target.value })}
          />
        )}
      </Card>
    </div>
  )
}
