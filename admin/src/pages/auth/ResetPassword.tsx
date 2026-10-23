import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { KeyRound } from 'lucide-react'
import { Link, useSearchParams } from 'react-router'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { TextField } from '@/components/ui/field'
import { publicRequest } from '@/lib/api'
import { showError } from '@/lib/form'
import { passwordSchema } from '@/lib/password'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'
import { AuthLayout } from './AuthLayout'

export function ResetPassword() {
  const { t } = useI18n()
  const { signIn } = useAuth()
  const [params] = useSearchParams()
  const code = params.get('code') ?? ''
  const schema = z
    .object({ password: passwordSchema(t), confirm: z.string() })
    .refine((v) => v.password === v.confirm, { path: ['confirm'], message: t('validation.match') })
  const { register, handleSubmit, formState, setError } = useForm<z.infer<typeof schema>>({ resolver: zodResolver(schema) })
  const submit = handleSubmit(async ({ password }) => {
    try {
      await signIn(await publicRequest<{ token: string }>('POST', '/auth/reset-password', { resetPasswordToken: code, password }))
    } catch (err) {
      showError(err, setError, t('common.error'))
    }
  })
  return (
    <AuthLayout title={t('auth.reset.title')}>
      {code === '' ? (
        <p className="text-sm text-destructive">{t('auth.reset.missing')}</p>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
          <TextField label={t('profile.newPassword')} type="password" autoComplete="new-password" autoFocus hint={t('validation.password')} error={formState.errors.password?.message} {...register('password')} />
          <TextField label={t('auth.confirmPassword')} type="password" autoComplete="new-password" error={formState.errors.confirm?.message} {...register('confirm')} />
          <Button type="submit" title={t('auth.reset.submit')} icon={<KeyRound className="h-4 w-4" />} loading={formState.isSubmitting} className="w-full">
            {t('auth.reset.submit')}
          </Button>
        </form>
      )}
      <Link to="/auth/forgot-password" className="mt-4 block text-center text-sm text-primary underline-offset-4 hover:underline">
        {t('auth.forgot.title')}
      </Link>
    </AuthLayout>
  )
}
