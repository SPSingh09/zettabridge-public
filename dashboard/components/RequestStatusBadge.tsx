import { Badge } from '@/components/ui/badge'
import { statusLabel } from '@/lib/requestDisplay'
import type { SymbolRequestStatus } from '@/lib/types'
import { cn } from '@/lib/utils'

const styles: Record<SymbolRequestStatus, string> = {
  pending: 'border-transparent bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
  accepted: 'border-transparent bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300',
  rejected: 'border-transparent bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  resolved: 'border-transparent bg-slate-100 text-slate-600 dark:bg-slate-800/60 dark:text-slate-300',
}

const dots: Record<SymbolRequestStatus, string> = {
  pending: 'bg-amber-500',
  accepted: 'bg-emerald-500',
  rejected: 'bg-red-500',
  resolved: 'bg-slate-400',
}

export function RequestStatusBadge({ status }: { status: SymbolRequestStatus }) {
  return (
    <Badge className={cn('gap-1.5 font-medium', styles[status])}>
      <span className={cn('h-1.5 w-1.5 rounded-full', dots[status])} aria-hidden />
      {statusLabel(status)}
    </Badge>
  )
}
