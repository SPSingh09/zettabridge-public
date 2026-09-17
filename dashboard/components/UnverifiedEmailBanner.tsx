'use client'

import Link from 'next/link'
import { MailWarning } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useUser } from '@/lib/user-context'

export function UnverifiedEmailBanner() {
  const { user } = useUser()
  if (user.email_verified) return null

  return (
    <Alert variant="warning" className="rounded-none border-x-0 border-t-0">
      <MailWarning className="h-4 w-4" />
      <AlertDescription>
        Please verify your email address to unlock all features.{' '}
        <Link
          href={`/resend-verification?email=${encodeURIComponent(user.email)}`}
          className="font-medium underline underline-offset-4"
        >
          Resend verification email
        </Link>
      </AlertDescription>
    </Alert>
  )
}
