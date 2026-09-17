'use client'

import { useEffect, useState, Suspense } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { CheckCircle, XCircle, Loader2 } from 'lucide-react'
import { Card, CardContent, CardFooter } from '@/components/ui/card'
import { apiFetch, ApiError } from '@/lib/api'
import type { VerifyEmailResponse } from '@/lib/types'

function VerifyEmailContent() {
  const searchParams = useSearchParams()
  const token = searchParams.get('token')

  const [status, setStatus] = useState<'loading' | 'success' | 'error'>('loading')
  const [email, setEmail] = useState<string | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  useEffect(() => {
    if (!token) {
      setErrorMsg('No verification token found in the URL.')
      setStatus('error')
      return
    }
    apiFetch<VerifyEmailResponse>(`/v1/auth/verify-email?token=${encodeURIComponent(token)}`)
      .then((res) => {
        setEmail(res.email)
        setStatus('success')
      })
      .catch((err) => {
        setErrorMsg(err instanceof ApiError ? err.message : 'Verification failed.')
        setStatus('error')
      })
  }, [token])

  return (
    <Card className="w-full max-w-sm text-center">
      <CardContent className="pt-6">
        {status === 'loading' && (
          <>
            <Loader2 className="mx-auto mb-4 h-12 w-12 animate-spin text-muted-foreground" />
            <p className="text-sm text-muted-foreground">Verifying your email…</p>
          </>
        )}
        {status === 'success' && (
          <>
            <CheckCircle className="mx-auto mb-4 h-12 w-12 text-green-500" />
            <h2 className="mb-2 text-xl font-semibold">Email verified!</h2>
            <p className="text-sm text-muted-foreground">
              {email} is now verified. You can sign in.
            </p>
          </>
        )}
        {status === 'error' && (
          <>
            <XCircle className="mx-auto mb-4 h-12 w-12 text-destructive" />
            <h2 className="mb-2 text-xl font-semibold">Verification failed</h2>
            <p className="text-sm text-muted-foreground">{errorMsg}</p>
          </>
        )}
      </CardContent>
      <CardFooter className="justify-center text-sm">
        {status === 'success' ? (
          <Link href="/login" className="font-medium text-foreground underline-offset-4 hover:underline">
            Sign in →
          </Link>
        ) : status === 'error' ? (
          <Link
            href="/resend-verification"
            className="text-muted-foreground underline-offset-4 hover:underline"
          >
            Request a new verification link
          </Link>
        ) : null}
      </CardFooter>
    </Card>
  )
}

export default function VerifyEmailPage() {
  return (
    <Suspense
      fallback={
        <Card className="w-full max-w-sm text-center">
          <CardContent className="pt-6">
            <Loader2 className="mx-auto h-12 w-12 animate-spin text-muted-foreground" />
          </CardContent>
        </Card>
      }
    >
      <VerifyEmailContent />
    </Suspense>
  )
}
