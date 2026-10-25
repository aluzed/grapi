import type { ReactNode } from 'react'
import { Save } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useI18n } from '@/providers/I18nProvider'

type DialogActionsProps = {
  onCancel: () => void
  onConfirm: () => void
  label?: string
  icon?: ReactNode
  loading?: boolean
  disabled?: boolean
  // Extra actions, between cancel and confirm.
  children?: ReactNode
}

export function DialogActions({ onCancel, onConfirm, label, icon = <Save className="h-4 w-4" />, loading, disabled, children }: DialogActionsProps) {
  const { t } = useI18n()
  const text = label ?? t('common.save')
  return (
    <>
      <Button variant="outline" title={t('common.cancel')} onClick={onCancel} className="w-full md:w-auto">
        {t('common.cancel')}
      </Button>
      {children}
      <Button title={text} icon={icon} loading={loading} disabled={disabled} onClick={onConfirm} className="w-full md:w-auto">
        {text}
      </Button>
    </>
  )
}
