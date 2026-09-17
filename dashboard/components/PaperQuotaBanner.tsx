'use client'

import Link from 'next/link'
import { AlertTriangle } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useUser } from '@/lib/user-context'

export function PaperQuotaBanner() {
  const { user } = useUser()
  if (!user.paper_trade_quota_exceeded) return null

  const limit = user.max_paper_trades_per_month ?? 10

  return (
    <Alert variant="warning" className="rounded-none border-x-0 border-t-0">
      <AlertTriangle className="h-4 w-4" />
      <AlertDescription>
        You&apos;ve used all <strong>{limit}</strong> paper trades included on your Free plan this
        month ({user.paper_trades_used_this_month}/{limit}).{' '}
        <Link href="/billing" className="underline font-medium">
          Upgrade your plan
        </Link>{' '}
        for unlimited paper trading, or wait until next month when your quota resets.
      </AlertDescription>
    </Alert>
  )
}
