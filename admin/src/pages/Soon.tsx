import { Construction } from 'lucide-react'
import { PageHeader } from '@/app/PageHeader'
import { Card } from '@/components/ui/card'
import type { MessageKey } from '@/i18n'
import { useI18n } from '@/providers/I18nProvider'

export function Soon({ title }: { title: MessageKey }) {
  const { t } = useI18n()
  return (
    <div className="mx-auto max-w-5xl">
      <PageHeader title={t(title)} />
      <Card className="flex items-center gap-3 text-sm text-muted">
        <Construction className="h-5 w-5 shrink-0" aria-hidden />
        {t('page.soon')}
      </Card>
    </div>
  )
}
