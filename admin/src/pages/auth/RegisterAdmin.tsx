import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { UserPlus } from 'lucide-react'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { TextField } from '@/components/ui/field'
import { publicRequest } from '@/lib/api'
import { showError } from '@/lib/form'
import { emailSchema, passwordSchema } from '@/lib/password'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'
import { AuthLayout } from './AuthLayout'

export function RegisterAdmin() {
  const { t } = useI18n()
  const { signIn } = useAuth()
  const schema = z
    .object({
      firstname: z.string().trim().min(1, t('validation.required')),
      lastname: z.string(),
      email: emailSchema(t),
      password: passwordSchema(t),
      confirm: z.string(),
    })
    .refine((v) => v.password === v.confirm, { path: ['confirm'], message: t('validation.match') })
  type Values = z.infer<typeof schema>
  const { register, handleSubmit, formState, setError } = useForm<Values>({ resolver: zodResolver(schema) })
  const submit = handleSubmit(async ({ confirm: _, ...body }) => {
    try {
      await signIn(await publicRequest<{ token: string }>('POST', '/auth/register-admin', body))
    } catch (err) {
      showError(err, setError, t('common.error'))
    }
  })
  return (
    <AuthLayout title={t('auth.register.title')} subtitle={t('auth.register.subtitle')}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <TextField label={t('auth.firstname')} autoComplete="given-name" autoFocus error={formState.errors.firstname?.message} {...register('firstname')} />
          <TextField label={t('auth.lastname')} autoComplete="family-name" error={formState.errors.lastname?.message} {...register('lastname')} />
        </div>
        <TextField label={t('auth.email')} type="email" autoComplete="email" error={formState.errors.email?.message} {...register('email')} />
        <TextField label={t('auth.password')} type="password" autoComplete="new-password" hint={t('validation.password')} error={formState.errors.password?.message} {...register('password')} />
        <TextField label={t('auth.confirmPassword')} type="password" autoComplete="new-password" error={formState.errors.confirm?.message} {...register('confirm')} />
        <Button type="submit" title={t('auth.register.submit')} icon={<UserPlus className="h-4 w-4" />} loading={formState.isSubmitting} className="w-full">
          {t('auth.register.submit')}
        </Button>
      </form>
    </AuthLayout>
  )
}
