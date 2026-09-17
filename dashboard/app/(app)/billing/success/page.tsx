'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { CheckCircle, Loader2 } from 'lucide-react'
import { Card, CardContent, CardFooter } from '@/components/ui/card'
import { PlanBadge } from '@/components/PlanBadge'
import { apiFetch } from '@/lib/api'
import type { GetMeResponse } from '@/lib/types'

const MAX_POLLS = 10
const POLL_INTERVAL_MS = 2000

export default function BillingSuccessPage() {
  const [plan, setPlan] = useState<GetMeResponse['plan'] | null>(null)
  const [polling, setPolling] = useState(true)

  useEffect(() => {
    let attempts = 0
    const id = setInterval(async () => {
      attempts++
      try {
        const me = await apiFetch<GetMeResponse>('/v1/me')
        if (me.plan !== 'free' || attempts >= MAX_POLLS) {
          setPlan(me.plan)
          setPolling(false)
          clearInterval(id)
        }
      } catch {
        setPolling(false)
        clearInterval(id)
      }
    }, POLL_INTERVAL_MS)
    return () => clearInterval(id)
  }, [])

  return (
    <Card className="mx-auto mt-16 w-full max-w-sm text-center">
      <CardContent className="pt-6">
        {polling ? (
          <>
            <Loader2 className="mx-auto mb-4 h-12 w-12 animate-spin text-primary" />
            <h2 className="mb-2 text-xl font-semibold">Processing payment…</h2>
            <p className="text-sm text-muted-foreground">Confirming your subscription.</p>
          </>
        ) : (
          <>
            <CheckCircle className="mx-auto mb-4 h-12 w-12 text-green-500" />
            <h2 className="mb-2 text-xl font-semibold">Payment successful!</h2>
            {plan && (
              <div className="flex justify-center gap-2">
                <span className="text-sm text-muted-foreground">Your plan:</span>
                <PlanBadge plan={plan} />
              </div>
            )}
          </>
        )}
      </CardContent>
      {!polling && (
        <CardFooter className="justify-center">
          <Link href="/webhooks" className="font-medium text-foreground underline-offset-4 hover:underline">
            Go to dashboard →
          </Link>
        </CardFooter>
      )}
    </Card>
  )
}
