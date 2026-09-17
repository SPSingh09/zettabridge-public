'use client'

import Link from 'next/link'
import { AlertTriangle } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useUser } from '@/lib/user-context'

export function DowngradePausedBanner() {
  const { user } = useUser()
  if (!user.has_auto_paused_items) return null

  const whPaper = user.max_paper_webhooks
  const whLive = user.max_live_webhooks
  const cr = user.max_broker_creds

  return (
    <Alert variant="warning" className="rounded-none border-x-0 border-t-0">
      <AlertTriangle className="h-4 w-4" />
      <AlertDescription>
        Your plan changed to <strong className="capitalize">{user.plan.replace('_', ' ')}</strong>.
        Some webhooks and credentials were paused to match your new limits (
        {whPaper} paper webhook{whPaper !== 1 ? 's' : ''}
        {user.live_trading_allowed ? `, ${whLive} live webhook${whLive !== 1 ? 's' : ''}` : ''},
        {cr} live broker account{cr !== 1 ? 's' : ''}). Go to{' '}
        <Link href="/webhooks" className="underline font-medium">
          Webhooks
        </Link>{' '}
        and{' '}
        <Link href="/credentials" className="underline font-medium">
          Broker Accounts
        </Link>{' '}
        to choose which ones to keep active. This notice disappears once you&apos;ve made your
        selections.
      </AlertDescription>
    </Alert>
  )
}
