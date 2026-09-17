'use client'

import { useEffect, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import { Users, Loader2, XCircle } from 'lucide-react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { apiFetch, ApiError } from '@/lib/api'
import { setToken } from '@/lib/auth'
import type { InvitePreviewResponse, AcceptInviteResponse } from '@/lib/types'

export default function InvitePage() {
  const params = useParams()
  const router = useRouter()
  const token = params.token as string

  const [preview, setPreview] = useState<InvitePreviewResponse | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [password, setPassword] = useState('')
  const [acceptError, setAcceptError] = useState<string | null>(null)
  const [accepting, setAccepting] = useState(false)

  useEffect(() => {
    apiFetch<InvitePreviewResponse>(`/v1/invites/${encodeURIComponent(token)}`)
      .then(setPreview)
      .catch((err) => {
        setLoadError(err instanceof ApiError ? err.message : 'Invite not found or expired.')
      })
  }, [token])

  async function handleAccept(e: React.FormEvent) {
    e.preventDefault()
    setAcceptError(null)
    setAccepting(true)
    try {
      const res = await apiFetch<AcceptInviteResponse>('/v1/invites/accept', {
        method: 'POST',
        body: JSON.stringify({ token, password }),
      })
      setToken(res.token)
      router.push('/webhooks')
    } catch (err) {
      setAcceptError(err instanceof ApiError ? err.message : 'Failed to accept invite.')
      setAccepting(false)
    }
  }

  if (loadError) {
    return (
      <div className="flex min-h-screen items-center justify-center px-4">
        <Card className="w-full max-w-sm text-center">
          <CardContent className="pt-6">
            <XCircle className="mx-auto mb-4 h-12 w-12 text-destructive" />
            <h2 className="mb-2 text-xl font-semibold">Invite unavailable</h2>
            <p className="text-sm text-muted-foreground">{loadError}</p>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (!preview) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <div className="flex items-center gap-2">
            <Users className="h-5 w-5 text-primary" />
            <CardTitle>Organisation invite</CardTitle>
          </div>
          <CardDescription>
            You&apos;ve been invited to join <strong>{preview.org_name}</strong> as{' '}
            <strong>{preview.role}</strong>.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="mb-4 rounded-md border bg-muted/40 p-3 text-sm space-y-1">
            <p>
              <span className="text-muted-foreground">Email: </span>
              <span className="font-medium">{preview.email}</span>
            </p>
            <p>
              <span className="text-muted-foreground">Expires: </span>
              <span>{new Date(preview.expires_at).toLocaleDateString()}</span>
            </p>
          </div>
          <form onSubmit={handleAccept} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="password">Set a password</Label>
              <Input
                id="password"
                type="password"
                autoComplete="new-password"
                required
                minLength={8}
                placeholder="Minimum 8 characters"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            {acceptError && <p className="text-sm text-destructive">{acceptError}</p>}
            <Button type="submit" className="w-full" disabled={accepting}>
              {accepting ? 'Joining…' : 'Accept & join organisation'}
            </Button>
          </form>
        </CardContent>
        <CardFooter className="justify-center text-xs text-muted-foreground">
          Already have an account?{' '}
          <a href="/login" className="ml-1 underline underline-offset-4">
            Sign in instead
          </a>
        </CardFooter>
      </Card>
    </div>
  )
}
