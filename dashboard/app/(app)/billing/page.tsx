'use client'

import { useState } from 'react'
import useSWR from 'swr'
import { CheckCircle, ExternalLink, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { PlanBadge } from '@/components/PlanBadge'
import { ApiError, apiFetch } from '@/lib/api'
import { useUser } from '@/lib/user-context'
import type { BillingPlanEntry, BillingPlansResponse } from '@/lib/types'

export default function BillingPage() {
  const { user } = useUser()
  const [loadingKey, setLoadingKey] = useState<string | null>(null)
  const [loadingPortal, setLoadingPortal] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const { data, isLoading } = useSWR<BillingPlansResponse>(
    '/v1/billing/plans',
    (url: string) => apiFetch<BillingPlansResponse>(url),
  )

  async function handleCheckout(plan: BillingPlanEntry) {
    const key = plan.plan
    setLoadingKey(key)
    setError(null)
    try {
      const res = await apiFetch<{ url: string }>('/v1/billing/checkout', {
        method: 'POST',
        body: JSON.stringify({ plan: plan.plan }),
      })
      window.location.href = res.url
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to start checkout')
    } finally {
      setLoadingKey(null)
    }
  }

  async function handlePortal() {
    setLoadingPortal(true)
    setError(null)
    try {
      const res = await apiFetch<{ url: string }>('/v1/billing/portal', {
        method: 'POST',
      })
      window.location.href = res.url
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to open portal')
    } finally {
      setLoadingPortal(false)
    }
  }

  const billingDisabled = data && !data.billing_enabled
  const hasPaidSub = !!user.stripe_subscription_id

  return (
    <div className="max-w-5xl">
      <div className="mb-6 flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-semibold">Billing</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Manage your plan, subscription, and usage limits.
          </p>
        </div>
        {hasPaidSub && (
          <Button variant="outline" disabled={loadingPortal} onClick={handlePortal}>
            {loadingPortal ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <>
                <ExternalLink className="mr-2 h-4 w-4" />
                Manage subscription
              </>
            )}
          </Button>
        )}
      </div>

      {error && (
        <div className="mb-4 rounded-md bg-destructive/10 px-4 py-2 text-sm text-destructive">
          {error}
        </div>
      )}

      <div className="mb-8 rounded-lg border bg-muted/30 px-6 py-5">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-medium">Current plan</span>
          <PlanBadge plan={user.plan} />
          {user.billing_source === 'admin' && (
            <Badge variant="outline" className="text-xs">
              Admin override
            </Badge>
          )}
        </div>
        <div className="mt-3 flex flex-wrap gap-6 text-sm text-muted-foreground">
          <span>
            <span className="font-medium text-foreground">{user.max_paper_webhooks}</span> paper
            webhooks
          </span>
          {user.live_trading_allowed && (
            <span>
              <span className="font-medium text-foreground">{user.max_live_webhooks}</span> live
              webhooks
            </span>
          )}
          <span>
            <span className="font-medium text-foreground">{user.max_paper_accounts}</span> paper
            accounts
          </span>
          {user.live_trading_allowed && (
            <span>
              <span className="font-medium text-foreground">{user.max_broker_creds}</span> live
              broker connections
            </span>
          )}
          {user.max_paper_trades_per_month != null && (
            <span>
              <span className="font-medium text-foreground">
                {user.paper_trades_used_this_month}/{user.max_paper_trades_per_month}
              </span>{' '}
              paper trades this month
            </span>
          )}
        </div>
      </div>

      {billingDisabled ? (
        <div className="rounded-md border border-dashed px-6 py-10 text-center space-y-3">
          <p className="text-sm text-muted-foreground">
            During closed beta, billing and plan changes are handled manually by ZettaBridge
            support. Contact support to change your plan, request higher limits, or discuss
            production access.
          </p>
          <a
            href="mailto:support@zettabridge.net"
            className="inline-flex items-center gap-1.5 rounded-md border px-4 py-2 text-sm font-medium hover:bg-muted transition-colors"
          >
            Contact Support
          </a>
        </div>
      ) : isLoading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading plans…
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          {(data?.plans ?? []).map(plan => {
            const key = plan.plan
            const isCurrent = plan.plan === user.plan
            const busy = loadingKey === key
            const rank = planRank(plan.plan)
            const currentRank = planRank(user.plan)
            const canUpgrade = rank > currentRank

            return (
              <Card key={key} className={isCurrent ? 'border-primary ring-1 ring-primary' : ''}>
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between">
                    <CardTitle className="text-base">{plan.name}</CardTitle>
                    {isCurrent && (
                      <Badge variant="default" className="text-xs">
                        Current
                      </Badge>
                    )}
                  </div>
                  {plan.price_label ? (
                    <CardDescription className="text-xl font-semibold text-foreground">
                      {plan.price_label}
                    </CardDescription>
                  ) : (
                    <CardDescription className="text-sm">Contact support for pricing</CardDescription>
                  )}
                </CardHeader>
                <CardContent className="space-y-1.5 text-sm">
                  <Feature label={`${plan.max_paper_accounts} paper accounts`} />
                  <Feature label={`${plan.max_paper_webhooks} paper webhooks`} />
                  {plan.max_paper_trades_per_month != null ? (
                    <Feature label={`${plan.max_paper_trades_per_month} paper trades / month`} />
                  ) : (
                    <Feature label="Unlimited paper trades / month" />
                  )}
                  {plan.live_trading && (
                    <>
                      <Feature label={`${plan.max_live_webhooks} live webhooks`} />
                      <Feature label={`${plan.max_brokers} live broker connections`} />
                      <Feature label="Live trading" />
                    </>
                  )}
                  {plan.multi_product_credentials && (
                    <Feature label="Multi-product broker accounts (MIS, CNC, NRML)" />
                  )}
                  {plan.audit_logs && <Feature label="Trade history & audit logs" />}
                </CardContent>
                <CardFooter>
                  {isCurrent ? (
                    <Button variant="outline" className="w-full" disabled>
                      Current plan
                    </Button>
                  ) : plan.plan === 'free' || !canUpgrade ? (
                    <Button variant="outline" className="w-full" disabled>
                      Contact support to change plan
                    </Button>
                  ) : (
                    <Button
                      className="w-full"
                      disabled={!!loadingKey}
                      onClick={() => handleCheckout(plan)}
                    >
                      {busy ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        `Upgrade to ${plan.name}`
                      )}
                    </Button>
                  )}
                </CardFooter>
              </Card>
            )
          })}
        </div>
      )}
    </div>
  )
}

function planRank(p: BillingPlanEntry['plan']): number {
  switch (p) {
    case 'free':
      return 0
    case 'paper':
      return 1
    case 'pro':
      return 2
    case 'pro_plus':
      return 3
    default:
      return 0
  }
}

function Feature({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2">
      <CheckCircle className="h-3.5 w-3.5 flex-shrink-0 text-primary" />
      <span className="text-muted-foreground">{label}</span>
    </div>
  )
}
