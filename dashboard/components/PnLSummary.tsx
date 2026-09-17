'use client'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { StatCard, fmt, fmtNotional } from '@/components/ui/stat-card'
import type { PnLSummary } from '@/lib/types'

const BROKER_LABELS: Record<string, string> = {
  mt5_cloud: 'MetaTrader 5',
  zerodha: 'Zerodha',
  angel: 'Angel One',
  dhan: 'Dhan',
}

const ZERODHA_CONNECTION: Record<string, { icon: string; label: string }> = {
  valid: { icon: '🟢', label: 'Connected' },
  expires_soon: { icon: '🟡', label: 'Expires soon' },
  expired: { icon: '🔴', label: 'Disconnected' },
}

function activeDestination(summary: PnLSummary): string {
  if (summary.broker_type) {
    const label = BROKER_LABELS[summary.broker_type] ?? summary.broker_type
    if (summary.broker_type === 'zerodha') {
      return summary.execution_mode === 'publisher' ? `${label} Kite Publisher` : `${label} OAuth`
    }
    return label
  }
  if (summary.market_profile_name) return `Paper Trading — ${summary.market_profile_name}`
  if (summary.paper_account_label) return `Paper Trading — ${summary.paper_account_label}`
  return '—'
}

// ZettaBridge-only latency: signal received (trade created_at) -> order
// handed to the broker (broker_responded_at — set once, right after the
// worker's PlaceOrder/basket-prepare call returns, and never touched again
// by later fill-poll or Publisher confirmation updates). Excludes any time
// spent waiting on Kite to fill, reject, or confirm.
function fmtLatency(ms: number | undefined): string {
  if (ms == null) return '—'
  if (ms < 1000) return `${Math.round(ms)} ms`
  const seconds = ms / 1000
  if (seconds < 60) return `${seconds.toFixed(1)} s`
  const minutes = seconds / 60
  if (minutes < 60) return `${minutes.toFixed(1)} min`
  return `${(minutes / 60).toFixed(1)} h`
}

function fmtDateTime(iso: string | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium">{value}</span>
    </div>
  )
}

interface Props {
  summary: PnLSummary | undefined
  loading?: boolean
  publisherMode?: boolean
}

export function PnLSummaryCard({ summary, loading, publisherMode }: Props) {
  if (loading) {
    return (
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {[...Array(4)].map((_, i) => (
            <Card key={i}>
              <CardContent className="pt-4 pb-4">
                <Skeleton className="h-3 w-20 mb-2" />
                <Skeleton className="h-8 w-16" />
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    )
  }

  const hasAnySignal = !!summary && ((summary.total_signals ?? 0) > 0 || summary.trade_count > 0)

  if (publisherMode && !hasAnySignal) {
    return (
      <div className="rounded-md border border-dashed px-5 py-8 text-center text-sm text-muted-foreground">
        P&amp;L is unavailable in Kite Publisher mode. Publisher does not return fill price, fill status, or execution P&amp;L to ZettaBridge.
      </div>
    )
  }

  if (!hasAnySignal) {
    return (
      <div className="rounded-md border border-dashed px-5 py-8 text-center text-sm text-muted-foreground">
        No signals yet — stats will appear here once this webhook receives one.
      </div>
    )
  }

  const summ = summary!
  const isLiveBroker = !!summ.broker_type
  const zerodhaOAuth =
    summ.broker_type === 'zerodha' &&
    (summ.execution_mode === 'user_api_oauth' || !summ.execution_mode)
  const fillStatsUnavailable = publisherMode || (isLiveBroker && !zerodhaOAuth)
  const unavailSub = publisherMode
    ? 'Unavailable in Publisher mode'
    : fillStatsUnavailable
      ? 'Not tracked for live orders yet'
      : undefined
  const zerodhaHealth = summ.broker_type === 'zerodha' && summ.execution_mode === 'user_api_oauth'
  const connection = summ.zerodha_connection_status ? ZERODHA_CONNECTION[summ.zerodha_connection_status] : undefined
  const rejectionReasons = Object.entries(summ.rejection_reasons ?? {}).sort((a, b) => b[1] - a[1])

  return (
    <div className="space-y-4">
      {publisherMode && (
        <div className="rounded-md border border-blue-200 bg-blue-50/60 dark:border-blue-800 dark:bg-blue-950/30 px-3 py-2.5 text-xs text-blue-800 dark:text-blue-300">
          Kite Publisher mode records basket handoffs only. Final execution status, fill price, rejection reason, and P&amp;L are not returned to ZettaBridge.
        </div>
      )}

      {/* Top row — the signal funnel, one consistent count of "what happened
          to a signal" from door to broker. Total Orders (=trade_count) is
          deliberately not shown here — it's just Accepted + Rejected, and
          having it as a separate number alongside these read as
          contradictory rather than additive. */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
        <StatCard label="Total Signals" value={fmt(summ.total_signals)} />
        <StatCard label="Accepted Signals" value={fmt(summ.accepted_signals)} />
        <StatCard label="Rejected Signals" value={fmt(summ.rejected_signals)} />
        <StatCard label="Submitted to Broker" value={fmt(summ.submitted_to_broker)} />
        <StatCard label="Broker Rejections" value={fmt(summ.broker_rejected)} />
      </div>

      {/* Second row — recent activity at a glance */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard
          label="Avg ZettaBridge Latency"
          value={fmtLatency(summ.avg_latency_ms)}
          sub="Signal received → sent to broker"
        />
        <StatCard label="Last Signal" value={fmtDateTime(summ.last_signal_at)} />
        <StatCard label="Last Broker Response" value={fmtDateTime(summ.last_broker_response_at)} />
        <StatCard label="Active Destination" value={activeDestination(summ)} />
      </div>

      {/* Legacy fill/win/notional — still meaningful for OAuth mode */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard label="Fill Rate" value={fmt(summ.fill_rate, true)} sub={unavailSub} />
        <StatCard label="Win Rate" value={fmt(summ.win_rate, true)} sub={unavailSub} />
        <StatCard label="Notional" value={summ.notional > 0 ? fmtNotional(summ.notional) : '—'} sub={unavailSub} />
        <StatCard label="Broker Success Rate" value={fmt(summ.broker_success_rate, true)} />
      </div>

      {/* Orders by side */}
      <div className="grid grid-cols-3 gap-3">
        <StatCard label="Buy Orders" value={fmt(summ.buy_orders)} />
        <StatCard label="Sell Orders" value={fmt(summ.sell_orders)} />
        <StatCard label="Close Orders" value={fmt(summ.close_orders)} />
      </div>

      {zerodhaHealth && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Broker Health — Zerodha</CardTitle>
          </CardHeader>
          <CardContent className="grid grid-cols-2 gap-x-6 gap-y-2 sm:grid-cols-3">
            <Row label="Zerodha Connection" value={connection ? `${connection.icon} ${connection.label}` : '—'} />
            <Row
              label="Access Token"
              value={
                summ.zerodha_connection_status === 'valid'
                  ? 'Valid today'
                  : summ.zerodha_connection_status === 'expires_soon'
                    ? 'Expires soon'
                    : 'Expired — reconnect required'
              }
            />
            <Row label="Orders Submitted" value={fmt(summ.submitted_to_broker)} />
            <Row label="Broker Rejections" value={fmt(summ.broker_rejected)} />
            <Row label="ZettaBridge Rejections" value={fmt(summ.zettabridge_rejected)} />
            <Row label="Last Broker Response" value={fmtDateTime(summ.last_broker_response_at)} />
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Signal Processing</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <Row label="Total Signals" value={fmt(summ.total_signals)} />
            <Row label="Accepted" value={fmt(summ.accepted_signals)} />
            <Row label="Rejected" value={fmt(summ.rejected_signals)} />
            <Row label="Duplicate Suppressed" value={fmt(summ.duplicate_suppressed)} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Broker Execution</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <Row label="Submitted to Broker" value={fmt(summ.submitted_to_broker)} />
            <Row label="Filled" value={isLiveBroker ? '—' : fmt(summ.filled)} />
            <Row label="Broker Rejected" value={fmt(summ.broker_rejected)} />
            <Row label="Avg ZettaBridge Latency" value={fmtLatency(summ.avg_latency_ms)} />
            {publisherMode && <Row label="Pending Confirmation" value={fmt(summ.pending_confirmation)} />}
            <Row label="Cancelled" value={fmt(summ.cancelled)} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Risk &amp; Validation</CardTitle>
            <p className="text-xs text-muted-foreground">Why ZettaBridge rejected a signal before it reached the broker.</p>
          </CardHeader>
          <CardContent className="space-y-2">
            {rejectionReasons.length === 0 ? (
              <p className="text-sm text-muted-foreground">No rejections recorded.</p>
            ) : (
              rejectionReasons.map(([reason, count]) => <Row key={reason} label={reason} value={count} />)
            )}
            <Row
              label="Last Rejection Reason"
              value={summ.last_rejection_reason ? summ.last_rejection_reason : '—'}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Recent Activity</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <Row label="Last Signal Time" value={fmtDateTime(summ.last_signal_at)} />
            <Row label="Last Broker Response" value={fmtDateTime(summ.last_broker_response_at)} />
            <Row
              label="Last Rejection Reason"
              value={summ.last_rejection_reason ? summ.last_rejection_reason : '—'}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
