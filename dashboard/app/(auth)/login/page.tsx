'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { apiFetch, ApiError } from '@/lib/api'
import { setToken } from '@/lib/auth'
import type { LoginResponse } from '@/lib/types'

export default function LoginPage() {
  const router = useRouter()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [unverified, setUnverified] = useState(false)
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setUnverified(false)
    setLoading(true)
    try {
      const res = await apiFetch<LoginResponse>('/v1/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email, password }),
      })
      setToken(res.token)
      router.push('/webhooks')
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 403 && err.message.toLowerCase().includes('not verified')) {
          setUnverified(true)
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

  return (
    <Card className="w-full max-w-sm">
      <CardHeader>
        <CardTitle>Sign in</CardTitle>
        <CardDescription>Enter your email and password to continue</CardDescription>
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
              autoComplete="current-password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>

          {error && (
            <p className="text-sm text-destructive">{error}</p>
          )}

          {unverified && (
            <p className="text-sm text-amber-700">
              Email not verified.{' '}
              <Link
                href={`/resend-verification?email=${encodeURIComponent(email)}`}
                className="font-medium underline underline-offset-4"
              >
                Resend verification email →
              </Link>
            </p>
          )}

          <Button type="submit" className="w-full" disabled={loading}>
            {loading ? 'Signing in…' : 'Sign in'}
          </Button>
        </form>
      </CardContent>
      <CardFooter className="justify-center text-sm text-muted-foreground">
        Don&apos;t have an account?{' '}
        <Link href="/register" className="ml-1 font-medium text-foreground underline-offset-4 hover:underline">
          Register
        </Link>
      </CardFooter>
    </Card>
  )
}
