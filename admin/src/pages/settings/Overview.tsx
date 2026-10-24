import { useRef } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ImagePlus, RotateCcw } from 'lucide-react'
import { toast } from 'sonner'
import { PageHeader } from '@/app/PageHeader'
import { Button } from '@/components/ui/button'
import { Card, CardHeader } from '@/components/ui/card'
import { api, upload } from '@/lib/api'
import { showError } from '@/lib/form'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'

type LogoFile = { name: string; url: string; width: number | null; height: number | null; ext: string; size: number }
type ProjectSettings = { menuLogo: LogoFile | null; authLogo: LogoFile | null }

function LogoSetting({ input, file, canUpdate }: { input: 'menuLogo' | 'authLogo'; file: LogoFile | null; canUpdate: boolean }) {
  const { t } = useI18n()
  const confirm = useConfirm()
  const { reloadLogos } = useAuth()
  const queryClient = useQueryClient()
  const picker = useRef<HTMLInputElement>(null)
  const send = useMutation({
    mutationFn: (f: File | null) => {
      const form = new FormData()
      // A text value resets the logo, as in Strapi.
      if (f) form.append(input, f)
      else form.append(input, '')
      return upload('/project-settings', form)
    },
    onSuccess: () => {
      toast.success(t('settings.logo.saved'))
      void queryClient.invalidateQueries({ queryKey: ['project-settings'] })
      void reloadLogos()
    },
    onError: (e) => showError(e),
  })
  return (
    <div className="flex flex-col gap-3 rounded-md border border-border p-4">
      <div>
        <p className="text-sm font-medium">{t(`settings.logo.${input}`)}</p>
        <p className="text-xs text-muted">{t('settings.logo.help')}</p>
      </div>
      <div className="flex h-24 items-center justify-center rounded bg-background">
        {file ? <img src={file.url} alt={file.name} className="max-h-20 max-w-full object-contain" /> : <span className="text-sm text-muted">{t('settings.logo.default')}</span>}
      </div>
      {file && <p className="truncate text-xs text-muted">{`${file.name}${file.width ? ` · ${file.width} × ${file.height}` : ''} · ${file.size} KB`}</p>}
      {canUpdate && (
        <div className="flex flex-col gap-2 md:flex-row md:flex-wrap *:whitespace-nowrap">
          <Button variant="outline" title={t('settings.logo.change')} icon={<ImagePlus className="h-4 w-4" />} loading={send.isPending} onClick={() => picker.current?.click()} className="w-full md:w-auto">
            {t('settings.logo.change')}
          </Button>
          {file && (
            <Button
              variant="destructive-outline"
              title={t('settings.logo.reset')}
              icon={<RotateCcw className="h-4 w-4" />}
              onClick={async () => {
                if (await confirm({ title: t('settings.logo.reset'), message: t('settings.logo.resetConfirm'), confirmLabel: t('settings.logo.reset'), destructive: true })) send.mutate(null)
              }}
              className="w-full md:w-auto"
            >
              {t('settings.logo.reset')}
            </Button>
          )}
          <input
            ref={picker}
            type="file"
            accept="image/jpeg,image/png,image/svg+xml"
            hidden
            onChange={(e) => {
              const f = e.target.files?.[0]
              if (f) send.mutate(f)
              e.target.value = ''
            }}
          />
        </div>
      )}
    </div>
  )
}

export function Overview() {
  const { t } = useI18n()
  const { information: info, can } = useAuth()
  const canRead = can('admin::project-settings.read')
  const project = useQuery({ queryKey: ['project-settings'], queryFn: () => api.get<ProjectSettings>('/project-settings'), enabled: canRead })
  const rows: [string, string][] = [
    [t('settings.overview.version'), info?.version ?? '-'],
    [t('settings.overview.edition'), t('settings.overview.editionValue')],
    [t('settings.overview.go'), info?.goVersion ?? '-'],
    [t('settings.overview.database'), info?.database ?? '-'],
    [t('settings.overview.environment'), info?.environment ?? '-'],
    [t('settings.overview.contentTypes'), String(info?.contentTypes ?? '-')],
  ]
  return (
    <div className="flex flex-col gap-4">
      <PageHeader title={t('settings.overview')} />
      <Card>
        <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {rows.map(([k, v]) => (
            <div key={k}>
              <dt className="text-xs font-semibold uppercase tracking-wide text-muted">{k}</dt>
              <dd className="mt-1 text-sm">{v}</dd>
            </div>
          ))}
        </dl>
      </Card>
      {canRead && project.data && (
        <Card>
          <CardHeader title={t('settings.logo.title')} />
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <LogoSetting input="menuLogo" file={project.data.menuLogo} canUpdate={can('admin::project-settings.update')} />
            <LogoSetting input="authLogo" file={project.data.authLogo} canUpdate={can('admin::project-settings.update')} />
          </div>
        </Card>
      )}
    </div>
  )
}
