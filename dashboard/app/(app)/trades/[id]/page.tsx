'use client'

import { useEffect, useState } from 'react'
import { useParams, useSearchParams } from 'next/navigation'
import Link from 'next/link'
import { ArrowLeft, CheckCircle2, Clock, ExternalLink, Loader2, XCircle } from 'lucide-react'
import { apiFetch, ApiError } from '@/lib/api'
import { isPublisherHandoffExpired, publisherHandoffExpiryLabel } from '@/lib/publisher'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import type { Trade, PublisherOrder } from '@/lib/types'

function fmtTime(ts: string) {
  return new Date(ts).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function ActionBadge({ action }: { action: string }) {
  const variant =
    action === 'BUY' ? 'default' : action === 'SELL' ? 'destructive' : 'secondary'
  return <Badge variant={variant}>{action}</Badge>
}

function StatusBadge({ status, isPublisher }: { status: string; isPublisher?: boolean }) {
  switch (status) {
    case 'filled':
      return <Badge variant="default">Filled</Badge>
    case 'submitted':
      return <Badge variant="secondary">{isPublisher ? 'Submitted — execution unknown' : 'Submitted'}</Badge>
    case 'rejected':
      return <Badge variant="destructive">Rejected</Badge>
    case 'cancelled':
      return <Badge variant="outline">Cancelled</Badge>
    case 'pending_confirmation':
      return <Badge variant="warning">Awaiting Kite confirmation</Badge>
    default:
      return <Badge variant="outline">{status}</Badge>
  }
}

function isPublisherTrade(trade: Trade): boolean {
  if (trade.status === 'pending_confirmation') return true
  // Publisher order IDs are UUIDs; Zerodha broker order IDs are numeric strings
  if (trade.broker_order && /^[0-9a-f]{8}-[0-9a-f]{4}-/.test(trade.broker_order)) return true
  return false
}

function postFormToKite(action: string, apiKey: string, data: string, state: string) {
  // Submit in the same window so Kite's "Finish" redirect returns cleanly to
  // ZettaBridge's callback URL. Opening in _blank breaks the connect session.
  const form = document.createElement('form')
  form.method = 'POST'
  form.action = action

  const addField = (name: string, value: string) => {
    const input = document.createElement('input')
    input.type = 'hidden'
    input.name = name
    input.value = value
    form.appendChild(input)
  }

  addField('api_key', apiKey)
  addField('data', data)
  addField('state', state)

  document.body.appendChild(form)
  form.submit()
  document.body.removeChild(form)
}

function PublisherExpiredBanner() {
  return (
    <div className="flex items-start gap-3 rounded-lg border border-red-200 bg-red-50/60 dark:border-red-800 dark:bg-red-950/30 px-4 py-3">
      <XCircle className="h-5 w-5 text-red-600 dark:text-red-400 shrink-0 mt-0.5" />
      <div>
        <p className="text-sm font-medium text-red-800 dark:text-red-300">Basket expired — missed</p>
        <p className="text-xs text-red-700/80 dark:text-red-400/80 mt-0.5">
          This Kite basket handoff expired before submission. It cannot be opened on Kite anymore. Wait for a new TradingView signal to receive a fresh handoff.
        </p>
      </div>
    </div>
  )
}

function PublisherResultBanner({ status }: { status: string }) {
  if (status === 'submitted') {
    return (
      <div className="flex items-start gap-3 rounded-lg border border-green-200 bg-green-50/60 dark:border-green-800 dark:bg-green-950/30 px-4 py-3">
        <CheckCircle2 className="h-5 w-5 text-green-600 dark:text-green-400 shrink-0 mt-0.5" />
        <div>
          <p className="text-sm font-medium text-green-800 dark:text-green-300">Basket submitted on Kite</p>
          <p className="text-xs text-green-700/80 dark:text-green-400/80 mt-0.5">
            ZettaBridge received the Kite return confirmation. Final order status, fill price, rejection reason, and P&amp;L are not available in Publisher mode. Please check Kite for execution details.
          </p>
        </div>
      </div>
    )
  }
  if (status === 'cancelled') {
    return (
      <div className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50/60 dark:border-amber-800 dark:bg-amber-950/30 px-4 py-3">
        <XCircle className="h-5 w-5 text-amber-600 dark:text-amber-400 shrink-0 mt-0.5" />
        <div>
          <p className="text-sm font-medium text-amber-800 dark:text-amber-300">Basket cancelled on Kite</p>
          <p className="text-xs text-amber-700/80 dark:text-amber-400/80 mt-0.5">
            You cancelled the basket on Kite — no orders were placed.
          </p>
        </div>
      </div>
    )
  }
  if (status === 'pending_confirmation') {
    return (
      <div className="flex items-start gap-3 rounded-lg border border-blue-200 bg-blue-50/60 dark:border-blue-800 dark:bg-blue-950/30 px-4 py-3">
        <Clock className="h-5 w-5 text-blue-600 dark:text-blue-400 shrink-0 mt-0.5" />
        <div>
          <p className="text-sm font-medium text-blue-800 dark:text-blue-300">Returned without submitting</p>
          <p className="text-xs text-blue-700/80 dark:text-blue-400/80 mt-0.5">
            You returned without submitting the basket. You can still open Kite below to review and submit.
          </p>
        </div>
      </div>
    )
  }
  return null
}

export default function TradePage() {
  const params = useParams()
  const searchParams = useSearchParams()
  const tradeId = params.id as string
  const fromPublisher = searchParams.get('publisher') === '1'

  const [trade, setTrade] = useState<Trade | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [opening, setOpening] = useState(false)
  const [handoffError, setHandoffError] = useState<string | null>(null)
  const [showMarkSubmitted, setShowMarkSubmitted] = useState(false)
  const [markingSubmitted, setMarkingSubmitted] = useState(false)
  useEffect(() => {
    apiFetch<Trade>(`/v1/trades/${tradeId}`)
      .then(t => setTrade(t))
      .catch(err => setLoadError(err instanceof Error ? err.message : 'Failed to load trade'))
      .finally(() => setLoading(false))
  }, [tradeId])

  async function handleOpenKite() {
    if (!trade?.broker_order) return
    setHandoffError(null)
    setOpening(true)
    try {
      const order = await apiFetch<PublisherOrder>(`/v1/publisher/orders/${trade.broker_order}`)
      const fields = order.handoff?.fields
      if (!fields?.api_key || !fields?.data || !fields?.state) {
        throw new Error('Order is not ready for confirmation — please try again shortly.')
      }
      postFormToKite(order.handoff!.action, fields.api_key, fields.data, fields.state)
    } catch (err) {
      if (err instanceof ApiError && err.status === 410) {
        setHandoffError(err.message)
        setTrade(prev => prev ? {
          ...prev,
          status: 'rejected',
          error_code: 'publisher_handoff_expired',
          error: err.message,
        } : prev)
      } else {
        setHandoffError(err instanceof Error ? err.message : 'Failed to load order — it may have expired.')
      }
    } finally {
      setOpening(false)
    }
  }

  async function handleMarkSubmitted() {
    if (!trade?.broker_order) return
    setShowMarkSubmitted(false)
    setMarkingSubmitted(true)
    setHandoffError(null)
    try {
      await apiFetch(`/v1/publisher/orders/${trade.broker_order}/mark-submitted`, { method: 'POST' })
      setTrade(prev => prev ? { ...prev, status: 'submitted' } : prev)
    } catch (err) {
      setHandoffError(err instanceof Error ? err.message : 'Could not update this order — please refresh and try again.')
    } finally {
      setMarkingSubmitted(false)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    )
  }

  if (loadError || !trade) {
    return (
      <div className="space-y-4">
        <Link href="/webhooks" className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="h-3.5 w-3.5" /> Webhooks
        </Link>
        <p className="text-sm text-destructive">{loadError ?? 'Trade not found.'}</p>
      </div>
    )
  }

  const isPending = trade.status === 'pending_confirmation'
  const isPublisher = isPublisherTrade(trade)
  const handoffExpired = isPublisher && isPublisherHandoffExpired(trade)
  const canOpenKite = isPending && !handoffExpired

  return (
    <div className="max-w-2xl space-y-6">
      {/* Breadcrumb */}
      <div>
        {trade.webhook_id ? (
          <Link
            href={`/webhooks/${trade.webhook_id}`}
            className="mb-2 flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            {trade.webhook_label || 'Webhook'}
          </Link>
        ) : (
          <Link href="/webhooks" className="mb-2 flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
            <ArrowLeft className="h-3.5 w-3.5" /> Webhooks
          </Link>
        )}
        <h1 className="flex items-center gap-3 text-2xl font-semibold">
          {isPublisher ? 'Kite Basket Handoff' : 'Trade'}
          <StatusBadge status={trade.status} />
        </h1>
        <p className="mt-1 text-xs text-muted-foreground font-mono">{trade.id}</p>
      </div>

      {/* Publisher result banner (shown after returning from Kite callback) */}
      {fromPublisher && !handoffExpired && <PublisherResultBanner status={trade.status} />}
      {handoffExpired && <PublisherExpiredBanner />}

      {/* Pending confirmation: open Kite handoff */}
      {canOpenKite && trade.broker_order && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Review &amp; Submit on Kite</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <p className="text-sm text-muted-foreground">
              This Kite basket is waiting for your review. Open Kite to review and submit the order on Zerodha.
              {trade.publisher_order_expires_at && (
                <> Expires in {publisherHandoffExpiryLabel(trade.publisher_order_expires_at)}.</>
              )}
            </p>
            {handoffError && (
              <p className="text-sm text-destructive">{handoffError}</p>
            )}
            <div className="flex items-center gap-2">
              <Button
                onClick={handleOpenKite}
                disabled={opening}
              >
                {opening
                  ? <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
                  : <ExternalLink className="mr-1.5 h-4 w-4" />}
                {opening ? 'Redirecting to Kite…' : 'Review & Submit on Kite'}
              </Button>
              <Button
                variant="outline"
                onClick={() => setShowMarkSubmitted(true)}
                disabled={markingSubmitted}
              >
                {markingSubmitted
                  ? <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
                  : <CheckCircle2 className="mr-1.5 h-4 w-4" />}
                Already submitted on Kite
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              If Kite already placed your order but didn&apos;t return you here (e.g. a
              &quot;connect session&quot; error on Kite&apos;s own page after submitting), use
              &quot;Already submitted on Kite&quot; so this doesn&apos;t stay stuck as pending.
            </p>
          </CardContent>
        </Card>
      )}

      <ConfirmDialog
        open={showMarkSubmitted}
        title="Mark this order as submitted?"
        description="Only confirm this if you already clicked Submit/Finish on Kite's basket page. ZettaBridge can't verify this independently in Publisher mode — check Kite's order book for the actual fill status."
        confirmLabel="Yes, I submitted it"
        onConfirm={handleMarkSubmitted}
        onCancel={() => setShowMarkSubmitted(false)}
      />

      {/* Basket / Trade details */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">{isPublisher ? 'Basket details' : 'Trade details'}</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm">
            <div>
              <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Side</dt>
              <dd className="mt-1"><ActionBadge action={trade.signal} /></dd>
            </div>
            <div>
              <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Status</dt>
              <dd className="mt-1"><StatusBadge status={trade.status} isPublisher={isPublisher} /></dd>
            </div>
            <div>
              <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Symbol</dt>
              <dd className="mt-1 font-mono">{trade.symbol || '—'}</dd>
            </div>
            <div>
              <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Qty</dt>
              <dd className="mt-1 font-mono">{trade.lot_size ?? '—'}</dd>
            </div>
            {trade.fill_price ? (
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Fill price</dt>
                <dd className="mt-1 font-mono">{trade.fill_price}</dd>
              </div>
            ) : null}
            <div>
              <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Time</dt>
              <dd className="mt-1 font-mono text-xs">{fmtTime(trade.created_at)}</dd>
            </div>
            {trade.broker_order ? (
              <div className="col-span-2">
                <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {isPublisher ? 'ZettaBridge Handoff ID' : 'Broker order ID'}
                </dt>
                <dd className="mt-1 font-mono text-xs break-all">{trade.broker_order}</dd>
              </div>
            ) : null}
            {trade.error ? (
              <div className="col-span-2">
                <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Error</dt>
                <dd className="mt-1 text-destructive text-xs">
                  {trade.error_code ? `[${trade.error_code}] ` : ''}{trade.error}
                </dd>
              </div>
            ) : null}
          </dl>
        </CardContent>
      </Card>

      {/* Publisher limitation note */}
      {isPublisher && (
        <p className="text-xs text-muted-foreground">
          Publisher mode does not return fill price, fill status, rejection reason, or P&amp;L. After submission on Kite, ZettaBridge records the handoff as submitted with execution status unknown.
        </p>
      )}
    </div>
  )
}
