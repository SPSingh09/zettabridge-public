'use client'

import { useEffect, useState } from 'react'
import { useParams } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import { apiFetch } from '@/lib/api'
import { CredentialForm } from '@/components/CredentialForm'
import type { BrokerCredential } from '@/lib/types'

export default function EditCredentialPage() {
  const params = useParams()
  const id = params.id as string
  const [cred, setCred] = useState<BrokerCredential | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    apiFetch<BrokerCredential[]>('/v1/credentials')
      .then(cs => {
        const c = cs.find(x => x.id === id)
        if (!c) setError('Credential not found')
        else setCred(c)
      })
      .catch(() => setError('Failed to load credentials'))
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
      <h1 className="mb-6 text-2xl font-semibold">Edit Broker</h1>
      {cred && <CredentialForm mode="edit" credential={cred} />}
    </div>
  )
}
