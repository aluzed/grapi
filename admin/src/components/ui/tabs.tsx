import { cn } from '@/lib/cn'

type Tab = { id: string; label: string; disabled?: boolean; hint?: string }

export function Tabs({ tabs, value, onChange }: { tabs: Tab[]; value: string; onChange: (id: string) => void }) {
  return (
    <div role="tablist" className="flex w-full rounded-md border border-border bg-background p-1 sm:inline-flex sm:w-fit">
      {tabs.map((t) => (
        <button
          key={t.id}
          role="tab"
          type="button"
          title={t.disabled && t.hint ? t.hint : t.label}
          aria-selected={value === t.id}
          disabled={t.disabled}
          onClick={() => onChange(t.id)}
          className={cn('min-h-9 flex-1 rounded px-3 text-sm font-medium disabled:cursor-not-allowed disabled:text-zinc-400 sm:flex-none', value === t.id ? 'bg-surface shadow-sm' : 'text-muted hover:text-foreground')}
        >
          {t.label}
        </button>
      ))}
    </div>
  )
}
