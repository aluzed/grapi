import { ArrowLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'

export function BackButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <div>
      <Button variant="outline" title={label} icon={<ArrowLeft className="h-4 w-4" />} onClick={onClick} className="w-full md:w-auto">
        {label}
      </Button>
    </div>
  )
}
