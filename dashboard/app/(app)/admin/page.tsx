'use client'

import { useState, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import useSWR from 'swr'
import Link from 'next/link'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PlanBadge } from '@/components/PlanBadge'
import { useToast } from '@/components/ToastProvider'
import { useUser } from '@/lib/user-context'
import { adminListUsers, adminPatchUser, adminDeleteUser, adminSetUserPlan } from '@/lib/api'
import type { AdminUser, Plan } from '@/lib/types'

const PLANS: Plan[] = ['free', 'paper', 'pro', 'pro_plus']

export default function AdminUsersPage() {
  const { user } = useUser()
  const router = useRouter()
  const { toast } = useToast()
  const [page, setPage] = useState(1)
  const [emailFilter, setEmailFilter] = useState('')
  const [planFilter, setPlanFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [debouncedEmail, setDebouncedEmail] = useState('')

  useEffect(() => {
    if (user.role !== 'admin') router.replace('/webhooks')
  }, [user.role, router])

  if (user.role !== 'admin') return null

  const swrKey = `/v1/admin/users?page=${page}&email=${debouncedEmail}&plan=${planFilter}&status=${statusFilter}`
  const { data, mutate, isLoading } = useSWR(
    swrKey,
    () => adminListUsers({ page, limit: 50, email: debouncedEmail || undefined, plan: planFilter || undefined, status: statusFilter || undefined }),
  )

  let emailTimeout: ReturnType<typeof setTimeout>
  function handleEmailInput(val: string) {
    setEmailFilter(val)
    clearTimeout(emailTimeout)
    emailTimeout = setTimeout(() => {
      setDebouncedEmail(val)
      setPage(1)
    }, 400)
  }

  async function handleSetPlan(u: AdminUser, plan: Plan) {
    try {
      await adminSetUserPlan(u.id, { plan })
      mutate()
      toast(`Plan updated to ${plan}`, 'success')
    } catch {
      toast('Failed to update plan', 'error')
    }
  }

  async function handleToggleStatus(u: AdminUser) {
    const newStatus = u.status === 'active' ? 'suspended' : 'active'
    try {
      await adminPatchUser(u.id, { status: newStatus })
      mutate()
      toast(`User ${newStatus}`, 'success')
    } catch {
      toast('Failed to update status', 'error')
    }
  }

  async function handleUnverifyEmail(u: AdminUser) {
    try {
      await adminPatchUser(u.id, { email_verified: false })
      mutate()
      toast('Email verification reset — user must verify again', 'success')
    } catch {
      toast('Failed to reset email verification', 'error')
    }
  }

  async function handleDeleteUser(u: AdminUser) {
    if (!confirm(`Delete ${u.email}? This cannot be undone.`)) return
    try {
      await adminDeleteUser(u.id)
      mutate()
      toast(`User ${u.email} deleted`, 'success')
    } catch {
      toast('Failed to delete user', 'error')
    }
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Admin — Users</h1>
        <Link href="/admin/settings" className="text-sm text-primary hover:underline">
          Platform Settings →
        </Link>
      </div>

      {/* Filters */}
      <div className="mb-4 flex gap-3 flex-wrap">
        <Input
          placeholder="Search by email…"
          value={emailFilter}
          onChange={e => handleEmailInput(e.target.value)}
          className="w-64"
        />
        <select
          value={planFilter}
          onChange={e => { setPlanFilter(e.target.value); setPage(1) }}
          className="h-10 rounded-md border bg-background px-3 text-sm"
        >
          <option value="">All plans</option>
          {PLANS.map(p => <option key={p} value={p}>{p}</option>)}
        </select>
        <select
          value={statusFilter}
          onChange={e => { setStatusFilter(e.target.value); setPage(1) }}
          className="h-10 rounded-md border bg-background px-3 text-sm"
        >
          <option value="">All statuses</option>
          <option value="active">active</option>
          <option value="suspended">suspended</option>
        </select>
      </div>

      <div className="rounded-md border">
        {isLoading ? (
          <div className="px-6 py-10 text-center text-sm text-muted-foreground">Loading…</div>
        ) : !(data?.users ?? []).length ? (
          <div className="px-6 py-10 text-center text-sm text-muted-foreground">No users found.</div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">
                <th className="px-4 py-3">Email</th>
                <th className="px-4 py-3">Plan</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Verified</th>
                <th className="px-4 py-3">Webhooks</th>
                <th className="px-4 py-3">Creds</th>
                <th className="px-4 py-3">Billing</th>
                <th className="px-4 py-3">Set Plan</th>
                <th className="px-4 py-3">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {(data?.users ?? []).map(u => (
                <tr key={u.id} className="hover:bg-muted/30">
                  <td className="px-4 py-3 text-xs font-mono">{u.email}</td>
                  <td className="px-4 py-3">
                    <PlanBadge plan={u.plan} />
                  </td>
                  <td className="px-4 py-3">
                    <Badge variant={u.status === 'active' ? 'default' : 'destructive'}>
                      {u.status}
                    </Badge>
                  </td>
                  <td className="px-4 py-3">
                    {u.email_verified_at ? (
                      <Badge variant="outline" className="text-green-700 border-green-300">verified</Badge>
                    ) : (
                      <Badge variant="outline" className="text-amber-700 border-amber-300">unverified</Badge>
                    )}
                  </td>
                  <td className="px-4 py-3 text-center">{u.webhook_count}</td>
                  <td className="px-4 py-3 text-center">{u.broker_count}</td>
                  <td className="px-4 py-3">
                    <Badge variant="outline">{u.billing_source}</Badge>
                  </td>
                  <td className="px-4 py-3">
                    <select
                      value={u.plan}
                      onChange={e => handleSetPlan(u, e.target.value as Plan)}
                      className="rounded border bg-background px-2 py-1 text-xs"
                    >
                      {PLANS.map(p => <option key={p} value={p}>{p}</option>)}
                    </select>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex gap-1">
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => handleToggleStatus(u)}
                      >
                        {u.status === 'active' ? 'Suspend' : 'Unsuspend'}
                      </Button>
                      {u.email_verified_at && (
                        <Button
                          size="sm"
                          variant="ghost"
                          className="text-amber-700 hover:text-amber-900"
                          title="Reset email verification (for testing)"
                          onClick={() => handleUnverifyEmail(u)}
                        >
                          Unverify
                        </Button>
                      )}
                      {u.role !== 'admin' && u.id !== user.id && (
                        <Button
                          size="sm"
                          variant="ghost"
                          className="text-red-600 hover:text-red-800"
                          onClick={() => handleDeleteUser(u)}
                        >
                          Delete
                        </Button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {data && data.total_pages > 1 && (
        <div className="mt-4 flex items-center justify-end gap-3">
          <Button
            size="sm"
            variant="outline"
            disabled={page <= 1}
            onClick={() => setPage(p => p - 1)}
          >
            ← Prev
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {data.page} of {data.total_pages}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={page >= data.total_pages}
            onClick={() => setPage(p => p + 1)}
          >
            Next →
          </Button>
        </div>
      )}
    </div>
  )
}
