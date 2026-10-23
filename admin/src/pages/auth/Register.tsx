import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { UserCheck } from 'lucide-react'
import { useSearchParams } from 'react-router'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { TextField } from '@/components/ui/field'
import { publicRequest } from '@/lib/api'
import { showError } from '@/lib/form'
import { passwordSchema } from '@/lib/password'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'
import { AuthLayout } from './AuthLayout'

type Info = { email: string; firstname: string; lastname: string }

export function Register() {
  const { t } = useI18n()
  const { signIn } = useAuth()
  const [params] = useSearchParams()
  const token = params.get('registrationToken') ?? ''
  const info = useQuery({
    queryKey: ['registration-info', token],
    queryFn: () => publicRequest<Info>('GET', `/auth/registration-info?registrationToken=${encodeURIComponent(token)}`),
    enabled: token !== '',
    retry: false,
  })
  const schema = z
    .object({ firstname: z.string().trim().min(1, t('validation.required')), lastname: z.string(), password: passwordSchema(t), confirm: z.string() })
    .refine((v) => v.password === v.confirm, { path: ['confirm'], message: t('validation.match') })
  type Values = z.infer<typeof schema>
  const { register, handleSubmit, formState, setError, reset } = useForm<Values>({ resolver: zodResolver(schema) })
  useEffect(() => {
    if (info.data) reset({ firstname: info.data.firstname ?? '', lastname: info.data.lastname ?? '', password: '', confirm: '' })
  }, [info.data, reset])
  const submit = handleSubmit(async ({ confirm: _, ...userInfo }) => {
    try {
      await signIn(await publicRequest<{ token: string }>('POST', '/auth/register', { registrationToken: token, userInfo }))
    } catch (err) {
      showError(err, setError, t('common.error'))
    }
  })
  if (token === '' || info.isError) {
    return (
      <AuthLayout title={t('auth.invite.title')}>
        <p className="text-sm text-destructive">{t('auth.invite.invalid')}</p>
      </AuthLayout>
    )
  }
  return (
    <AuthLayout title={t('auth.invite.title')} subtitle={t('auth.invite.subtitle')}>
      {info.isPending ? (
        <p className="text-sm text-muted">{t('common.loading')}</p>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
          <TextField label={t('auth.email')} value={info.data?.email ?? ''} readOnly disabled />
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <TextField label={t('auth.firstname')} autoComplete="given-name" error={formState.errors.firstname?.message} {...register('firstname')} />
            <TextField label={t('auth.lastname')} autoComplete="family-name" {...register('lastname')} />
          </div>
          <TextField label={t('auth.password')} type="password" autoComplete="new-password" hint={t('validation.password')} error={formState.errors.password?.message} {...register('password')} />
          <TextField label={t('auth.confirmPassword')} type="password" autoComplete="new-password" error={formState.errors.confirm?.message} {...register('confirm')} />
          <Button type="submit" title={t('auth.invite.submit')} icon={<UserCheck className="h-4 w-4" />} loading={formState.isSubmitting} className="w-full">
            {t('auth.invite.submit')}
          </Button>
        </form>
      )}
    </AuthLayout>
  )
}
