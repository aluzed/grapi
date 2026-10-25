import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/cn'
import { useI18n } from '@/providers/I18nProvider'

export function Pager({ page, pageCount, onPage, className }: { page: number; pageCount: number; onPage: (page: number) => void; className?: string }) {
  const { t } = useI18n()
  return (
    <div className={cn('flex flex-col gap-2 md:flex-row md:items-center', className)}>
      <span className="text-center text-sm text-muted">{t('cm.page', { page: String(page), pages: String(pageCount) })}</span>
      <Button variant="outline" title={t('cm.previous')} icon={<ChevronLeft className="h-4 w-4" />} disabled={page <= 1} onClick={() => onPage(page - 1)} className="w-full md:w-auto">
        {t('cm.previous')}
      </Button>
      <Button variant="outline" title={t('cm.next')} icon={<ChevronRight className="h-4 w-4" />} disabled={page >= pageCount} onClick={() => onPage(page + 1)} className="w-full md:w-auto">
        {t('cm.next')}
      </Button>
    </div>
  )
}
