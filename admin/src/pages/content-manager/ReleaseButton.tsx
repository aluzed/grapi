import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CalendarClock, Plus } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { SelectField } from '@/components/ui/field'
import { api } from '@/lib/api'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import type { Model } from '@/lib/types'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'
import type { Release } from '@/pages/releases/model'
import { ReleaseDialog } from '@/pages/releases/Releases'

type Membership = { id: number; name: string; type: 'publish' | 'unpublish'; locale: string | null }

export function ReleaseButton({ model, documentId, locale }: { model: Model; documentId: string; locale?: string }) {
  const { t } = useI18n()
  const { permissions } = useAuth()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [releaseID, setReleaseID] = useState('')
  const [type, setType] = useState<'publish' | 'unpublish'>('publish')
  const memberships = useQuery({
    queryKey: ['releases', 'document', model.uid, documentId],
    queryFn: () => api.get<Membership[]>(`/releases/document/${encodeURIComponent(model.uid)}/${encodeURIComponent(documentId)}`),
  })
  const releases = useQuery({ queryKey: ['releases', 'pending'], queryFn: () => api.get<Release[]>('/releases'), enabled: open })
  const add = useMutation({
    mutationFn: () => api.post(`/releases/${releaseID}/actions`, { type, uid: model.uid, documentId, locale }),
    onSuccess: () => {
      toast.success(t('releases.added'))
      void queryClient.invalidateQueries({ queryKey: ['releases'] })
      setOpen(false)
    },
    onError: (e) => showError(e, undefined, t('common.error')),
  })
  if (!model.draftAndPublish || !can(permissions, 'admin::releases.actions.create') || !model.permissions.publish) return null
  const mine = (memberships.data ?? []).filter((m) => !model.localized || !locale || m.locale === locale)
  const available = (releases.data ?? []).filter((r) => r.status !== 'failed' && !mine.some((m) => m.id === r.id))
  return (
    <>
      <Button variant="outline" title={t('releases.addTo')} icon={<CalendarClock className="h-4 w-4" />} onClick={() => setOpen(true)} className="w-full md:w-auto">
        {t('releases.addTo')}
      </Button>
      {open && (
        <Dialog
          open
          title={t('releases.addTo')}
          onClose={() => setOpen(false)}
          closeLabel={t('common.cancel')}
          footer={
            <>
              <Button variant="outline" title={t('common.cancel')} onClick={() => setOpen(false)} className="w-full md:w-auto">
                {t('common.cancel')}
              </Button>
              <Button title={t('releases.add')} icon={<Plus className="h-4 w-4" />} loading={add.isPending} disabled={!releaseID} onClick={() => add.mutate()} className="w-full md:w-auto">
                {t('releases.add')}
              </Button>
            </>
          }
        >
          <div className="flex flex-col gap-4">
            {mine.length > 0 && (
              <div className="text-sm">
                <p className="font-medium">{t('releases.alreadyIn')}</p>
                <ul className="mt-1 list-disc pl-5 text-muted">
                  {mine.map((m) => (
                    <li key={m.id}>
                      {m.name} ({t(m.type === 'publish' ? 'releases.type.publish' : 'releases.type.unpublish')})
                    </li>
                  ))}
                </ul>
              </div>
            )}
            <SelectField label={t('releases.release')} value={releaseID} onChange={(e) => setReleaseID(e.target.value)}>
              <option value="">{t('releases.choose')}</option>
              {available.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </SelectField>
            {can(permissions, 'admin::releases.create') && (
              <div>
                <Button variant="outline" title={t('releases.create')} icon={<Plus className="h-4 w-4" />} onClick={() => setCreating(true)} className="w-full md:w-auto">
                  {t('releases.create')}
                </Button>
              </div>
            )}
            <SelectField label={t('releases.action')} value={type} onChange={(e) => setType(e.target.value as 'publish' | 'unpublish')}>
              <option value="publish">{t('releases.type.publish')}</option>
              <option value="unpublish">{t('releases.type.unpublish')}</option>
            </SelectField>
          </div>
        </Dialog>
      )}
      {creating && <ReleaseDialog release={null} onClose={() => setCreating(false)} onSaved={(r) => setReleaseID(String(r.id))} />}
    </>
  )
}
