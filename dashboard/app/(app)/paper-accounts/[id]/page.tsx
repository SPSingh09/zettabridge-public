'use client'

import { useEffect, useMemo, useState } from 'react'
import { useParams } from 'next/navigation'
import Link from 'next/link'
import useSWR from 'swr'
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import {
  ArrowLeft, CheckCircle2, Loader2, Rocket, TriangleAlert, X, AlertTriangle, Circle,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { StatCard, fmt, fmtNotional, pnlTone } from '@/components/ui/stat-card'
import { ActionBadge } from '@/components/ActionBadge'
import { StatusBadge } from '@/components/StatusBadge'
import { PaperAccountStatusBadge } from '@/components/PaperAccountStatusBadge'
import { WsIndicator } from '@/components/WsIndicator'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { useToast } from '@/components/ToastProvider'
import { apiFetch, downloadCsv } from '@/lib/api'
import { getToken } from '@/lib/auth'
import { useUser } from '@/lib/user-context'
import { createTradeSocket, type WsStatus } from '@/lib/ws'
import { formatCurrencyAmount } from '@/lib/marketDisplay'
import { computeLiveReadiness, healthLabel, healthTone } from '@/lib/liveReadiness'
import type {
  PaperAccount, PaperPosition, PaginatedPaperOrders, PaperPnLSummary, PaperAccountSnapshot, Webhook,
} from '@/lib/types'

const ORDERS_PAGE_SIZE = 50

function fmtTime(ts: string) {
  return new Date(ts).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function fmtChartTime(ts: string) {
  return new Date(ts).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function fmtDate(ts: string) {
  return new Date(ts).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

function minuteBucket(ts: number): string {
  const d = new Date(ts)
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}-${d.getHours()}-${d.getMinutes()}`
}

function collapseChartPoints<T extends { ts: number }>(points: T[]): T[] {
  const collapsed: T[] = []
  for (const point of points) {
    const prev = collapsed[collapsed.length - 1]
    if (prev && minuteBucket(prev.ts) === minuteBucket(point.ts)) {
      collapsed[collapsed.length - 1] = point
    } else {
      collapsed.push(point)
    }
  }
  return collapsed
}

function shortOrderId(id: string): string {
  return id.length > 8 ? `${id.slice(0, 8)}…` : id
}

function fmtPnL(n: number): string {
  const sign = n > 0 ? '+' : ''
  return `${sign}${fmtNotional(n)}`
}

function fmtDrawdown(n: number): string {
  if (n === 0) return fmtNotional(0)
  return `-${fmtNotional(n)}`
}

function SectionHeading({ title, description }: { title: string; description?: string }) {
  return (
    <div className="mb-3">
      <h2 className="text-lg font-semibold">{title}</h2>
      {description && <p className="text-sm text-muted-foreground">{description}</p>}
    </div>
  )
}

function ReadinessIcon({ status }: { status: 'pass' | 'warn' | 'fail' }) {
  if (status === 'pass') return <CheckCircle2 className="h-4 w-4 shrink-0 text-green-600 dark:text-green-500" />
  if (status === 'warn') return <AlertTriangle className="h-4 w-4 shrink-0 text-amber-500" />
  return <Circle className="h-4 w-4 shrink-0 text-muted-foreground" />
}

export default function PaperAccountDetailPage() {
  const params = useParams()
  const id = params.id as string
  const { user } = useUser()
  const canLive = user.live_trading_allowed
  const { toast } = useToast()

  const [closing, setClosing] = useState<PaperPosition | null>(null)
  const [closingBusy, setClosingBusy] = useState(false)

  const { data: account, isLoading: accountLoading } = useSWR<PaperAccount>(
    id ? `/v1/paper-accounts/${id}` : null,
    (url: string) => apiFetch<PaperAccount>(url),
  )
  const { data: positions, mutate: mutatePositions } = useSWR<PaperPosition[]>(
    id ? `/v1/paper-accounts/${id}/positions` : null,
    (url: string) => apiFetch<PaperPosition[]>(url),
  )

  const [offset, setOffset] = useState(0)
  const { data: ordersPage, mutate: mutateOrders } = useSWR<PaginatedPaperOrders>(
    id ? `/v1/paper-accounts/${id}/orders?limit=${ORDERS_PAGE_SIZE}&offset=${offset}` : null,
    (url: string) => apiFetch<PaginatedPaperOrders>(url),
  )
  const [allOrders, setAllOrders] = useState<PaginatedPaperOrders['orders']>([])
  useEffect(() => {
    if (!ordersPage) return
    setAllOrders(prev => (offset === 0 ? ordersPage.orders : [...prev, ...ordersPage.orders]))
  }, [ordersPage, offset])

  const { data: pnl, mutate: mutatePnl } = useSWR<PaperPnLSummary>(
    id ? `/v1/paper-accounts/${id}/pnl` : null,
    (url: string) => apiFetch<PaperPnLSummary>(url),
  )
  const { data: snapshots, mutate: mutateSnapshots } = useSWR<PaperAccountSnapshot[]>(
    id ? `/v1/paper-accounts/${id}/snapshots?limit=200` : null,
    (url: string) => apiFetch<PaperAccountSnapshot[]>(url),
  )
  const { data: webhooks } = useSWR<Webhook[]>('/v1/webhooks', (url: string) => apiFetch<Webhook[]>(url))
  const linkedWebhooks = useMemo(
    () => webhooks?.filter(w => w.paper_account_id === id) ?? [],
    [webhooks, id],
  )
  const linkedWebhookIds = useMemo(() => new Set(linkedWebhooks.map(w => w.id)), [linkedWebhooks])

  const readiness = useMemo(
    () => computeLiveReadiness(pnl, account?.status ?? 'active'),
    [pnl, account?.status],
  )

  const [wsStatus, setWsStatus] = useState<WsStatus>('connecting')
  useEffect(() => {
    const token = getToken()
    if (!token || !id) return
    const socket = createTradeSocket(
      token,
      (trade) => {
        if (linkedWebhookIds.size > 0 && !linkedWebhookIds.has(trade.webhook_id)) return
        mutatePositions()
        setOffset(0)
        mutateOrders()
        mutatePnl()
        mutateSnapshots()
      },
      setWsStatus,
      (mark) => {
        if (mark.paper_account_id !== id) return
        mutatePositions()
        mutatePnl()
        mutateSnapshots()
      },
    )
    socket.connect()
    return () => socket.disconnect()
  }, [id, linkedWebhookIds]) // eslint-disable-line react-hooks/exhaustive-deps

  async function handleClosePosition() {
    if (!closing) return
    setClosingBusy(true)
    try {
      await apiFetch(`/v1/paper-accounts/${id}/positions/close`, {
        method: 'POST',
        body: JSON.stringify({ symbol: closing.symbol }),
      })
      setClosing(null)
      mutatePositions()
      setOffset(0)
      mutateOrders()
      mutatePnl()
      mutateSnapshots()
      toast(`${closing.symbol} position closed`, 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to close position', 'error')
    } finally {
      setClosingBusy(false)
    }
  }

  const chartData = useMemo(() => {
    if (!account) return []

    type Point = { time: string; equity: number; ts: number }
    const points: Point[] = [{
      time: fmtChartTime(account.created_at),
      equity: account.starting_balance,
      ts: new Date(account.created_at).getTime(),
    }]

    for (const s of [...(snapshots ?? [])].reverse()) {
      const ts = new Date(s.created_at).getTime()
      if (ts <= points[points.length - 1].ts) continue
      points.push({ time: fmtChartTime(s.created_at), equity: s.equity, ts })
    }

    if (pnl) {
      const now = Date.now()
      const last = points[points.length - 1]
      if (!last || Math.abs(last.equity - pnl.equity) > 0.01 || now - last.ts > 30_000) {
        points.push({ time: 'Now', equity: pnl.equity, ts: now })
      }
    }

    return collapseChartPoints(points).map(({ time, equity }) => ({ time, equity }))
  }, [account, snapshots, pnl])

  if (accountLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    )
  }

  if (!account) {
    return <div className="text-sm text-muted-foreground">Paper trading account not found.</div>
  }

  const openPositions = (positions ?? []).filter(p => p.quantity !== 0)
  const total = ordersPage?.total ?? 0
  const hasMoreOrders = allOrders.length < total
  const exposurePct = pnl?.exposure_pct != null ? `${(pnl.exposure_pct * 100).toFixed(0)}%` : '—'

  return (
    <div>
      <div className="mb-6 flex items-start justify-between">
        <div>
          <Link
            href="/paper-accounts"
            className="mb-2 flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
          >
            <ArrowLeft className="h-3.5 w-3.5" /> Paper Accounts
          </Link>
          <h1 className="flex flex-wrap items-center gap-3 text-2xl font-semibold">
            {account.label || <span className="italic text-muted-foreground">Unnamed</span>}
            <PaperAccountStatusBadge status={account.status} paperContext />
          </h1>
          <p className="mt-1 font-mono text-sm text-muted-foreground">
            {account.exchange} · {account.default_product} · {account.base_currency}
          </p>
        </div>
        {linkedWebhooks.length > 0 && <WsIndicator status={wsStatus} connectedLabel="Connected" />}
      </div>

      <Alert variant="warning" className="mb-6">
        <TriangleAlert className="h-4 w-4" />
        <AlertTitle>Simulation notice</AlertTitle>
        <AlertDescription>
          Paper trading results are simulated and may differ from live execution due to slippage,
          liquidity, latency, rejection, and broker/exchange rules.
        </AlertDescription>
      </Alert>

      {/* Live Readiness */}
      <Card className="mb-6">
        <CardContent className="flex flex-col gap-5 py-5 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex items-start gap-4">
            <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xl font-bold text-primary">
              {pnl ? `${readiness.score}%` : '—'}
            </div>
            <div>
              <p className="font-medium">Live Readiness Check</p>
              <p className="text-sm text-muted-foreground">
                An advisory checklist based on paper-trading activity. It does not guarantee live
                execution performance.
              </p>
              {readiness.checks.length > 0 && (
                <ul className="mt-3 space-y-1.5">
                  {readiness.checks.map(c => (
                    <li key={c.label} className="flex items-center gap-2 text-sm">
                      <ReadinessIcon status={c.status} />
                      <span>{c.label}</span>
                      {c.detail && <span className="text-muted-foreground">· {c.detail}</span>}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-3">
            <Rocket className="h-5 w-5 shrink-0 text-muted-foreground" />
            <div className="text-sm">
              <p className="font-medium">Ready to go live?</p>
              <p className="text-muted-foreground">
                {!canLive
                  ? 'Upgrade your plan to connect a live broker and trade real money with this configuration.'
                  : linkedWebhooks.length > 0
                    ? "Connect a real broker and clone this webhook's configuration to trade real money."
                    : 'Create a webhook for this account first, then you can go live with the same setup.'}
              </p>
            </div>
            <Button asChild variant="outline" size="sm" className="ml-2 shrink-0">
              <Link
                href={
                  !canLive
                    ? '/billing'
                    : linkedWebhooks.length > 0
                      ? `/webhooks/new?cloneFrom=${linkedWebhooks[0].id}`
                      : '/webhooks/new'
                }
              >
                {!canLive ? 'Upgrade' : 'Go Live'}
              </Link>
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* 1. Account Summary */}
      <SectionHeading title="Account Summary" description="Health, equity, and today's performance." />
      <div className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <StatCard
          label="Status"
          value={healthLabel(pnl?.account_health_status)}
          tone={healthTone(pnl?.account_health_status)}
        />
        <StatCard label="Paper Equity" value={pnl ? fmtNotional(pnl.equity) : '—'} />
        <StatCard label="Cash Balance" value={pnl ? fmtNotional(pnl.cash_balance) : '—'} />
        <StatCard label="Buying Power" value={pnl ? fmtNotional(pnl.buying_power) : '—'} />
        <StatCard label="Used Margin" value={pnl ? fmtNotional(pnl.used_margin) : '—'} />
        <StatCard
          label="Total P&L"
          value={pnl ? fmtPnL(pnl.total_pnl) : '—'}
          tone={pnl ? pnlTone(pnl.total_pnl) : 'neutral'}
        />
      </div>
      <div className="mb-8 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <StatCard
          label="Today's P&L"
          value={pnl?.today_pnl != null ? fmtPnL(pnl.today_pnl) : '—'}
          tone={pnl?.today_pnl != null ? pnlTone(pnl.today_pnl) : 'neutral'}
        />
        <StatCard label="Today's Trades" value={fmt(pnl?.today_trades)} />
        <StatCard label="Today's Win Rate" value={fmt(pnl?.today_win_rate, true)} />
        <StatCard label="Today's Fees" value={pnl ? fmtNotional(pnl.today_fees) : '—'} />
        <StatCard
          label="Today's Notional"
          value={pnl && pnl.today_notional > 0 ? fmtNotional(pnl.today_notional) : '—'}
        />
      </div>

      {/* 2. Trading Performance */}
      <SectionHeading title="Trading Performance" description="Lifetime stats traders use to evaluate a strategy." />
      <div className="mb-8 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        <StatCard label="Win Rate" value={fmt(pnl?.win_rate, true)} />
        <StatCard
          label="Profit Factor"
          value={pnl?.profit_factor != null ? pnl.profit_factor.toFixed(2) : '—'}
          sub={pnl ? `Gross +${fmtNotional(pnl.gross_profit)} / -${fmtNotional(-pnl.gross_loss)}` : undefined}
        />
        <StatCard
          label="Average Win"
          value={pnl?.average_win != null ? fmtPnL(pnl.average_win) : '—'}
          tone={pnl?.average_win != null ? 'positive' : 'neutral'}
        />
        <StatCard
          label="Average Loss"
          value={pnl?.average_loss != null ? fmtPnL(pnl.average_loss) : '—'}
          tone={pnl?.average_loss != null ? 'negative' : 'neutral'}
        />
        <StatCard
          label="Largest Win"
          value={pnl?.largest_win != null ? fmtPnL(pnl.largest_win) : '—'}
          tone={pnl?.largest_win != null ? 'positive' : 'neutral'}
        />
        <StatCard
          label="Largest Loss"
          value={pnl?.largest_loss != null ? fmtPnL(pnl.largest_loss) : '—'}
          tone={pnl?.largest_loss != null ? 'negative' : 'neutral'}
        />
        <StatCard
          label="Peak Equity"
          value={pnl ? fmtNotional(pnl.peak_equity) : '—'}
        />
        <StatCard
          label="Current Drawdown"
          value={pnl ? fmtDrawdown(pnl.current_drawdown) : '—'}
          tone={pnl && pnl.current_drawdown > 0 ? 'negative' : 'neutral'}
        />
        <StatCard
          label="Max Drawdown"
          value={pnl ? fmtDrawdown(pnl.max_drawdown) : '—'}
          tone={pnl && pnl.max_drawdown > 0 ? 'negative' : 'neutral'}
        />
        <StatCard
          label="Expectancy"
          value={pnl?.expectancy != null ? fmtPnL(pnl.expectancy) : '—'}
          tone={pnl?.expectancy != null ? pnlTone(pnl.expectancy) : 'neutral'}
        />
        <StatCard label="Total Trades" value={fmt(pnl?.trade_count)} />
        <StatCard label="Total Fees" value={pnl ? fmtNotional(pnl.total_fees) : '—'} />
      </div>

      {/* 3. Risk */}
      <SectionHeading title="Risk" description="Exposure, drawdown, and open risk at a glance." />
      <div className="mb-8 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        <StatCard
          label="Current Exposure"
          value={pnl ? fmtNotional(pnl.exposure) : '—'}
          sub={exposurePct !== '—' ? `${exposurePct} of equity` : undefined}
        />
        <StatCard label="Largest Position" value={pnl ? fmtNotional(pnl.largest_position) : '—'} />
        <StatCard label="Open Risk" value={pnl ? fmtNotional(pnl.open_risk) : '—'} />
        <StatCard label="Today's Risk" value={pnl ? fmtNotional(pnl.today_risk) : '—'} />
        <StatCard label="Open Positions" value={fmt(pnl?.open_positions)} sub={`Long ${pnl?.long_positions ?? 0} · Short ${pnl?.short_positions ?? 0}`} />
        <StatCard label="Open Orders" value={fmt(pnl?.open_orders)} />
        <StatCard label="Net Exposure" value={pnl ? fmtNotional(pnl.exposure) : '—'} />
        <StatCard
          label="Largest Losing Trade"
          value={pnl?.largest_loss != null ? fmtPnL(pnl.largest_loss) : '—'}
          tone={pnl?.largest_loss != null ? 'negative' : 'neutral'}
        />
      </div>

      {/* 4. Positions & Trades */}
      <SectionHeading title="Positions & Trades" description="Equity curve, open positions, and order history." />

      <Card className="mb-6">
        <CardHeader>
          <CardTitle className="text-base">Equity</CardTitle>
        </CardHeader>
        <CardContent>
          {!pnl ? (
            <p className="py-8 text-center text-sm text-muted-foreground">
              <Loader2 className="mx-auto mb-2 h-4 w-4 animate-spin" /> Loading equity…
            </p>
          ) : chartData.length < 2 ? (
            <p className="py-8 text-center text-sm text-muted-foreground">
              Equity history will appear after your first trade.
            </p>
          ) : (
            <div className="h-64 w-full">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={chartData} margin={{ top: 8, right: 8, left: 8, bottom: 0 }}>
                  <defs>
                    <linearGradient id="equityFill" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="hsl(var(--primary))" stopOpacity={0.35} />
                      <stop offset="95%" stopColor="hsl(var(--primary))" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" opacity={0.2} />
                  <XAxis
                    dataKey="time"
                    tick={{ fontSize: 11 }}
                    minTickGap={48}
                    interval="preserveStartEnd"
                  />
                  <YAxis tick={{ fontSize: 11 }} domain={['auto', 'auto']} width={70} />
                  <Tooltip formatter={(v) => fmtNotional(Number(v))} />
                  <Area type="monotone" dataKey="equity" stroke="hsl(var(--primary))" fill="url(#equityFill)" strokeWidth={2} />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="mb-6">
        <CardHeader>
          <CardTitle className="text-base">Positions</CardTitle>
          <p className="text-xs text-muted-foreground">
            LTP and unrealized P&L update from market data when available, otherwise from signal prices.
          </p>
        </CardHeader>
        <CardContent className="p-0">
          {openPositions.length === 0 ? (
            <p className="px-6 pb-6 text-sm text-muted-foreground">No open positions.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    <th className="px-4 py-3">Symbol</th>
                    <th className="px-4 py-3">Product</th>
                    <th className="px-4 py-3">Qty</th>
                    <th className="px-4 py-3">Avg Price</th>
                    <th className="px-4 py-3">LTP</th>
                    <th className="px-4 py-3">Market value</th>
                    <th className="px-4 py-3">Unrealized P&L</th>
                    <th className="px-4 py-3 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {openPositions.map(p => (
                    <tr key={p.id} className="hover:bg-muted/30">
                      <td className="px-4 py-3 font-mono text-xs">{p.symbol}</td>
                      <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{p.product}</td>
                      <td className={`px-4 py-3 font-mono text-xs ${p.quantity > 0 ? 'text-green-600 dark:text-green-500' : 'text-destructive'}`}>
                        {p.quantity > 0 ? '+' : ''}{p.quantity}
                      </td>
                      <td className="px-4 py-3 font-mono text-xs">{p.avg_entry_price.toFixed(2)}</td>
                      <td className="px-4 py-3 font-mono text-xs">{p.last_price.toFixed(2)}</td>
                      <td className="px-4 py-3 font-mono text-xs">
                        {formatCurrencyAmount(Math.abs(p.quantity) * p.last_price, account.base_currency)}
                      </td>
                      <td className={`px-4 py-3 font-mono text-xs ${pnlTone(p.unrealized_pnl) === 'positive' ? 'text-green-600 dark:text-green-500' : pnlTone(p.unrealized_pnl) === 'negative' ? 'text-destructive' : ''}`}>
                        {fmtPnL(p.unrealized_pnl)}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          title="Close position"
                          onClick={() => setClosing(p)}
                        >
                          <X className="mr-1 h-3.5 w-3.5" /> Close position
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      <ConfirmDialog
        open={!!closing}
        title="Close position?"
        description={
          closing
            ? `This closes your ${closing.quantity > 0 ? '+' : ''}${closing.quantity} ${closing.symbol} position at the last known price of ${closing.last_price.toFixed(2)}. Realized P&L will be based on this price.`
            : ''
        }
        confirmLabel={closingBusy ? 'Closing…' : 'Close position'}
        variant="destructive"
        onConfirm={handleClosePosition}
        onCancel={() => setClosing(null)}
      />

      <Card className="mb-8">
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle className="text-base">Trades</CardTitle>
            {pnl && (
              <p className="mt-1 text-xs text-muted-foreground">
                BUY {pnl.buy_orders} · SELL {pnl.sell_orders} · CLOSE {pnl.close_orders}
                {' · '}Filled {pnl.filled} · Rejected {pnl.rejected} · Cancelled {pnl.cancelled}
                {pnl.duplicate_suppressed > 0 && ` · Duplicate ignored ${pnl.duplicate_suppressed}`}
              </p>
            )}
          </div>
          {allOrders.length > 0 && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => downloadCsv(`/v1/paper-accounts/${id}/orders/export`, `paper-trades-${id}.csv`)}
            >
              Export CSV
            </Button>
          )}
        </CardHeader>
        <CardContent className="p-0">
          {allOrders.length === 0 ? (
            <p className="px-6 pb-6 text-sm text-muted-foreground">No signals received yet.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    <th className="px-4 py-3">Time</th>
                    <th className="px-4 py-3">Source</th>
                    <th className="px-4 py-3">Order ID</th>
                    <th className="px-4 py-3">Side</th>
                    <th className="px-4 py-3">Symbol</th>
                    <th className="px-4 py-3">Qty</th>
                    <th className="px-4 py-3">Fill Price</th>
                    <th className="px-4 py-3">Status</th>
                    <th className="px-4 py-3">Realized P&L</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {allOrders.map(o => (
                    <tr key={o.id} className="hover:bg-muted/30">
                      <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{fmtTime(o.created_at)}</td>
                      <td className="px-4 py-3 text-xs">Webhook</td>
                      <td className="px-4 py-3 font-mono text-xs text-muted-foreground" title={o.id}>
                        {shortOrderId(o.id)}
                      </td>
                      <td className="px-4 py-3"><ActionBadge action={o.side} /></td>
                      <td className="px-4 py-3 font-mono text-xs">{o.symbol}</td>
                      <td className="px-4 py-3 font-mono text-xs">{o.quantity || '—'}</td>
                      <td className="px-4 py-3 font-mono text-xs">{o.fill_price?.toFixed(2) ?? '—'}</td>
                      <td className="px-4 py-3"><StatusBadge status={o.status} /></td>
                      <td className={`px-4 py-3 font-mono text-xs ${o.realized_pnl == null ? 'text-muted-foreground' : pnlTone(o.realized_pnl) === 'positive' ? 'text-green-600 dark:text-green-500' : pnlTone(o.realized_pnl) === 'negative' ? 'text-destructive' : ''}`}>
                        {o.realized_pnl != null ? (
                          fmtPnL(o.realized_pnl)
                        ) : o.reason?.startsWith('unknown or inactive symbol') ||
                          o.reason?.includes('not available for paper trading') ? (
                          <div className="space-y-1.5 normal-case">
                            <p className="text-muted-foreground">
                              {account.exchange}:{o.symbol} is not available for paper trading.
                            </p>
                            <Link
                              href={`/requests/new?symbol=${encodeURIComponent(o.symbol)}&profile=${encodeURIComponent(account.market_profile)}&exchange=${encodeURIComponent(account.exchange)}`}
                              className="inline-flex text-xs font-medium text-primary underline"
                            >
                              Request instrument
                            </Link>
                          </div>
                        ) : (
                          o.reason || '—'
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {hasMoreOrders && (
            <div className="flex justify-center border-t p-4">
              <Button variant="outline" size="sm" onClick={() => setOffset(o => o + ORDERS_PAGE_SIZE)}>
                Load more ({allOrders.length} of {total})
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {/* 5. Diagnostics */}
      <SectionHeading title="Diagnostics" description="Webhook signal flow, market data, and account metadata." />
      <div className="mb-6 grid gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Webhook</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            {(pnl?.linked_webhooks?.length ?? 0) === 0 ? (
              <p className="text-muted-foreground">No webhook linked to this account.</p>
            ) : (
              pnl!.linked_webhooks!.map(wh => (
                <div key={wh.id} className="flex items-center justify-between">
                  <Link href={`/webhooks/${wh.id}`} className="font-medium text-primary hover:underline">
                    {wh.label || wh.id.slice(0, 8)}
                  </Link>
                  <StatusBadge status={wh.status} />
                </div>
              ))
            )}
            <div className="mt-3 grid grid-cols-2 gap-2 border-t pt-3 text-xs">
              <div><span className="text-muted-foreground">Signals received</span><p className="font-medium">{pnl?.signals_received ?? '—'}</p></div>
              <div><span className="text-muted-foreground">Accepted</span><p className="font-medium">{pnl?.signals_accepted ?? '—'}</p></div>
              <div><span className="text-muted-foreground">Rejected</span><p className="font-medium">{pnl?.signals_rejected ?? '—'}</p></div>
              <div><span className="text-muted-foreground">Duplicate ignored</span><p className="font-medium">{pnl?.duplicate_suppressed ?? '—'}</p></div>
              <div><span className="text-muted-foreground">Fill rate</span><p className="font-medium">{fmt(pnl?.webhook_fill_rate, true)}</p></div>
              <div><span className="text-muted-foreground">Last webhook</span><p className="font-medium">{pnl?.last_webhook_signal_at ? fmtTime(pnl.last_webhook_signal_at) : '—'}</p></div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Market</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <div className="flex justify-between"><span className="text-muted-foreground">Profile</span><span>{pnl?.market_profile_name ?? account.market_profile}</span></div>
            <div className="flex justify-between"><span className="text-muted-foreground">Exchange</span><span>{account.exchange}</span></div>
            <div className="flex justify-between"><span className="text-muted-foreground">Session</span><span className="capitalize">{pnl?.trading_session ?? '—'}</span></div>
            <div className="flex justify-between"><span className="text-muted-foreground">Last market update</span><span>{pnl?.last_market_update_at ? fmtTime(pnl.last_market_update_at) : '—'}</span></div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Account Metadata</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <div className="flex justify-between"><span className="text-muted-foreground">Created</span><span>{fmtDate(account.created_at)}</span></div>
            <div className="flex justify-between"><span className="text-muted-foreground">Starting balance</span><span>{fmtNotional(account.starting_balance)}</span></div>
            <div className="flex justify-between"><span className="text-muted-foreground">Currency</span><span>{account.base_currency}</span></div>
            <div className="flex justify-between"><span className="text-muted-foreground">Product</span><span>{account.default_product}</span></div>
            <div className="flex justify-between"><span className="text-muted-foreground">Exchange</span><span>{account.exchange}</span></div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
