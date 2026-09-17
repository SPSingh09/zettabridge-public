'use client'

import { useEffect, useState } from 'react'
import useSWR from 'swr'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { CheckCircle, Loader2, Pause, Pencil, Play, Plus, RefreshCw, Trash2, XCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { useToast } from '@/components/ToastProvider'
import { formatCredentialProducts } from '@/lib/credentialProducts'
import { apiFetch } from '@/lib/api'
import { useUser } from '@/lib/user-context'
import { useZerodhaStatus } from '@/components/ZerodhaReconnectBanner'
import type { BrokerCredential, VerifyResult } from '@/lib/types'

const BROKER_LABELS: Record<string, string> = {
  mt5_cloud: 'MetaTrader 5',
  zerodha: 'Zerodha',
  angel: 'Angel One',
  dhan: 'Dhan',
}

const EXECUTION_MODE_LABELS: Record<string, string> = {
  publisher: 'Kite Publisher',
  user_api_oauth: 'OAuth',
}

type VerifyState = VerifyResult | 'loading' | 'error'

function TableSkeleton() {
  return (
    <div className="rounded-md border divide-y">
      {[1, 2, 3].map(i => (
        <div key={i} className="flex items-center gap-4 px-4 py-3.5">
          <Skeleton className="h-4 w-28" />
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-5 w-14 rounded-full" />
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-4 w-16" />
          <div className="ml-auto flex gap-2">
            <Skeleton className="h-8 w-16 rounded-md" />
            <Skeleton className="h-8 w-8 rounded-md" />
            <Skeleton className="h-8 w-8 rounded-md" />
          </div>
        </div>
      ))}
    </div>
  )
}

export default function CredentialsPage() {
  const { user, refresh: refreshUser } = useUser()
  const { toast } = useToast()
  const router = useRouter()
  const searchParams = useSearchParams()

  const { data: creds, mutate: mutateCreds, isLoading } = useSWR<BrokerCredential[]>(
    '/v1/credentials',
    (url: string) => apiFetch<BrokerCredential[]>(url),
  )

  const { data: zerodhaStatus, mutate: mutateZerodhaStatus } = useZerodhaStatus()

  useEffect(() => {
    const zerodha = searchParams.get('zerodha')
    const reason = searchParams.get('reason')
    if (!zerodha) return
    // Clear the param first so a re-render doesn't reprocess it.
    router.replace('/credentials')
    if (zerodha === 'connected' || zerodha === 'reconnected') {
      const msg = zerodha === 'reconnected'
        ? 'Zerodha reconnected — token refreshed'
        : 'Zerodha account connected successfully'
      toast(msg, 'success')
      mutateCreds()
      mutateZerodhaStatus()
    } else if (zerodha === 'error') {
      toast(`Zerodha connection failed: ${reason ?? 'unknown error'}`, 'error')
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const list = creds ?? []

  const [confirmDelete, setConfirmDelete] = useState<{ id: string; label: string } | null>(null)
  const [verifyState, setVerifyState] = useState<Record<string, VerifyState>>({})
  const [busyId, setBusyId] = useState<string | null>(null)
  const [reconnectingId, setReconnectingId] = useState<string | null>(null)

  const atLimit = list.length >= user.max_broker_creds

  // Persist verify results across page loads. Key includes connected_at so
  // Zerodha OAuth results are invalidated automatically after a token reconnect.
  function verifyStorageKey(cred: BrokerCredential) {
    return `zb_verify_${cred.id}_${cred.connected_at ?? 'none'}`
  }

  useEffect(() => {
    if (!creds || creds.length === 0) return
    const persisted: Record<string, VerifyState> = {}
    for (const cred of creds) {
      const raw = localStorage.getItem(verifyStorageKey(cred))
      if (!raw) continue
      try { persisted[cred.id] = JSON.parse(raw) } catch { /* ignore */ }
    }
    if (Object.keys(persisted).length > 0) {
      setVerifyState(prev => ({ ...persisted, ...prev }))
    }
  }, [creds])

  async function handleVerify(id: string) {
    setVerifyState(prev => ({ ...prev, [id]: 'loading' }))
    try {
      const result = await apiFetch<VerifyResult>(`/v1/credentials/${id}/verify`, {
        method: 'POST',
      })
      setVerifyState(prev => ({ ...prev, [id]: result }))
      const cred = creds?.find(c => c.id === id)
      if (cred) localStorage.setItem(verifyStorageKey(cred), JSON.stringify(result))
    } catch {
      setVerifyState(prev => ({ ...prev, [id]: 'error' }))
    }
  }

  async function handleTogglePause(cred: BrokerCredential) {
    setBusyId(cred.id)
    try {
      const path =
        cred.status === 'active'
          ? `/v1/credentials/${cred.id}/pause`
          : `/v1/credentials/${cred.id}/resume`
      await apiFetch(path, { method: 'PUT' })
      await mutateCreds()
      mutateZerodhaStatus()
      refreshUser()
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to update status', 'error')
    } finally {
      setBusyId(null)
    }
  }

  async function handleDelete(id: string) {
    setBusyId(id)
    try {
      await apiFetch(`/v1/credentials/${id}`, { method: 'DELETE' })
      setConfirmDelete(null)
      await mutateCreds()
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to delete credential', 'error')
    } finally {
      setBusyId(null)
    }
  }

  async function handleReconnect(id: string) {
    setReconnectingId(id)
    try {
      const { redirect_url } = await apiFetch<{ redirect_url: string }>(
        `/v1/credentials/zerodha/connect?id=${id}`,
      )
      window.location.href = redirect_url
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to initiate Zerodha reconnect', 'error')
      setReconnectingId(null)
    }
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Broker Accounts</h1>
        <div className="flex items-center gap-3">
          {atLimit && (
            <span className="text-sm text-amber-600">
              Plan limit reached ({user.max_broker_creds} max) —{' '}
              <Link href="/billing" className="underline">
                upgrade
              </Link>
            </span>
          )}
          {atLimit ? (
            <Button disabled>
              <Plus className="h-4 w-4" /> Add Broker
            </Button>
          ) : (
            <Button asChild>
              <Link href="/credentials/new">
                <Plus className="h-4 w-4" /> Add Broker
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
            No credentials yet.{' '}
            <Link href="/credentials/new" className="text-primary underline">
              Add your first broker credential
            </Link>{' '}
            to start trading.
          </CardContent>
        </Card>
      ) : (
        <div className="rounded-md border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">
                <th className="px-4 py-3">Broker</th>
                <th className="px-4 py-3">Connection Name</th>
                <th className="px-4 py-3">Execution mode</th>
                <th className="px-4 py-3">Account</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Exchange / Product</th>
                <th className="px-4 py-3">Algo ID</th>
                <th className="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {list.map(cred => {
                const vs = verifyState[cred.id]
                const busy = busyId === cred.id
                const zStatus = zerodhaStatus?.find(z => z.credential_id === cred.id)
                const needsReconnect = (zStatus?.needs_reconnect ?? false) && cred.execution_mode !== 'publisher'
                const isReconnecting = reconnectingId === cred.id
                return (
                  <tr key={cred.id} className="hover:bg-muted/30">
                    <td className="px-4 py-3 font-medium">
                      {BROKER_LABELS[cred.broker_type] || cred.broker_type}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {cred.account_label || <span className="italic">—</span>}
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {cred.execution_mode
                        ? (EXECUTION_MODE_LABELS[cred.execution_mode] ?? cred.execution_mode)
                        : '—'}
                    </td>
                    <td className="px-4 py-3">
                      <Badge
                        variant={cred.account_mode === 'live' ? 'default' : 'secondary'}
                        className="capitalize"
                      >
                        {cred.account_mode}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-col gap-1">
                        <Badge
                          variant={cred.status === 'paused' ? 'outline' : 'secondary'}
                          className={cred.status === 'paused' ? 'text-amber-600 border-amber-400' : 'text-green-700'}
                        >
                          {cred.status === 'active' ? 'Connected' : cred.status === 'paused' ? 'Paused' : (cred.status ?? 'Connected')}
                        </Badge>
                        {cred.admin_disabled && (
                          <span className="text-xs text-amber-600 dark:text-amber-400">
                            Disabled by admin
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                      {cred.exchange && (cred.product || cred.products?.length)
                        ? `${cred.exchange} / ${formatCredentialProducts(cred)}`
                        : '—'}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                      {cred.algo_id || '—'}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-2">
                        {vs && vs !== 'loading' && (
                          <span className="flex items-center gap-1 text-xs">
                            {vs === 'error' ? (
                              <>
                                <XCircle className="h-3.5 w-3.5 text-destructive" />
                                <span className="text-destructive">Failed</span>
                              </>
                            ) : vs.valid ? (
                              <>
                                <CheckCircle className="h-3.5 w-3.5 text-green-600" />
                                <span className="text-green-600">Verified</span>
                              </>
                            ) : (
                              <>
                                <XCircle className="h-3.5 w-3.5 text-destructive" />
                                <span className="text-destructive">Invalid</span>
                              </>
                            )}
                          </span>
                        )}

                        {needsReconnect ? (
                          <Button
                            size="sm"
                            variant={zStatus?.status === 'expired' ? 'destructive' : 'outline'}
                            className={zStatus?.status === 'expires_soon' ? 'border-amber-400 text-amber-700 hover:bg-amber-50' : ''}
                            disabled={isReconnecting}
                            onClick={() => handleReconnect(cred.id)}
                          >
                            {isReconnecting
                              ? <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                              : <RefreshCw className="mr-1 h-3 w-3" />}
                            {isReconnecting ? 'Redirecting…' : 'Reconnect'}
                          </Button>
                        ) : (
                          <Button
                            variant="outline"
                            size="sm"
                            title={vs && vs !== 'loading' ? 'Re-verify credential' : 'Verify credential'}
                            disabled={vs === 'loading'}
                            onClick={() => handleVerify(cred.id)}
                          >
                            {vs === 'loading' ? (
                              <Loader2 className="h-3 w-3 animate-spin" />
                            ) : vs ? (
                              'Re-verify'
                            ) : (
                              'Verify'
                            )}
                          </Button>
                        )}

                        <Button
                          variant="ghost"
                          size="icon"
                          title={
                            cred.status === 'active'
                              ? 'Pause'
                              : cred.admin_disabled
                                ? 'This execution mode is disabled by the ZettaBridge admin — cannot resume'
                                : 'Resume'
                          }
                          disabled={busy || (cred.status === 'paused' && cred.admin_disabled)}
                          onClick={() => handleTogglePause(cred)}
                        >
                          {cred.status === 'active' ? (
                            <Pause className="h-4 w-4" />
                          ) : (
                            <Play className="h-4 w-4" />
                          )}
                        </Button>

                        <Button variant="ghost" size="icon" title="Edit" asChild>
                          <Link href={`/credentials/${cred.id}/edit`}>
                            <Pencil className="h-4 w-4" />
                          </Link>
                        </Button>

                        <Button
                          variant="ghost"
                          size="icon"
                          title="Delete"
                          disabled={busy}
                          className="text-destructive hover:text-destructive"
                          onClick={() =>
                            setConfirmDelete({
                              id: cred.id,
                              label: cred.account_label || cred.broker_type,
                            })
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
        open={!!confirmDelete}
        title="Delete credential?"
        description={`"${confirmDelete?.label}" will be permanently deleted. Webhooks using this credential will fail until updated.`}
        confirmLabel="Delete"
        variant="destructive"
        onConfirm={() => confirmDelete && handleDelete(confirmDelete.id)}
        onCancel={() => setConfirmDelete(null)}
      />
    </div>
  )
}
