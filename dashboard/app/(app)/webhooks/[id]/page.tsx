'use client'

import { useEffect, useRef, useState } from 'react'
import { useParams } from 'next/navigation'
import Link from 'next/link'
import { ArrowLeft, ExternalLink, Loader2, Rocket } from 'lucide-react'
import useSWR from 'swr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { apiFetch, getWebhookPnL, downloadCsv } from '@/lib/api'
import { getToken } from '@/lib/auth'
import { useUser } from '@/lib/user-context'
import { cn } from '@/lib/utils'
import { createTradeSocket, type WsStatus } from '@/lib/ws'
import { isPublisherHandoffExpired, publisherHandoffExpiryLabel } from '@/lib/publisher'
import { PnLSummaryCard } from '@/components/PnLSummary'
import { ActionBadge } from '@/components/ActionBadge'
import { StatusBadge } from '@/components/StatusBadge'
import { WsIndicator } from '@/components/WsIndicator'
import type { BrokerCredential, PnLSummary, Trade, Webhook } from '@/lib/types'

type Tab = 'trades' | 'pnl'

function fmtTime(ts: string) {
  return new Date(ts).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function fmtExpiry(expiresAt: string | undefined): string | null {
  return publisherHandoffExpiryLabel(expiresAt ?? undefined)
}

function fmtTradeField(v: string | undefined): string {
  if (!v) return '—'
  return v.toUpperCase()
}

const WS_BROKER_LABELS: Record<string, string> = {
  mt5_cloud: 'MetaTrader 5',
  angel: 'Angel One',
  dhan: 'Dhan',
}

// Label shown on the WsIndicator pill while the live-trade feed is
// connected — names the actual execution destination (e.g. "Zerodha OAuth ·
// Live") instead of a generic "Live", so it reads as broker/destination
// status rather than an unlabeled websocket ping.
function wsConnectedLabel(pnl: PnLSummary | undefined, isPublisher: boolean): string {
  if (pnl?.broker_type === 'zerodha') {
    return isPublisher ? 'Zerodha Kite Publisher · Live' : 'Zerodha OAuth · Live'
  }
  if (pnl?.broker_type) {
    return `${WS_BROKER_LABELS[pnl.broker_type] ?? pnl.broker_type} · Live`
  }
  if (pnl?.market_profile_name) {
    return `Paper Trading · Live`
  }
  return 'Live'
}

function fmtRelative(ts: string | undefined): string {
  if (!ts) return 'Never used'
  const diffMs = Date.now() - new Date(ts).getTime()
  const mins = Math.floor(diffMs / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins} min ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

export default function WebhookDetailPage() {
  const params = useParams()
  const id = params.id as string
  const { user } = useUser()
  const isFree = user.plan === 'free'

  const [webhook, setWebhook] = useState<Webhook | null>(null)
  const [trades, setTrades] = useState<Trade[]>([])
  const [wsStatus, setWsStatus] = useState<WsStatus>('connecting')
  const [loading, setLoading] = useState(true)
  const [tab, setTab] = useState<Tab>('trades')
  const socketRef = useRef<ReturnType<typeof createTradeSocket> | null>(null)

  const { data: pnl, isLoading: pnlLoading } = useSWR(
    id ? `/v1/webhooks/${id}/pnl` : null,
    () => getWebhookPnL(id),
  )

  const { data: creds } = useSWR<BrokerCredential[]>(
    '/v1/credentials',
    (url: string) => apiFetch<BrokerCredential[]>(url),
  )

  const isPublisher = !!(webhook && creds?.find(c => c.id === webhook.broker_cred_id)?.execution_mode === 'publisher')
  const isPaperDestination = !!webhook?.paper_account_id
  const canLive = user.live_trading_allowed

  useEffect(() => {
    async function load() {
      const wh = await apiFetch<Webhook>(`/v1/webhooks/${id}`)
      setWebhook(wh ?? null)

      try {
        const ts = await apiFetch<Trade[]>(`/v1/webhooks/${id}/trades`)
        setTrades(Array.isArray(ts) ? ts : [])
      } catch {
        // ignore — trades remain empty
      }
      setLoading(false)
    }
    load().catch(() => setLoading(false))
  }, [id])

  useEffect(() => {
    const token = getToken()
    if (!token) return

    const socket = createTradeSocket(
      token,
      (trade) => {
        if (trade.webhook_id !== id) return
        // The backend re-emits this event for status transitions on an
        // existing trade (e.g. submitted -> filled via the Zerodha fill-poll
        // job), not just on creation — same trade id, updated fields. Update
        // that row in place instead of prepending a duplicate.
        setTrades(prev => {
          const idx = prev.findIndex(t => t.id === trade.id)
          if (idx === -1) return [trade, ...prev].slice(0, 200)
          const next = [...prev]
          next[idx] = trade
          return next
        })
      },
      setWsStatus,
    )
    socket.connect()
    socketRef.current = socket

    return () => socket.disconnect()
  }, [id])

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    )
  }

  if (!webhook) {
    return (
      <div className="text-sm text-muted-foreground">Webhook not found.</div>
    )
  }

  return (
    <div>
      <div className="mb-6 flex items-start justify-between">
        <div>
          <Link
            href="/webhooks"
            className="mb-2 flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
          >
            <ArrowLeft className="h-3.5 w-3.5" /> Webhooks
          </Link>
          <h1 className="flex items-center gap-3 text-2xl font-semibold">
            {webhook.label || <span className="italic text-muted-foreground">Unnamed</span>}
            <Badge
              className={cn(
                'gap-1.5 font-medium border-transparent',
                webhook.status === 'active'
                  ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300'
                  : 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
              )}
            >
              <span
                className={cn(
                  'h-1.5 w-1.5 rounded-full',
                  webhook.status === 'active' ? 'bg-emerald-500' : 'bg-amber-500',
                )}
                aria-hidden
              />
              {webhook.status === 'active' ? 'Active' : 'Paused'}
            </Badge>
            {isPublisher && (() => {
              const pubCred = creds?.find(c => c.id === webhook.broker_cred_id)
              return (
                <Badge variant="outline" className="text-xs font-normal text-muted-foreground">
                  Kite Publisher · {pubCred?.account_mode === 'live' ? 'Live' : 'Demo'}
                </Badge>
              )
            })()}
          </h1>
          <p className="mt-1 font-mono text-sm text-muted-foreground">
            {(webhook.allowed_symbols?.length ? webhook.allowed_symbols.join(', ') : webhook.symbol) || '—'}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            Last signal: {pnlLoading ? '…' : fmtRelative(pnl?.last_signal_at)}
          </p>
        </div>

        <WsIndicator status={wsStatus} connectedLabel={wsConnectedLabel(pnl, isPublisher)} />
      </div>

      {isPaperDestination && (
        <div className="mb-6 flex items-center justify-between gap-4 rounded-md border px-4 py-3">
          <div className="flex items-center gap-2 text-sm">
            <Rocket className="h-4 w-4 shrink-0 text-muted-foreground" />
            {canLive
              ? "Go live with this configuration — connect a real broker and clone this webhook's settings."
              : 'Upgrade your plan to connect a live broker and trade real money with this configuration.'}
          </div>
          <Button asChild variant="outline" size="sm">
            <Link href={canLive ? `/webhooks/new?cloneFrom=${webhook.id}` : '/billing'}>
              {canLive ? 'Go Live' : 'Upgrade'}
            </Link>
          </Button>
        </div>
      )}

      {/* Tab bar */}
      <div className="mb-4 flex gap-1 border-b">
        {(['trades', 'pnl'] as Tab[]).map(t => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`px-4 py-2 text-sm font-medium capitalize transition-colors border-b-2 -mb-px ${
              tab === t
                ? 'border-primary text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            {t === 'pnl' ? 'Summary' : isPublisher ? 'Handoffs' : 'Trades'}
          </button>
        ))}
      </div>

      {tab === 'pnl' && (
        <PnLSummaryCard summary={pnl} loading={pnlLoading} publisherMode={isPublisher} />
      )}

      {tab === 'trades' && (
        <>
          {isPublisher && trades.length > 0 && (
            <p className="mb-3 text-xs text-muted-foreground">
              Kite Publisher mode creates basket handoffs. Final execution happens only after you review and submit the basket on Kite.
            </p>
          )}
          <div className="rounded-md border">
            {trades.length === 0 ? (
              <div className="px-6 py-10 text-center text-sm text-muted-foreground">
                {isPublisher
                  ? 'No signals yet. Kite Publisher basket handoffs will appear here in real time.'
                  : isPaperDestination
                    ? 'No paper trades yet. Simulated fills from incoming signals will appear here in real time.'
                    : 'No trades yet. Signals sent to this webhook will appear here in real time.'}
              </div>
            ) : (
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    <th className="px-4 py-3">Time</th>
                    <th className="px-4 py-3">Side</th>
                    <th className="px-4 py-3">Symbol</th>
                    <th className="px-4 py-3">Qty</th>
                    <th className="px-4 py-3">Order Type</th>
                    <th className="px-4 py-3">Product</th>
                    <th className="px-4 py-3">Status</th>
                    {isPublisher && <th className="px-4 py-3">Expires</th>}
                    <th className="px-4 py-3">Error</th>
                    {isPublisher && <th className="px-4 py-3">Next step</th>}
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {trades.map(t => {
                    const hasError = !!(t.error_code || t.error)
                    const errorLabel = t.error_code && t.error
                      ? `[${t.error_code}] ${t.error}`
                      : t.error_code ?? t.error ?? ''
                    return (
                    <tr key={t.id} className="hover:bg-muted/30">
                      <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                        {fmtTime(t.created_at)}
                      </td>
                      <td className="px-4 py-3">
                        <ActionBadge action={t.signal} />
                      </td>
                      <td className="px-4 py-3 font-mono text-xs">{t.symbol}</td>
                      <td className="px-4 py-3 font-mono text-xs">{t.lot_size || '—'}</td>
                      <td className="px-4 py-3 font-mono text-xs">{fmtTradeField(t.order_type)}</td>
                      <td className="px-4 py-3 font-mono text-xs">{fmtTradeField(t.product)}</td>
                      <td className="px-4 py-3">
                        <StatusBadge status={t.status} />
                      </td>
                      {isPublisher && (
                        <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                          {t.status === 'pending_confirmation'
                            ? (fmtExpiry(t.publisher_order_expires_at) ?? '—')
                            : '—'}
                        </td>
                      )}
                      <td className="px-4 py-3 font-mono text-xs max-w-xs">
                        {hasError ? (
                          <span
                            className="text-destructive break-words"
                            title={errorLabel}
                          >
                            {errorLabel.length > 80 ? errorLabel.slice(0, 80) + '…' : errorLabel}
                          </span>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </td>
                      {isPublisher && (
                        <td className="px-4 py-3 text-xs">
                          {t.status === 'pending_confirmation' && !isPublisherHandoffExpired(t) ? (
                            <Link
                              href={`/trades/${t.id}`}
                              className="text-primary underline inline-flex items-center gap-0.5"
                            >
                              Review &amp; Submit on Kite <ExternalLink className="h-3 w-3" />
                            </Link>
                          ) : t.status === 'pending_confirmation' || t.error_code === 'publisher_handoff_expired' ? (
                            <span className="text-destructive">Expired — missed</span>
                          ) : (
                            <span className="text-muted-foreground">—</span>
                          )}
                        </td>
                      )}
                    </tr>
                    )
                  })}
                </tbody>
              </table>
            )}
          </div>

          {isFree && !isPaperDestination && (
            <p className="mt-2 text-xs text-muted-foreground">
              Showing last 20 trades.{' '}
              <Link href="/billing" className="text-primary underline">
                Upgrade
              </Link>{' '}
              for full history and export.
            </p>
          )}

          {trades.length > 0 && (
            <div className="mt-3">
              <Button
                variant="outline"
                size="sm"
                onClick={() => downloadCsv(`/v1/webhooks/${id}/trades/export`, `trades-${id}.csv`)}
              >
                Export CSV
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
