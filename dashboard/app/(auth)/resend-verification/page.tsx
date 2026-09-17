'use client'

import { useState, Suspense } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { MailCheck } from 'lucide-react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { apiFetch, ApiError } from '@/lib/api'

function ResendContent() {
  const searchParams = useSearchParams()
  const [email, setEmail] = useState(searchParams.get('email') ?? '')
  const [loading, setLoading] = useState(false)
  const [sent, setSent] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      await apiFetch('/v1/auth/resend-verification', {
        method: 'POST',
        body: JSON.stringify({ email }),
      })
      setSent(true)
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) {
        setError('Too many attempts. Please wait an hour before trying again.')
      } else if (err instanceof ApiError && err.status >= 500) {
        setError('Failed to send the email. Please try again in a few minutes.')
      } else {
        setSent(true) // non-enumerable: treat unknown errors as success
      }
    } finally {
      setLoading(false)
    }
  }

  if (sent) {
    return (
      <Card className="w-full max-w-sm text-center">
        <CardContent className="pt-6">
          <MailCheck className="mx-auto mb-4 h-12 w-12 text-primary" />
          <h2 className="mb-2 text-xl font-semibold">Email sent</h2>
          <p className="text-sm text-muted-foreground">
            If <strong>{email}</strong> is registered and unverified, we sent a new verification link.
          </p>
        </CardContent>
        <CardFooter className="justify-center text-sm">
          <Link href="/login" className="font-medium text-foreground underline-offset-4 hover:underline">
            Back to sign in
          </Link>
        </CardFooter>
      </Card>
    )
  }

  return (
    <Card className="w-full max-w-sm">
      <CardHeader>
        <CardTitle>Resend verification</CardTitle>
        <CardDescription>
          Enter your email and we&apos;ll send a new verification link.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input
              id="email"
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </div>
          {error && (
            <p className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>
          )}
          <Button type="submit" className="w-full" disabled={loading}>
            {loading ? 'Sending…' : 'Send verification email'}
          </Button>
        </form>
      </CardContent>
      <CardFooter className="justify-center text-sm">
        <Link href="/login" className="text-muted-foreground underline-offset-4 hover:underline">
          Back to sign in
        </Link>
      </CardFooter>
    </Card>
  )
}

export default function ResendVerificationPage() {
  return (
    <Suspense fallback={null}>
      <ResendContent />
    </Suspense>
  )
}
