import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { LogIn } from 'lucide-react'
import { Link } from 'react-router'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { Checkbox, TextField } from '@/components/ui/field'
import { publicRequest } from '@/lib/api'
import { showError } from '@/lib/form'
import { emailSchema } from '@/lib/password'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'
import { AuthLayout } from './AuthLayout'

export function Login() {
  const { t } = useI18n()
  const { signIn } = useAuth()
  const schema = z.object({ email: emailSchema(t), password: z.string().min(1, t('validation.required')), rememberMe: z.boolean() })
  type Values = z.infer<typeof schema>
  const { register, handleSubmit, formState, setError } = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { rememberMe: false } })
  const submit = handleSubmit(async (v) => {
    try {
      await signIn(await publicRequest<{ token: string }>('POST', '/auth/login', v))
    } catch (err) {
      showError(err, setError, t('common.error'))
    }
  })
  return (
    <AuthLayout title={t('auth.login.title')}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <TextField label={t('auth.email')} type="email" autoComplete="email" autoFocus error={formState.errors.email?.message} {...register('email')} />
        <TextField label={t('auth.password')} type="password" autoComplete="current-password" error={formState.errors.password?.message} {...register('password')} />
        <Checkbox label={t('auth.rememberMe')} {...register('rememberMe')} />
        <Button type="submit" title={t('auth.login.submit')} icon={<LogIn className="h-4 w-4" />} loading={formState.isSubmitting} className="w-full">
          {t('auth.login.submit')}
        </Button>
        <Link to="/auth/forgot-password" className="text-center text-sm text-primary underline-offset-4 hover:underline">
          {t('auth.login.forgot')}
        </Link>
      </form>
    </AuthLayout>
  )
}
