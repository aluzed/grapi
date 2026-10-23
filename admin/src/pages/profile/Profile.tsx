import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { KeyRound, Laptop, Save, ShieldOff, Trash2 } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { z } from 'zod'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Card, CardHeader } from '@/components/ui/card'
import { SelectField, TextField } from '@/components/ui/field'
import { api } from '@/lib/api'
import { showError } from '@/lib/form'
import { emailSchema, passwordSchema } from '@/lib/password'
import type { AdminSession } from '@/lib/types'
import { languages, type Language } from '@/i18n'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'

function Details() {
  const { t, language, setLanguage } = useI18n()
  const { user, reload } = useAuth()
  const schema = z.object({
    firstname: z.string().trim().min(1, t('validation.required')),
    lastname: z.string(),
    username: z.string(),
    email: emailSchema(t),
    preferedLanguage: z.enum(['en', 'fr']),
  })
  type Values = z.infer<typeof schema>
  const { register, handleSubmit, formState, setError, reset } = useForm<Values>({ resolver: zodResolver(schema) })
  useEffect(() => {
    if (user)
      reset({
        firstname: user.firstname ?? '',
        lastname: user.lastname ?? '',
        username: user.username ?? '',
        email: user.email,
        preferedLanguage: (languages as string[]).includes(user.preferedLanguage) ? (user.preferedLanguage as Language) : language,
      })
  }, [user, reset, language])
  const submit = handleSubmit(async (v) => {
    try {
      await api.put('/users/me', v)
      setLanguage(v.preferedLanguage)
      await reload()
      toast.success(t('common.saved'))
    } catch (err) {
      showError(err, setError, t('common.error'))
    }
  })
  return (
    <Card>
      <CardHeader title={t('profile.details')} />
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <TextField label={t('auth.firstname')} autoComplete="given-name" error={formState.errors.firstname?.message} {...register('firstname')} />
          <TextField label={t('auth.lastname')} autoComplete="family-name" {...register('lastname')} />
          <TextField label={t('auth.email')} type="email" autoComplete="email" error={formState.errors.email?.message} {...register('email')} />
          <TextField label={t('profile.username')} autoComplete="username" {...register('username')} />
          <SelectField label={t('profile.language')} {...register('preferedLanguage')}>
            {languages.map((l) => (
              <option key={l} value={l}>
                {t(`language.${l}`)}
              </option>
            ))}
          </SelectField>
        </div>
        <div className="flex flex-col md:flex-row md:justify-end">
          <Button type="submit" title={t('common.save')} icon={<Save className="h-4 w-4" />} loading={formState.isSubmitting} className="w-full md:w-auto">
            {t('common.save')}
          </Button>
        </div>
      </form>
    </Card>
  )
}

function Password() {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const schema = z
    .object({ currentPassword: z.string().min(1, t('validation.required')), password: passwordSchema(t), confirm: z.string() })
    .refine((v) => v.password === v.confirm, { path: ['confirm'], message: t('validation.match') })
  type Values = z.infer<typeof schema>
  const { register, handleSubmit, formState, setError, reset } = useForm<Values>({ resolver: zodResolver(schema) })
  const submit = handleSubmit(async ({ currentPassword, password }) => {
    try {
      await api.put('/users/me', { currentPassword, password })
      reset({ currentPassword: '', password: '', confirm: '' })
      await queryClient.invalidateQueries({ queryKey: ['me', 'sessions'] })
      toast.success(t('profile.passwordChanged'))
    } catch (err) {
      showError(err, setError, t('common.error'))
    }
  })
  return (
    <Card>
      <CardHeader title={t('profile.password')} />
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <TextField label={t('profile.currentPassword')} type="password" autoComplete="current-password" error={formState.errors.currentPassword?.message} {...register('currentPassword')} />
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <TextField label={t('profile.newPassword')} type="password" autoComplete="new-password" hint={t('validation.password')} error={formState.errors.password?.message} {...register('password')} />
          <TextField label={t('auth.confirmPassword')} type="password" autoComplete="new-password" error={formState.errors.confirm?.message} {...register('confirm')} />
        </div>
        <div className="flex flex-col md:flex-row md:justify-end">
          <Button type="submit" title={t('profile.password')} icon={<KeyRound className="h-4 w-4" />} loading={formState.isSubmitting} className="w-full md:w-auto">
            {t('profile.password')}
          </Button>
        </div>
      </form>
    </Card>
  )
}

function device(s: AdminSession): string {
  const ua = s.metadata?.userAgent ?? ''
  const browser = /Firefox\//.test(ua) ? 'Firefox' : /Edg\//.test(ua) ? 'Edge' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : ''
  const os = /iPhone|iPad/.test(ua) ? 'iOS' : /Android/.test(ua) ? 'Android' : /Mac OS X/.test(ua) ? 'macOS' : /Windows/.test(ua) ? 'Windows' : /Linux/.test(ua) ? 'Linux' : ''
  return [browser, os].filter(Boolean).join(' · ') || ua || s.deviceId.slice(0, 8)
}

function Sessions() {
  const { t, language } = useI18n()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const sessions = useQuery({ queryKey: ['me', 'sessions'], queryFn: () => api.get<AdminSession[]>('/users/me/sessions') })
  const refreshList = () => queryClient.invalidateQueries({ queryKey: ['me', 'sessions'] })
  const revoke = useMutation({
    mutationFn: (id: string) => api.del(`/users/me/sessions/${encodeURIComponent(id)}`),
    onSuccess: () => {
      toast.success(t('profile.sessions.revoked'))
      void refreshList()
    },
    onError: (err) => showError(err, undefined, t('common.error')),
  })
  const revokeOthers = useMutation({
    mutationFn: () => api.del('/users/me/sessions'),
    onSuccess: () => {
      toast.success(t('profile.sessions.revoked'))
      void refreshList()
    },
    onError: (err) => showError(err, undefined, t('common.error')),
  })
  const list = sessions.data ?? []
  const others = list.filter((s) => !s.isCurrent)
  const date = (iso: string) => new Date(iso).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'short' })
  return (
    <Card>
      <CardHeader
        title={t('profile.sessions')}
        subtitle={t('profile.sessions.subtitle')}
        actions={
          others.length > 0 && (
            <Button
              variant="destructive-outline"
              title={t('profile.sessions.revokeOthers')}
              icon={<ShieldOff className="h-4 w-4" />}
              loading={revokeOthers.isPending}
              className="w-full md:w-auto"
              onClick={async () => {
                if (await confirm({ title: t('profile.sessions.revokeOthers'), message: t('profile.sessions.revokeOthersConfirm'), confirmLabel: t('profile.sessions.revoke'), destructive: true }))
                  revokeOthers.mutate()
              }}
            >
              {t('profile.sessions.revokeOthers')}
            </Button>
          )
        }
      />
      {sessions.isPending ? (
        <p className="text-sm text-muted">{t('common.loading')}</p>
      ) : (
        <ul className="flex flex-col divide-y divide-border">
          {list.map((s) => (
            <li key={s.id} className="flex flex-col gap-3 py-3 md:flex-row md:items-center md:justify-between">
              <div className="flex min-w-0 items-start gap-3">
                <Laptop className="mt-0.5 h-5 w-5 shrink-0 text-muted" aria-hidden />
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">
                    {device(s)}
                    {s.isCurrent && <span className="ml-2 rounded bg-success/10 px-1.5 py-0.5 text-xs text-success">{t('profile.sessions.current')}</span>}
                  </p>
                  <p className="text-xs text-muted">
                    {t('profile.sessions.since', { date: date(s.metadata?.loginAt ?? s.createdAt) })}
                    {' · '}
                    {s.type === 'refresh' ? t('profile.sessions.remembered') : t('profile.sessions.browser')}
                  </p>
                </div>
              </div>
              {!s.isCurrent && (
                <Button
                  variant="destructive-outline"
                  title={t('profile.sessions.revoke')}
                  icon={<Trash2 className="h-4 w-4" />}
                  className="w-full md:w-auto"
                  onClick={async () => {
                    if (await confirm({ title: t('profile.sessions.revoke'), message: t('profile.sessions.revokeConfirm'), confirmLabel: t('profile.sessions.revoke'), destructive: true }))
                      revoke.mutate(s.id)
                  }}
                >
                  {t('profile.sessions.revoke')}
                </Button>
              )}
            </li>
          ))}
          {others.length === 0 && <li className="py-3 text-sm text-muted">{t('profile.sessions.none')}</li>}
        </ul>
      )}
    </Card>
  )
}

export function Profile() {
  const { t } = useI18n()
  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-6">
      <PageHeader title={t('profile.title')} />
      <Details />
      <Password />
      <Sessions />
    </div>
  )
}
