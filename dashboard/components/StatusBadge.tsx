import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

// Dot + colored badge for the three "final outcome" statuses, matching the
// emerald/red/slate palette used elsewhere for status indicators
// (PaperAccountStatusBadge, webhooks list page) — green for a good outcome,
// red for a broker/validation rejection, neutral gray for a cancellation
// (neither good nor bad, just terminated).
const OUTCOME: Record<string, { label: string; dot: string; badge: string }> = {
  filled: {
    label: 'Filled',
    dot: 'bg-emerald-500',
    badge: 'border-transparent bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300',
  },
  rejected: {
    label: 'Rejected',
    dot: 'bg-red-500',
    badge: 'border-transparent bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  },
  cancelled: {
    label: 'Cancelled',
    dot: 'bg-slate-400',
    badge: 'border-transparent bg-slate-100 text-slate-600 dark:bg-slate-800/60 dark:text-slate-300',
  },
}

// StatusBadge renders a trade/order status. Handles both live trade statuses
// (lowercase: filled, submitted, rejected, cancelled, pending_confirmation)
// and paper order statuses (uppercase: RECEIVED, VALIDATED, FILLED, REJECTED,
// CANCELLED) case-insensitively.
export function StatusBadge({ status }: { status: string }) {
  const key = status.toLowerCase()
  const outcome = OUTCOME[key]
  if (outcome) {
    return (
      <Badge className={cn('gap-1.5 font-medium', outcome.badge)}>
        <span className={cn('h-1.5 w-1.5 rounded-full', outcome.dot)} aria-hidden />
        {outcome.label}
      </Badge>
    )
  }
  switch (key) {
    case 'submitted':
      return <Badge variant="secondary">Submitted</Badge>
    case 'pending_confirmation':
      return <Badge variant="warning">Awaiting Kite confirmation</Badge>
    case 'received':
      return <Badge variant="secondary">Received</Badge>
    case 'validated':
      return <Badge variant="secondary">Validated</Badge>
    default:
      return <Badge variant="outline">{status}</Badge>
  }
}
