'use client'

import { useEffect, useState } from 'react'
import { useParams } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import { apiFetch } from '@/lib/api'
import { WebhookForm } from '@/components/WebhookForm'
import type { Webhook } from '@/lib/types'

export default function EditWebhookPage() {
  const params = useParams()
  const id = params.id as string
  const [webhook, setWebhook] = useState<Webhook | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    apiFetch<Webhook[]>('/v1/webhooks')
      .then(whs => {
        const wh = whs.find(w => w.id === id)
        if (!wh) setError('Webhook not found')
        else setWebhook(wh)
      })
      .catch(() => setError('Failed to load webhooks'))
      .finally(() => setLoading(false))
  }, [id])

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-md bg-destructive/10 px-4 py-2 text-sm text-destructive">{error}</div>
    )
  }

  return (
    <div>
      <h1 className="mb-6 text-2xl font-semibold">Edit Webhook</h1>
      {webhook && <WebhookForm mode="edit" webhook={webhook} />}
    </div>
  )
}
