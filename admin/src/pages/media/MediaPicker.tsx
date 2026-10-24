import { useState } from 'react'
import { Check } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Tabs } from '@/components/ui/tabs'
import { can } from '@/lib/permissions'
import type { MediaFile } from '@/lib/types'
import { useAuth } from '@/providers/AuthProvider'
import { useI18n } from '@/providers/I18nProvider'
import { MediaBrowser } from './MediaBrowser'
import { UploadPanel } from './UploadPanel'

// MediaPicker is the media library dialog of a field: browse the library, or
// add new files, which come back selected.
export function MediaPicker({ multiple, allowedTypes, onChoose, onClose }: { multiple: boolean; allowedTypes?: string[]; onChoose: (files: MediaFile[]) => void; onClose: () => void }) {
  const { t } = useI18n()
  const { permissions } = useAuth()
  const canUpload = can(permissions, 'plugin::upload.assets.create')
  const [tab, setTab] = useState<'browse' | 'add'>('browse')
  const [selection, setSelection] = useState<MediaFile[]>([])
  return (
    <Dialog
      open
      title={t('media.pick')}
      onClose={onClose}
      closeLabel={t('common.cancel')}
      wide
      footer={
        <>
          <span className="text-sm text-muted md:mr-auto">{t('media.selected', { count: String(selection.length) })}</span>
          <Button variant="outline" title={t('common.cancel')} onClick={onClose} className="w-full md:w-auto">
            {t('common.cancel')}
          </Button>
          <Button title={t('media.choose')} icon={<Check className="h-4 w-4" />} disabled={selection.length === 0} onClick={() => onChoose(selection)} className="w-full md:w-auto">
            {t('media.choose')}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {canUpload && (
          <Tabs
            tabs={[
              { id: 'browse', label: t('media.tabBrowse') },
              { id: 'add', label: t('media.tabAdd') },
            ]}
            value={tab}
            onChange={(id) => setTab(id as 'browse' | 'add')}
          />
        )}
        {tab === 'add' && canUpload ? (
          <UploadPanel
            allowedTypes={allowedTypes}
            onDone={(files) => {
              const known = new Set(selection.map((f) => f.id))
              setSelection(multiple ? [...selection, ...files.filter((f) => !known.has(f.id))] : files.slice(-1))
              setTab('browse')
            }}
          />
        ) : (
          <MediaBrowser pick={{ multiple, selected: selection, onChange: setSelection, allowedTypes }} />
        )}
      </div>
    </Dialog>
  )
}
