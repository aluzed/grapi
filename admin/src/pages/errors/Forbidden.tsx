import { House, ShieldX } from 'lucide-react'
import { useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { useI18n } from '@/providers/I18nProvider'

export function ErrorPage({ title, text }: { title: string; text: string }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  return (
    <div className="mx-auto flex max-w-md flex-col items-center gap-4 py-16 text-center">
      <ShieldX className="h-10 w-10 text-muted" aria-hidden />
      <h1 className="text-xl font-semibold">{title}</h1>
      <p className="text-sm text-muted">{text}</p>
      <Button variant="outline" title={t('errors.home')} icon={<House className="h-4 w-4" />} onClick={() => navigate('/')} className="w-full md:w-auto">
        {t('errors.home')}
      </Button>
    </div>
  )
}

export function Forbidden() {
  const { t } = useI18n()
  return <ErrorPage title={t('errors.forbidden.title')} text={t('errors.forbidden.text')} />
}

export function NotFound() {
  const { t } = useI18n()
  return <ErrorPage title={t('errors.notFound.title')} text={t('errors.notFound.text')} />
}
