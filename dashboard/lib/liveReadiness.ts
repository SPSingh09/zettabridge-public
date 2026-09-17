import type { PaperPnLSummary } from '@/lib/types'

export type ReadinessCheckStatus = 'pass' | 'warn' | 'fail'

export interface LiveReadinessCheck {
  label: string
  status: ReadinessCheckStatus
  detail?: string
}

export interface LiveReadinessResult {
  score: number
  checks: LiveReadinessCheck[]
}

const CHECK_WEIGHT = 20

function checkStatus(ok: boolean, warn: boolean): ReadinessCheckStatus {
  if (ok) return 'pass'
  if (warn) return 'warn'
  return 'fail'
}

function scoreFromStatus(status: ReadinessCheckStatus): number {
  if (status === 'pass') return CHECK_WEIGHT
  if (status === 'warn') return CHECK_WEIGHT / 2
  return 0
}

/** Computes a 0–100 live-readiness score from paper account P&L diagnostics. */
export function computeLiveReadiness(
  pnl: PaperPnLSummary | undefined,
  accountStatus: string,
): LiveReadinessResult {
  if (!pnl) {
    return { score: 0, checks: [] }
  }

  const checks: LiveReadinessCheck[] = []

  const hasWebhook = (pnl.linked_webhooks?.length ?? 0) > 0
  const stableWebhook = hasWebhook && pnl.signals_received >= 5
  checks.push({
    label: 'Webhook connected',
    status: checkStatus(stableWebhook, hasWebhook),
    detail: hasWebhook
      ? `${pnl.signals_received} signals received`
      : 'No webhook linked',
  })

  const fillRate = pnl.webhook_fill_rate ?? pnl.fill_rate
  const consistentFills = fillRate != null && fillRate >= 0.75
  checks.push({
    label: 'Consistent simulated fills',
    status: checkStatus(consistentFills, fillRate != null && fillRate >= 0.5),
    detail: fillRate != null ? `${(fillRate * 100).toFixed(0)}% fill rate` : 'No fills yet',
  })

  const rejectionRate =
    pnl.signals_received > 0 ? pnl.signals_rejected / pnl.signals_received : null
  const lowRejection = rejectionRate != null && rejectionRate <= 0.15
  checks.push({
    label: 'Low rejection rate',
    status: checkStatus(lowRejection, rejectionRate != null && rejectionRate <= 0.3),
    detail:
      rejectionRate != null
        ? `${(rejectionRate * 100).toFixed(0)}% rejected`
        : 'No signals yet',
  })

  const positiveExpectancy = pnl.expectancy != null && pnl.expectancy > 0
  checks.push({
    label: 'Positive expectancy',
    status: checkStatus(positiveExpectancy, pnl.expectancy != null && pnl.expectancy >= 0),
    detail:
      pnl.expectancy != null ? `${pnl.expectancy >= 0 ? '+' : ''}${pnl.expectancy.toFixed(2)}/trade` : 'Need closed trades',
  })

  const drawdownPct =
    pnl.starting_balance > 0 ? pnl.max_drawdown / pnl.starting_balance : 0
  const drawdownOk = drawdownPct <= 0.10
  checks.push({
    label: drawdownOk ? 'Drawdown within 10%' : 'Drawdown above 10%',
    status: checkStatus(drawdownOk, drawdownPct <= 0.15),
    detail: `${(drawdownPct * 100).toFixed(1)}% max drawdown`,
  })

  if (accountStatus === 'paused') {
    return { score: 0, checks: [{ label: 'Account paused', status: 'fail', detail: 'Resume the account to evaluate readiness' }] }
  }

  const score = Math.round(
    checks.reduce((sum, c) => sum + scoreFromStatus(c.status), 0) /
      (checks.length * CHECK_WEIGHT) *
      100,
  )

  return { score, checks }
}

export function healthLabel(status: string | undefined): string {
  switch (status) {
    case 'healthy':
      return 'Healthy'
    case 'paused':
      return 'Paused'
    case 'warning':
      return 'Attention'
    default:
      return 'Unknown'
  }
}

export function healthTone(status: string | undefined): 'positive' | 'negative' | 'neutral' {
  switch (status) {
    case 'healthy':
      return 'positive'
    case 'warning':
      return 'negative'
    default:
      return 'neutral'
  }
}
