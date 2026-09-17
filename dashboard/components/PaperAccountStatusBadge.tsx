import { Badge } from '@/components/ui/badge'
import type { PaperAccount } from '@/lib/types'
import { cn } from '@/lib/utils'

const statusConfig: Record<
  PaperAccount['status'],
  { label: string; paperLabel: string; dot: string; badge: string }
> = {
  active: {
    label: 'Active',
    paperLabel: 'Paper Trading Active',
    dot: 'bg-emerald-500',
    badge:
      'border-transparent bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300',
  },
  paused: {
    label: 'Paused',
    paperLabel: 'Paper Trading Paused',
    dot: 'bg-amber-500',
    badge:
      'border-transparent bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
  },
  closed: {
    label: 'Closed',
    paperLabel: 'Closed',
    dot: 'bg-slate-400',
    badge:
      'border-transparent bg-slate-100 text-slate-600 dark:bg-slate-800/60 dark:text-slate-300',
  },
}

export function PaperAccountStatusBadge({
  status,
  paperContext = false,
}: {
  status: PaperAccount['status']
  paperContext?: boolean
}) {
  const cfg = statusConfig[status] ?? statusConfig.closed
  return (
    <Badge className={cn('gap-1.5 font-medium', cfg.badge)}>
      <span className={cn('h-1.5 w-1.5 rounded-full', cfg.dot)} aria-hidden />
      {paperContext ? cfg.paperLabel : cfg.label}
    </Badge>
  )
}

export function formatMarketRouting(
  marketName: string,
  exchange: string,
  product: string,
): string {
  return `${marketName} · ${exchange} · ${product}`
}
