import { useState } from 'react'
import { PageHeader } from '@/app/PageHeader'
import type { MediaFile } from '@/lib/types'
import { useI18n } from '@/providers/I18nProvider'
import { FileDialog } from './FileDialog'
import { MediaBrowser } from './MediaBrowser'

export function MediaLibrary() {
  const { t } = useI18n()
  const [open, setOpen] = useState<MediaFile | null>(null)
  return (
    <div className="mx-auto max-w-6xl">
      <PageHeader title={t('media.title')} />
      <MediaBrowser onOpen={setOpen} />
      {open && <FileDialog key={open.id} file={open} onClose={() => setOpen(null)} />}
    </div>
  )
}
