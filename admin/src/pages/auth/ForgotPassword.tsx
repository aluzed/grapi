import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Mail } from 'lucide-react'
import { Link } from 'react-router'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { TextField } from '@/components/ui/field'
import { publicRequest } from '@/lib/api'
import { showError } from '@/lib/form'
import { emailSchema } from '@/lib/password'
import { useI18n } from '@/providers/I18nProvider'
import { AuthLayout } from './AuthLayout'

export function ForgotPassword() {
  const { t } = useI18n()
  const [sent, setSent] = useState(false)
  const schema = z.object({ email: emailSchema(t) })
  const { register, handleSubmit, formState, setError } = useForm<z.infer<typeof schema>>({ resolver: zodResolver(schema) })
  const submit = handleSubmit(async (v) => {
    try {
      await publicRequest('POST', '/auth/forgot-password', v)
      setSent(true)
    } catch (err) {
      showError(err, setError, t('common.error'))
    }
  })
  return (
    <AuthLayout title={t('auth.forgot.title')} subtitle={sent ? undefined : t('auth.forgot.subtitle')}>
      {sent ? (
        <p role="status" className="rounded-md bg-success/10 p-3 text-sm text-success">
          {t('auth.forgot.sent')}
        </p>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
          <TextField label={t('auth.email')} type="email" autoComplete="email" autoFocus error={formState.errors.email?.message} {...register('email')} />
          <Button type="submit" title={t('auth.forgot.submit')} icon={<Mail className="h-4 w-4" />} loading={formState.isSubmitting} className="w-full">
            {t('auth.forgot.submit')}
          </Button>
        </form>
      )}
      <Link to="/auth/login" className="mt-4 block text-center text-sm text-primary underline-offset-4 hover:underline">
        {t('auth.forgot.back')}
      </Link>
    </AuthLayout>
  )
}
