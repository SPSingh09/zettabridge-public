import { Badge } from '@/components/ui/badge'
import { instrumentTypeLabel, instrumentTypeVariant } from '@/lib/marketDisplay'
import type { Instrument } from '@/lib/types'
import { cn } from '@/lib/utils'

const styles: Record<ReturnType<typeof instrumentTypeVariant>, string> = {
  equity: 'border-transparent bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300',
  index: 'border-transparent bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
  future: 'border-transparent bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300',
  option: 'border-transparent bg-violet-100 text-violet-800 dark:bg-violet-900/40 dark:text-violet-300',
}

export function InstrumentTypeBadge({ type }: { type: Instrument['instrument_type'] }) {
  const variant = instrumentTypeVariant(type)
  return (
    <Badge className={cn('font-medium', styles[variant])}>{instrumentTypeLabel(type)}</Badge>
  )
}
