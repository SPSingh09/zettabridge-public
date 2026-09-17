'use client'

import { useEffect, useState, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import { apiFetch, ApiError } from '@/lib/api'
import { UserContext } from '@/lib/user-context'
import { NavBar } from '@/components/NavBar'
import { UnverifiedEmailBanner } from '@/components/UnverifiedEmailBanner'
import { ZerodhaReconnectBanner } from '@/components/ZerodhaReconnectBanner'
import { DowngradePausedBanner } from '@/components/DowngradePausedBanner'
import { PaperQuotaBanner } from '@/components/PaperQuotaBanner'
import { PendingSymbolRequestsBanner } from '@/components/PendingSymbolRequestsBanner'
import type { GetMeResponse } from '@/lib/types'

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const [user, setUser] = useState<GetMeResponse | null>(null)

  const fetchMe = useCallback(async () => {
    try {
      const me = await apiFetch<GetMeResponse>('/v1/me')
      setUser(me)
    } catch {
      router.push('/login')
    }
  }, [router])

  useEffect(() => {
    fetchMe()
  }, [fetchMe])

  if (!user) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <UserContext.Provider value={{ user, refresh: fetchMe }}>
      <div className="flex min-h-screen flex-col">
        <NavBar />
        <UnverifiedEmailBanner />
        <ZerodhaReconnectBanner />
        <DowngradePausedBanner />
        <PaperQuotaBanner />
        <PendingSymbolRequestsBanner />
        <main className="flex-1 container py-6">{children}</main>
      </div>
    </UserContext.Provider>
  )
}
