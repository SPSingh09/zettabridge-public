'use client'

import { useState, Suspense } from 'react'
import { useSearchParams } from 'next/navigation'
import Link from 'next/link'
import { MailCheck } from 'lucide-react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { apiFetch, ApiError } from '@/lib/api'
import type { RegisterResponse } from '@/lib/types'

function RegisterForm() {
  const searchParams = useSearchParams()
  const initialInvite = searchParams.get('invite') ?? ''

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [inviteCode, setInviteCode] = useState(initialInvite)
  const [showInviteField, setShowInviteField] = useState(!!initialInvite)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [done, setDone] = useState(false)
  const [verificationRequired, setVerificationRequired] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      const body: Record<string, string> = { email, password }
      if (inviteCode.trim()) body.invite_code = inviteCode.trim()

      const res = await apiFetch<RegisterResponse>('/v1/auth/register', {
        method: 'POST',
        body: JSON.stringify(body),
      })
      setVerificationRequired(res.email_verification_required ?? false)
      setDone(true)
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.error_code === 'registration_closed') {
          setShowInviteField(true)
          setError('This beta is invite-only. Please use the invite link you received.')
        } else if (err.error_code === 'invalid_invite') {
          setShowInviteField(true)
          setError('Invite code is invalid or has already been used.')
        } else if (err.error_code === 'invite_email_mismatch') {
          setError('This invite is for a different email address.')
        } else {
          setError(err.message)
        }
      } else {
        setError('Something went wrong. Please try again.')
      }
    } finally {
      setLoading(false)
    }
  }

  if (done) {
    return (
      <Card className="w-full max-w-sm text-center">
        <CardContent className="pt-6">
          <MailCheck className="mx-auto mb-4 h-12 w-12 text-primary" />
          <h2 className="mb-2 text-xl font-semibold">Account created!</h2>
          {verificationRequired ? (
            <p className="text-sm text-muted-foreground">
              We sent a verification link to <strong>{email}</strong>. Click it to activate your account.
            </p>
          ) : (
            <p className="text-sm text-muted-foreground">
              Your account is ready.{' '}
              <Link href="/login" className="font-medium text-foreground underline-offset-4 hover:underline">
                Sign in →
              </Link>
            </p>
          )}
        </CardContent>
        {verificationRequired && (
          <CardFooter className="justify-center text-sm">
            <Link
              href={`/resend-verification?email=${encodeURIComponent(email)}`}
              className="text-muted-foreground underline-offset-4 hover:underline"
            >
              Didn&apos;t receive it? Resend
            </Link>
          </CardFooter>
        )}
      </Card>
    )
  }

  return (
    <Card className="w-full max-w-sm">
      <CardHeader>
        <CardTitle>Create account</CardTitle>
        <CardDescription>Get started with ZettaBridge</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input
              id="email"
              type="email"
              autoComplete="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="password">Password</Label>
            <Input
              id="password"
              type="password"
              autoComplete="new-password"
              required
              minLength={8}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">Minimum 8 characters</p>
          </div>

          {showInviteField && (
            <div className="space-y-2">
              <Label htmlFor="invite_code">Invite code</Label>
              <Input
                id="invite_code"
                type="text"
                autoComplete="off"
                value={inviteCode}
                onChange={(e) => setInviteCode(e.target.value)}
                placeholder="inv_..."
              />
            </div>
          )}

          {error && <p className="text-sm text-destructive">{error}</p>}

          <Button type="submit" className="w-full" disabled={loading}>
            {loading ? 'Creating account…' : 'Create account'}
          </Button>

          <p className="text-xs text-muted-foreground text-center leading-relaxed">
            By creating an account you agree to our{' '}
            <a href="https://zettabridge.net/privacy" target="_blank" rel="noopener noreferrer" className="underline underline-offset-2 hover:text-foreground">
              Privacy Policy
            </a>{' '}
            and{' '}
            <a href="https://zettabridge.net/terms" target="_blank" rel="noopener noreferrer" className="underline underline-offset-2 hover:text-foreground">
              Terms of Service
            </a>
            .
          </p>
        </form>
      </CardContent>
      <CardFooter className="justify-center text-sm text-muted-foreground">
        Already have an account?{' '}
        <Link href="/login" className="ml-1 font-medium text-foreground underline-offset-4 hover:underline">
          Sign in
        </Link>
      </CardFooter>
    </Card>
  )
}

export default function RegisterPage() {
  return (
    <Suspense>
      <RegisterForm />
    </Suspense>
  )
}
