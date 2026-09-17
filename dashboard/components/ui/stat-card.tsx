import { Card, CardContent } from '@/components/ui/card'

interface StatCardProps {
  label: string
  value: string
  sub?: string
  /** Optional color tone for the value text, e.g. for P&L figures. */
  tone?: 'positive' | 'negative' | 'neutral'
}

const TONE_CLASSES: Record<NonNullable<StatCardProps['tone']>, string> = {
  positive: 'text-green-600 dark:text-green-500',
  negative: 'text-destructive',
  neutral: '',
}

export function StatCard({ label, value, sub, tone = 'neutral' }: StatCardProps) {
  return (
    <Card>
      <CardContent className="pt-4 pb-4">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className={`mt-1 text-2xl font-semibold ${TONE_CLASSES[tone]}`}>{value}</p>
        {sub && <p className="mt-0.5 text-xs text-muted-foreground/70">{sub}</p>}
      </CardContent>
    </Card>
  )
}

/** Picks a StatCard tone from a signed P&L number. */
export function pnlTone(n: number): StatCardProps['tone'] {
  if (n > 0) return 'positive'
  if (n < 0) return 'negative'
  return 'neutral'
}

// fmt formats a plain number, or a 0.0-1.0 ratio as a percentage when pct is true.
export function fmt(n: number | undefined, pct = false): string {
  if (n === undefined || n === null) return '—'
  if (pct) return `${(n * 100).toFixed(1)}%`
  return n.toLocaleString()
}

// fmtNotional formats a rupee amount with K/M suffixes for large values.
export function fmtNotional(n: number): string {
  if (n >= 1_000_000) return `₹${(n / 1_000_000).toFixed(2)}M`
  if (n >= 1_000) return `₹${(n / 1_000).toFixed(1)}K`
  return `₹${n.toFixed(2)}`
}
