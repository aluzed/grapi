import { PageHeader } from '@/app/PageHeader'
import { Card } from '@/components/ui/card'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'

export function Overview() {
  const { t } = useI18n()
  const { information: info } = useAuth()
  const rows: [string, string][] = [
    [t('settings.overview.version'), info?.version ?? '-'],
    [t('settings.overview.edition'), t('settings.overview.editionValue')],
    [t('settings.overview.go'), info?.goVersion ?? '-'],
    [t('settings.overview.database'), info?.database ?? '-'],
    [t('settings.overview.environment'), info?.environment ?? '-'],
    [t('settings.overview.contentTypes'), String(info?.contentTypes ?? '-')],
  ]
  return (
    <div className="flex flex-col gap-4">
      <PageHeader title={t('settings.overview')} />
      <Card>
        <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {rows.map(([k, v]) => (
            <div key={k}>
              <dt className="text-xs font-semibold uppercase tracking-wide text-muted">{k}</dt>
              <dd className="mt-1 text-sm">{v}</dd>
            </div>
          ))}
        </dl>
      </Card>
    </div>
  )
}
