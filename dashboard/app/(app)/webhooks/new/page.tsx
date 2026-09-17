'use client'

import { useSearchParams } from 'next/navigation'
import useSWR from 'swr'
import { Loader2 } from 'lucide-react'
import { WebhookForm } from '@/components/WebhookForm'
import { apiFetch } from '@/lib/api'
import type { Webhook } from '@/lib/types'

export default function NewWebhookPage() {
  const cloneFromId = useSearchParams().get('cloneFrom')

  const { data: cloneFrom, isLoading } = useSWR<Webhook>(
    cloneFromId ? `/v1/webhooks/${cloneFromId}` : null,
    (url: string) => apiFetch<Webhook>(url),
  )

  if (cloneFromId && isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    )
  }

  return (
    <div>
      <h1 className="mb-6 text-2xl font-semibold">
        {cloneFrom ? 'Go Live' : 'Create Webhook'}
      </h1>
      <WebhookForm mode="create" cloneFrom={cloneFrom} />
    </div>
  )
}
