'use client'

import { useState, useEffect } from 'react'
import useSWR from 'swr'
import Link from 'next/link'
import { Check, Copy, Pause, Pencil, Play, Plus, RotateCcw, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { useToast } from '@/components/ToastProvider'
import { apiFetch, INGEST_BASE_URL } from '@/lib/api'
import { useUser } from '@/lib/user-context'
import { cn } from '@/lib/utils'
import { formatCredentialLabel, formatPaperAccountLabel } from '@/lib/executionDestination'
import type { BrokerCredential, PaperAccount, Webhook } from '@/lib/types'

const BROKER_LABELS: Record<string, string> = {
  mt5_cloud: 'MT5',
  zerodha: 'Zerodha',
  angel: 'Angel',
  dhan: 'Dhan',
}

function destinationLabel(
  credMap: Map<string, BrokerCredential>,
  paperMap: Map<string, PaperAccount>,
  wh: Webhook,
): string {
  if (wh.paper_account_id) {
    return formatPaperAccountLabel(paperMap.get(wh.paper_account_id))
  }
  const credId = wh.broker_cred_id
  if (!credId) return '—'
  const c = credMap.get(credId)
  if (!c) return credId.slice(0, 8) + '…'
  // Just "Publisher" here — the live/demo account mode isn't worth the extra
  // width in a table column; the Status cell's "Manual Confirmation Required"
  // note already flags what matters about this destination.
  if (c.execution_mode === 'publisher') return 'Publisher'
  return formatCredentialLabel(c, { brokerLabels: BROKER_LABELS })
}

function isPublisherWebhook(credMap: Map<string, BrokerCredential>, wh: Webhook): boolean {
  if (!wh.broker_cred_id) return false
  return credMap.get(wh.broker_cred_id)?.execution_mode === 'publisher'
}

function TableSkeleton() {
  return (
    <div className="rounded-md border divide-y">
      {[1, 2, 3].map(i => (
        <div key={i} className="flex items-center gap-4 px-4 py-3.5">
          <Skeleton className="h-4 w-28" />
          <Skeleton className="h-4 w-16" />
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-5 w-14 rounded-full" />
          <Skeleton className="h-4 w-40 flex-1" />
          <div className="ml-auto flex gap-1">
            <Skeleton className="h-8 w-8 rounded-md" />
            <Skeleton className="h-8 w-8 rounded-md" />
            <Skeleton className="h-8 w-8 rounded-md" />
            <Skeleton className="h-8 w-8 rounded-md" />
            <Skeleton className="h-8 w-8 rounded-md" />
          </div>
        </div>
      ))}
    </div>
  )
}

export default function WebhooksPage() {
  const { user, refresh: refreshUser } = useUser()
  const { toast } = useToast()

  const { data: webhooks, mutate, isLoading } = useSWR<Webhook[]>(
    '/v1/webhooks',
    (url: string) => apiFetch<Webhook[]>(url),
  )
  const { data: creds } = useSWR<BrokerCredential[]>(
    '/v1/credentials',
    (url: string) => apiFetch<BrokerCredential[]>(url),
  )
  const { data: paperAccounts } = useSWR<PaperAccount[]>(
    '/v1/paper-accounts',
    (url: string) => apiFetch<PaperAccount[]>(url),
  )

  const list = webhooks ?? []
  const credMap = new Map((creds ?? []).map(c => [c.id, c]))
  const paperMap = new Map((paperAccounts ?? []).map(a => [a.id, a]))

  const [confirmDelete, setConfirmDelete] = useState<{ id: string; label: string } | null>(null)
  const [confirmRotate, setConfirmRotate] = useState<{ id: string; label: string } | null>(null)
  const [newToken, setNewToken] = useState<Record<string, string>>({})
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)

  // Recover the raw token from a just-created webhook (WebhookForm stores it in sessionStorage
  // because the list endpoint never returns the raw token — only creation/rotation do).
  useEffect(() => {
    try {
      const stored = sessionStorage.getItem('newWebhookToken')
      if (stored) {
        const { id, token } = JSON.parse(stored) as { id: string; token: string }
        sessionStorage.removeItem('newWebhookToken')
        if (id && token) setNewToken(prev => ({ ...prev, [id]: token }))
      }
    } catch { /* ignore */ }
  }, [])

  const paperCount = list.filter(w => w.paper_account_id).length
  const liveCount = list.filter(w => w.broker_cred_id).length
  const canAddPaper = paperCount < user.max_paper_webhooks
  const canAddLive =
    user.live_trading_allowed && liveCount < user.max_live_webhooks
  const canAddAny = canAddPaper || canAddLive

  async function togglePause(wh: Webhook) {
    setBusyId(wh.id)
    try {
      const path =
        wh.status === 'active'
          ? `/v1/webhooks/${wh.id}/pause`
          : `/v1/webhooks/${wh.id}/resume`
      await apiFetch(path, { method: 'PUT' })
      await mutate()
      refreshUser()
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to update status', 'error')
    } finally {
      setBusyId(null)
    }
  }

  async function handleRotate(id: string) {
    setBusyId(id)
    try {
      const updated = await apiFetch<Webhook>(`/v1/webhooks/${id}/rotate-token`, {
        method: 'POST',
      })
      setNewToken(prev => ({ ...prev, [id]: updated.token! }))
      setConfirmRotate(null)
      await mutate()
      toast('Token rotated — copy your new webhook URL', 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to rotate token', 'error')
    } finally {
      setBusyId(null)
    }
  }

  async function handleDelete(id: string) {
    setBusyId(id)
    try {
      await apiFetch(`/v1/webhooks/${id}`, { method: 'DELETE' })
      setConfirmDelete(null)
      await mutate()
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to delete webhook', 'error')
    } finally {
      setBusyId(null)
    }
  }

  async function copyUrl(wh: Webhook) {
    const token = newToken[wh.id] ?? wh.token
    if (!token) {
      toast('Token not available — rotate the token to get the webhook URL', 'error')
      return
    }
    const url = `${INGEST_BASE_URL}/v1/webhook/${token}`
    await navigator.clipboard.writeText(url)
    setCopiedId(wh.id)
    setTimeout(() => setCopiedId(id => (id === wh.id ? null : id)), 2000)
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Webhooks</h1>
        <div className="flex items-center gap-3">
          {!canAddAny && (
            <span className="text-sm text-amber-600">
              Plan limit reached —{' '}
              <Link href="/billing" className="underline">
                upgrade
              </Link>
            </span>
          )}
          {!canAddAny ? (
            <Button disabled>
              <Plus className="h-4 w-4" /> New Webhook
            </Button>
          ) : (
            <Button asChild>
              <Link href="/webhooks/new">
                <Plus className="h-4 w-4" /> New Webhook
              </Link>
            </Button>
          )}
        </div>
      </div>

      {isLoading ? (
        <TableSkeleton />
      ) : list.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="py-16 text-center text-sm text-muted-foreground">
            No webhooks yet.{' '}
            <Link href="/webhooks/new" className="text-primary underline">
              Create your first webhook
            </Link>{' '}
            to start receiving TradingView signals.
          </CardContent>
        </Card>
      ) : (
        <div className="rounded-md border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">
                <th className="px-4 py-3">Webhook name</th>
                <th className="px-4 py-3">Allowed symbols</th>
                <th className="px-4 py-3">Execution destination</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Webhook URL</th>
                <th className="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {list.map(wh => {
                const displayToken = newToken[wh.id] ?? wh.token
                const busy = busyId === wh.id
                return (
                  <tr key={wh.id} className="hover:bg-muted/30">
                    <td className="px-4 py-3 font-medium">
                      <Link href={`/webhooks/${wh.id}`} className="text-primary hover:underline">
                        {wh.label || (
                          <span className="italic text-muted-foreground">Unnamed</span>
                        )}
                      </Link>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs">
                      {(wh.allowed_symbols?.length ? wh.allowed_symbols.join(', ') : wh.symbol) || '—'}
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {destinationLabel(credMap, paperMap, wh)}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-col gap-1">
                        <Badge
                          className={cn(
                            'w-fit gap-1.5 font-medium border-transparent',
                            wh.status === 'active'
                              ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300'
                              : 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
                          )}
                        >
                          <span
                            className={cn(
                              'h-1.5 w-1.5 rounded-full',
                              wh.status === 'active' ? 'bg-emerald-500' : 'bg-amber-500',
                            )}
                            aria-hidden
                          />
                          {wh.status === 'active' ? 'Active' : wh.status === 'paused' ? 'Paused' : wh.status}
                        </Badge>
                        {isPublisherWebhook(credMap, wh) && (
                          <span className="text-xs text-amber-600 dark:text-amber-400">
                            Manual Confirmation Required
                          </span>
                        )}
                        {wh.admin_disabled && (
                          <span className="text-xs text-amber-600 dark:text-amber-400">
                            Disabled by admin
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      {displayToken ? (
                        <Button
                          variant="outline"
                          size="sm"
                          title="Copy webhook URL"
                          onClick={() => copyUrl(wh)}
                          className={newToken[wh.id] ? 'border-green-400 text-green-700' : ''}
                        >
                          {copiedId === wh.id ? (
                            <Check className="mr-1.5 h-3.5 w-3.5 text-green-600" />
                          ) : (
                            <Copy className="mr-1.5 h-3.5 w-3.5" />
                          )}
                          {copiedId === wh.id ? 'Copied' : 'Copy URL'}
                        </Button>
                      ) : (
                        <span className="text-xs text-muted-foreground">Rotate to reveal</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="icon"
                          title={
                            wh.status === 'active'
                              ? 'Pause webhook'
                              : wh.admin_disabled
                                ? 'This execution mode is disabled by the ZettaBridge admin — cannot resume'
                                : 'Resume webhook'
                          }
                          disabled={busy || (wh.status === 'paused' && wh.admin_disabled)}
                          onClick={() => togglePause(wh)}
                        >
                          {wh.status === 'active' ? (
                            <Pause className="h-4 w-4" />
                          ) : (
                            <Play className="h-4 w-4" />
                          )}
                        </Button>

                        <Button
                          variant="ghost"
                          size="icon"
                          title="Rotate webhook token"
                          disabled={busy}
                          onClick={() =>
                            setConfirmRotate({ id: wh.id, label: wh.label || 'Unnamed' })
                          }
                        >
                          <RotateCcw className="h-4 w-4" />
                        </Button>

                        <Button variant="ghost" size="icon" title="Edit webhook" asChild>
                          <Link href={`/webhooks/${wh.id}/edit`}>
                            <Pencil className="h-4 w-4" />
                          </Link>
                        </Button>

                        <Button
                          variant="ghost"
                          size="icon"
                          title="Delete webhook"
                          disabled={busy}
                          className="text-destructive hover:text-destructive"
                          onClick={() =>
                            setConfirmDelete({ id: wh.id, label: wh.label || 'Unnamed' })
                          }
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      <ConfirmDialog
        open={!!confirmRotate}
        title="Rotate webhook token?"
        description={`This invalidates the current token for "${confirmRotate?.label}". Any TradingView alerts using the old URL will stop working immediately. Copy the new URL after rotating.`}
        confirmLabel="Rotate token"
        onConfirm={() => confirmRotate && handleRotate(confirmRotate.id)}
        onCancel={() => setConfirmRotate(null)}
      />

      <ConfirmDialog
        open={!!confirmDelete}
        title="Delete webhook?"
        description={`"${confirmDelete?.label}" and all its trade history will be permanently deleted. This cannot be undone.`}
        confirmLabel="Delete"
        variant="destructive"
        onConfirm={() => confirmDelete && handleDelete(confirmDelete.id)}
        onCancel={() => setConfirmDelete(null)}
      />
    </div>
  )
}
