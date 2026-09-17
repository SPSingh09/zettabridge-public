'use client'

import { useState } from 'react'
import useSWR from 'swr'
import Link from 'next/link'
import { Plus, Trash2, Pencil, RotateCcw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import {
  PaperAccountStatusBadge,
  formatMarketRouting,
} from '@/components/PaperAccountStatusBadge'
import { useToast } from '@/components/ToastProvider'
import { apiFetch, listActiveMarketProfiles } from '@/lib/api'
import { formatCurrencyAmount, profileName } from '@/lib/marketDisplay'
import { useUser } from '@/lib/user-context'
import type { PaperAccount } from '@/lib/types'

const PRODUCTS = ['MIS', 'CNC', 'NRML'] as const
// F&O trading isn't supported yet — keep NRML visible but unselectable.
const DISABLED_PRODUCTS: (typeof PRODUCTS)[number][] = ['NRML']

function TableSkeleton() {
  return (
    <div className="rounded-md border divide-y">
      {[1, 2].map(i => (
        <div key={i} className="flex items-center gap-4 px-4 py-3.5">
          <Skeleton className="h-4 w-28" />
          <Skeleton className="h-4 w-40" />
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-5 w-20 rounded-full" />
          <div className="ml-auto flex gap-2">
            <Skeleton className="h-8 w-16 rounded-md" />
          </div>
        </div>
      ))}
    </div>
  )
}

export default function PaperAccountsPage() {
  const { user } = useUser()
  const { toast } = useToast()

  const { data: accounts, mutate: mutateAccounts, isLoading } = useSWR<PaperAccount[]>(
    '/v1/paper-accounts',
    (url: string) => apiFetch<PaperAccount[]>(url),
  )

  const { data: profiles } = useSWR('/v1/market-profiles', listActiveMarketProfiles)

  const list = accounts ?? []
  const atLimit = list.length >= user.max_paper_accounts

  const [confirmDelete, setConfirmDelete] = useState<{ id: string; label: string } | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [togglingId, setTogglingId] = useState<string | null>(null)

  const [confirmReset, setConfirmReset] = useState<PaperAccount | null>(null)
  const [resetBalance, setResetBalance] = useState('')
  const [resetting, setResetting] = useState(false)

  const [editingId, setEditingId] = useState<string | null>(null)
  const [edit, setEdit] = useState({ label: '', default_product: 'MIS', starting_balance: '' })
  const [savingId, setSavingId] = useState<string | null>(null)

  async function handleDelete(id: string) {
    setBusyId(id)
    try {
      await apiFetch(`/v1/paper-accounts/${id}`, { method: 'DELETE' })
      setConfirmDelete(null)
      await mutateAccounts()
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to delete paper account', 'error')
    } finally {
      setBusyId(null)
    }
  }

  async function handleTogglePause(acc: PaperAccount) {
    const action = acc.status === 'active' ? 'pause' : 'resume'
    setTogglingId(acc.id)
    try {
      await apiFetch(`/v1/paper-accounts/${acc.id}/${action}`, { method: 'PUT' })
      await mutateAccounts()
      toast(`"${acc.label || 'Account'}" ${action === 'pause' ? 'deactivated' : 'activated'}`, 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : `Failed to ${action} paper account`, 'error')
    } finally {
      setTogglingId(null)
    }
  }

  function startReset(acc: PaperAccount) {
    setConfirmReset(acc)
    setResetBalance(String(acc.starting_balance))
  }

  async function handleReset() {
    if (!confirmReset) return
    const bal = parseFloat(resetBalance)
    if (!bal || bal <= 0) {
      toast('Starting balance must be greater than zero', 'error')
      return
    }
    setResetting(true)
    try {
      await apiFetch(`/v1/paper-accounts/${confirmReset.id}/reset`, {
        method: 'PUT',
        body: JSON.stringify({ starting_balance: bal }),
      })
      setConfirmReset(null)
      await mutateAccounts()
      toast(`"${confirmReset.label || 'Account'}" reset`, 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to reset paper account', 'error')
    } finally {
      setResetting(false)
    }
  }

  function startEdit(acc: PaperAccount) {
    setEditingId(acc.id)
    setEdit({
      label: acc.label,
      default_product: acc.default_product,
      starting_balance: String(acc.starting_balance),
    })
  }

  async function handleSaveEdit(acc: PaperAccount) {
    const balanceEditable = acc.cash_balance === acc.starting_balance
    const body: Record<string, unknown> = {
      label: edit.label.trim(),
      default_product: edit.default_product,
    }
    if (balanceEditable) {
      const bal = parseFloat(edit.starting_balance)
      if (!bal || bal <= 0) {
        toast('Starting balance must be greater than zero', 'error')
        return
      }
      body.starting_balance = bal
    }
    setSavingId(acc.id)
    try {
      await apiFetch(`/v1/paper-accounts/${acc.id}`, { method: 'PATCH', body: JSON.stringify(body) })
      setEditingId(null)
      await mutateAccounts()
      toast(`"${edit.label || acc.label}" updated`, 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to update paper account', 'error')
    } finally {
      setSavingId(null)
    }
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between gap-4">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-2xl font-semibold">Paper Accounts</h1>
          <Badge
            variant="outline"
            className="border-blue-200 bg-blue-50 font-normal text-blue-800 dark:border-blue-800 dark:bg-blue-950/40 dark:text-blue-300"
          >
            Simulated Execution Only
          </Badge>
        </div>
        <div className="flex shrink-0 items-center gap-3">
          {atLimit && (
            <span className="text-sm text-amber-600">
              Plan limit reached ({user.max_paper_accounts} max)
            </span>
          )}
          {atLimit ? (
            <Button disabled>
              <Plus className="h-4 w-4" /> Create paper account
            </Button>
          ) : (
            <Button asChild>
              <Link href="/paper-accounts/new">
                <Plus className="h-4 w-4" /> Create paper account
              </Link>
            </Button>
          )}
        </div>
      </div>

      <p className="mb-6 text-sm text-muted-foreground">
        Paper Trading runs fully inside ZettaBridge for testing and strategy validation. It does
        not connect to real brokers and place live orders.
      </p>

      {isLoading ? (
        <TableSkeleton />
      ) : list.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="py-16 text-center">
            <p className="text-sm text-muted-foreground">No paper accounts yet.</p>
            <p className="mt-1 text-sm text-muted-foreground">
              Create a paper account to test webhook strategies safely before connecting a live
              broker.
            </p>
            {!atLimit && (
              <Button asChild className="mt-4">
                <Link href="/paper-accounts/new">
                  <Plus className="h-4 w-4" /> Create paper account
                </Link>
              </Button>
            )}
          </CardContent>
        </Card>
      ) : (
        <div className="rounded-md border overflow-x-auto">
          <table className="w-full min-w-[880px] text-sm">
            <thead>
              <tr className="border-b bg-muted/50 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">
                <th className="px-4 py-3">Account name</th>
                <th className="px-4 py-3">Market / Exchange / Product</th>
                <th className="px-4 py-3">Starting balance</th>
                <th className="px-4 py-3">Cash balance</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {list.map(acc => {
                const busy = busyId === acc.id
                const isEditing = editingId === acc.id
                const balanceEditable = acc.cash_balance === acc.starting_balance
                const marketLabel = profileName(profiles, acc.market_profile)
                const routing = formatMarketRouting(marketLabel, acc.exchange, acc.default_product)

                return (
                  <tr key={acc.id} className="hover:bg-muted/30">
                    <td className="px-4 py-3 font-medium">
                      {isEditing ? (
                        <Input
                          className="w-40"
                          value={edit.label}
                          onChange={e => setEdit(v => ({ ...v, label: e.target.value }))}
                          placeholder="Account name"
                        />
                      ) : (
                        <Link href={`/paper-accounts/${acc.id}`} className="text-primary hover:underline">
                          {acc.label || `${marketLabel} Paper Account`}
                        </Link>
                      )}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {isEditing ? (
                        <Select
                          className="w-24"
                          value={edit.default_product}
                          onChange={e => setEdit(v => ({ ...v, default_product: e.target.value }))}
                        >
                          {PRODUCTS.map(p => (
                            <option key={p} value={p} disabled={DISABLED_PRODUCTS.includes(p)}>
                              {p}
                            </option>
                          ))}
                        </Select>
                      ) : (
                        routing
                      )}
                    </td>
                    <td className="px-4 py-3 tabular-nums">
                      {isEditing && balanceEditable ? (
                        <Input
                          type="number"
                          step="0.01"
                          className="w-28"
                          value={edit.starting_balance}
                          onChange={e => setEdit(v => ({ ...v, starting_balance: e.target.value }))}
                        />
                      ) : (
                        formatCurrencyAmount(acc.starting_balance, acc.base_currency)
                      )}
                    </td>
                    <td className="px-4 py-3 tabular-nums">
                      {formatCurrencyAmount(acc.cash_balance, acc.base_currency)}
                    </td>
                    <td className="px-4 py-3">
                      <PaperAccountStatusBadge status={acc.status} />
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-2">
                        {isEditing ? (
                          <>
                            <Button size="sm" onClick={() => handleSaveEdit(acc)} disabled={savingId === acc.id}>
                              {savingId === acc.id ? 'Saving…' : 'Save'}
                            </Button>
                            <Button size="sm" variant="outline" onClick={() => setEditingId(null)}>
                              Cancel
                            </Button>
                          </>
                        ) : (
                          <>
                            <Button asChild size="sm" variant="default">
                              <Link href={`/paper-accounts/${acc.id}`}>Open account</Link>
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              disabled={togglingId === acc.id || acc.status === 'closed'}
                              onClick={() => handleTogglePause(acc)}
                            >
                              {acc.status === 'active' ? 'Deactivate' : 'Activate'}
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8"
                              title="Edit account name"
                              onClick={() => startEdit(acc)}
                            >
                              <Pencil className="h-4 w-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8"
                              title="Reset balance"
                              onClick={() => startReset(acc)}
                            >
                              <RotateCcw className="h-4 w-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-destructive hover:text-destructive"
                              title="Delete account"
                              disabled={busy}
                              onClick={() =>
                                setConfirmDelete({ id: acc.id, label: acc.label || 'this account' })
                              }
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </>
                        )}
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
        title="Delete paper account?"
        description={`"${confirmDelete?.label}" and all of its simulated orders/positions/trades will be permanently deleted.`}
        confirmLabel="Delete"
        variant="destructive"
        onConfirm={() => confirmDelete && handleDelete(confirmDelete.id)}
        onCancel={() => setConfirmDelete(null)}
      />

      <ConfirmDialog
        open={!!confirmReset}
        title="Reset paper account?"
        description={`All of "${confirmReset?.label || 'this account'}"'s positions, orders, trades, and equity history will be permanently deleted, and cash balance will be restored to the amount below.`}
        confirmLabel={resetting ? 'Resetting…' : 'Reset balance'}
        variant="destructive"
        onConfirm={handleReset}
        onCancel={() => setConfirmReset(null)}
      >
        <label className="mb-1 block text-sm font-medium">
          New starting balance ({confirmReset?.base_currency})
        </label>
        <Input
          type="number"
          step="0.01"
          value={resetBalance}
          onChange={e => setResetBalance(e.target.value)}
        />
      </ConfirmDialog>
    </div>
  )
}
