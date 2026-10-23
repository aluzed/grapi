import { useState } from 'react'
import { Languages } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { SelectField } from '@/components/ui/field'
import { ApiError } from '@/lib/api'
import { showError } from '@/lib/form'
import type { Entry, Locale } from '@/lib/types'
import { useI18n } from '@/providers/I18nProvider'

type Props = { locales: Locale[]; load: (locale: string) => Promise<Entry>; onFill: (entry: Entry) => void }

export function FillFromLocale({ locales, load, onFill }: Props) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const [source, setSource] = useState('')
  const [loading, setLoading] = useState(false)
  const chosen = source || locales[0]?.code || ''
  const name = (code: string) => locales.find((l) => l.code === code)?.name ?? code

  const fill = async () => {
    setLoading(true)
    try {
      onFill(await load(chosen))
      toast.success(t('cm.filled', { locale: name(chosen) }))
      setOpen(false)
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) toast.error(t('cm.fillMissing', { locale: name(chosen) }))
      else showError(err, undefined, t('common.error'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <>
      <Button variant="outline" title={t('cm.fill')} icon={<Languages className="h-4 w-4" />} onClick={() => setOpen(true)} className="w-full md:w-auto">
        {t('cm.fill')}
      </Button>
      <Dialog
        open={open}
        title={t('cm.fill')}
        onClose={() => setOpen(false)}
        closeLabel={t('common.cancel')}
        footer={
          <>
            <Button variant="outline" title={t('common.cancel')} onClick={() => setOpen(false)} className="w-full md:w-auto">
              {t('common.cancel')}
            </Button>
            <Button title={t('cm.fillConfirm')} icon={<Languages className="h-4 w-4" />} loading={loading} onClick={() => void fill()} className="w-full md:w-auto">
              {t('cm.fillConfirm')}
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-4">
          <p className="text-sm text-muted">{t('cm.fillHint')}</p>
          <SelectField label={t('cm.fillSource')} value={chosen} onChange={(e) => setSource(e.target.value)}>
            {locales.map((l) => (
              <option key={l.code} value={l.code}>
                {l.name}
              </option>
            ))}
          </SelectField>
        </div>
      </Dialog>
    </>
  )
}
