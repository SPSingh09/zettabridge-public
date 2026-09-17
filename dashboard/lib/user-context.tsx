'use client'

import { createContext, useContext } from 'react'
import type { GetMeResponse } from '@/lib/types'

interface UserContextValue {
  user: GetMeResponse
  refresh: () => void
}

export const UserContext = createContext<UserContextValue | null>(null)

export function useUser(): UserContextValue {
  const ctx = useContext(UserContext)
  if (!ctx) throw new Error('useUser must be used inside AuthGuard')
  return ctx
}
