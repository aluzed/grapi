import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'
import { Check } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { useI18n } from './I18nProvider'

type Options = { title: string; message: string; confirmLabel?: string; destructive?: boolean }
type Confirm = (o: Options) => Promise<boolean>

const ConfirmContext = createContext<Confirm | null>(null)

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const { t } = useI18n()
  const [opts, setOpts] = useState<Options | null>(null)
  const resolver = useRef<(v: boolean) => void>(() => {})
  const confirm = useCallback<Confirm>((o) => {
    setOpts(o)
    return new Promise<boolean>((resolve) => {
      resolver.current = resolve
    })
  }, [])
  const close = (v: boolean) => {
    resolver.current(v)
    setOpts(null)
  }
  const label = opts?.confirmLabel ?? t('common.confirm')
  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <Dialog
        open={opts !== null}
        title={opts?.title ?? ''}
        onClose={() => close(false)}
        closeLabel={t('common.cancel')}
        footer={
          <>
            <Button variant="outline" title={t('common.cancel')} onClick={() => close(false)} className="w-full md:w-auto">
              {t('common.cancel')}
            </Button>
            <Button variant={opts?.destructive ? 'destructive' : 'primary'} title={label} icon={<Check className="h-4 w-4" />} onClick={() => close(true)} className="w-full md:w-auto">
              {label}
            </Button>
          </>
        }
      >
        <p className="text-sm">{opts?.message}</p>
      </Dialog>
    </ConfirmContext.Provider>
  )
}

export function useConfirm(): Confirm {
  const ctx = useContext(ConfirmContext)
  if (!ctx) throw new Error('useConfirm outside ConfirmProvider')
  return ctx
}
