import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Send } from 'lucide-react'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Card, CardHeader } from '@/components/ui/card'
import { TextField } from '@/components/ui/field'
import { api } from '@/lib/api'
import { showError } from '@/lib/form'
import { useI18n } from '@/providers/I18nProvider'

type Settings = { provider: string; configured: boolean; defaultFrom: string; defaultReplyTo: string }

export function Email() {
  const { t } = useI18n()
  const settings = useQuery({ queryKey: ['email', 'settings'], queryFn: () => api.get<Settings>('/email/settings') })
  const [to, setTo] = useState('')
  const send = useMutation({
    mutationFn: () => api.post('/email/test', { to }),
    onSuccess: () => toast.success(t('email.sent', { to })),
    onError: (e) => showError(e, undefined, t('common.error')),
  })
  const s = settings.data
  return (
    <div className="flex flex-col gap-4">
      <PageHeader title={t('settings.email')} subtitle={t('email.configHelp')} />
      <Card>
        <dl className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          {[
            [t('email.provider'), s?.provider],
            [t('email.defaultFrom'), s?.defaultFrom],
            [t('email.defaultReplyTo'), s?.defaultReplyTo],
          ].map(([k, v]) => (
            <div key={k}>
              <dt className="text-xs font-semibold uppercase tracking-wide text-muted">{k}</dt>
              <dd className="mt-1 break-all text-sm">{v || '-'}</dd>
            </div>
          ))}
        </dl>
        {s && !s.configured && <p className="mt-4 text-sm text-destructive">{t('email.notConfigured')}</p>}
      </Card>
      <Card>
        <CardHeader title={t('email.test')} />
        <form
          className="flex flex-col gap-3 md:flex-row md:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            send.mutate()
          }}
        >
          <TextField label={t('email.to')} type="email" value={to} onChange={(e) => setTo(e.target.value)} className="flex-1" />
          <Button type="submit" title={t('email.send')} icon={<Send className="h-4 w-4" />} loading={send.isPending} disabled={!to} className="w-full md:w-auto">
            {t('email.send')}
          </Button>
        </form>
      </Card>
    </div>
  )
}
