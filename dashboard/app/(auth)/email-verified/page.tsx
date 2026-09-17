'use client'

import { Suspense } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { CheckCircle, XCircle, AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardFooter } from '@/components/ui/card'

type ErrorVariant = {
  icon: 'amber' | 'red'
  heading: string
  message: string
  primaryAction: 'resend' | 'login'
}

const ERROR_VARIANTS: Record<string, ErrorVariant> = {
  link_superseded: {
    icon: 'amber',
    heading: 'Link no longer valid',
    message: 'A newer verification email has been sent to your inbox. Please use the link from the most recent email.',
    primaryAction: 'login',
  },
  expired_token: {
    icon: 'amber',
    heading: 'Link expired',
    message: 'This verification link has expired (links are valid for 24 hours). Request a new one below.',
    primaryAction: 'resend',
  },
  invalid_token: {
    icon: 'red',
    heading: 'Invalid link',
    message: 'This verification link is not valid. It may have been copied incorrectly.',
    primaryAction: 'resend',
  },
  server_error: {
    icon: 'red',
    heading: 'Something went wrong',
    message: 'We could not process your verification. Please try again.',
    primaryAction: 'resend',
  },
}

function EmailVerifiedContent() {
  const searchParams = useSearchParams()
  const email = searchParams.get('email')
  const error = searchParams.get('error')

  if (!error && email) {
    return (
      <Card className="w-full max-w-sm text-center shadow-sm">
        <CardContent className="pt-8 pb-2">
          <CheckCircle className="mx-auto mb-4 h-14 w-14 text-green-500" />
          <h1 className="mb-2 text-2xl font-semibold">Email verified!</h1>
          <p className="text-sm text-muted-foreground">
            <span className="font-medium text-foreground">{email}</span> has been verified.
            <br />
            You can now sign in to your account.
          </p>
        </CardContent>
        <CardFooter className="justify-center pb-8">
          <Button asChild className="mt-4 w-full">
            <Link href="/login">Go to Login</Link>
          </Button>
        </CardFooter>
      </Card>
    )
  }

  const variant = ERROR_VARIANTS[error ?? ''] ?? {
    icon: 'red' as const,
    heading: 'Verification failed',
    message: 'The verification link is invalid or has expired.',
    primaryAction: 'resend' as const,
  }

  return (
    <Card className="w-full max-w-sm text-center shadow-sm">
      <CardContent className="pt-8 pb-2">
        {variant.icon === 'amber' ? (
          <AlertCircle className="mx-auto mb-4 h-14 w-14 text-amber-500" />
        ) : (
          <XCircle className="mx-auto mb-4 h-14 w-14 text-destructive" />
        )}
        <h1 className="mb-2 text-2xl font-semibold">{variant.heading}</h1>
        <p className="text-sm text-muted-foreground">{variant.message}</p>
      </CardContent>
      <CardFooter className="flex flex-col gap-2 pb-8">
        {variant.primaryAction === 'resend' ? (
          <>
            <Button asChild className="mt-4 w-full">
              <Link href="/resend-verification">Request a new link</Link>
            </Button>
            <Button asChild variant="ghost" className="w-full">
              <Link href="/login">Back to Login</Link>
            </Button>
          </>
        ) : (
          <>
            <Button asChild className="mt-4 w-full">
              <Link href="/login">Go to Login</Link>
            </Button>
            <Button asChild variant="ghost" className="w-full">
              <Link href="/resend-verification">Request a new link anyway</Link>
            </Button>
          </>
        )}
      </CardFooter>
    </Card>
  )
}

export default function EmailVerifiedPage() {
  return (
    <Suspense
      fallback={
        <Card className="w-full max-w-sm text-center">
          <CardContent className="pt-8 pb-8">
            <div className="mx-auto h-14 w-14 rounded-full bg-muted animate-pulse" />
          </CardContent>
        </Card>
      }
    >
      <EmailVerifiedContent />
    </Suspense>
  )
}
