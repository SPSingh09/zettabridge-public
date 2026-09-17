import type { Trade } from '@/lib/types'

export function isPublisherHandoffExpired(trade: Trade): boolean {
  if (trade.error_code === 'publisher_handoff_expired') return true
  if (trade.status !== 'pending_confirmation') return false
  if (!trade.publisher_order_expires_at) return false
  return new Date(trade.publisher_order_expires_at).getTime() <= Date.now()
}

export function publisherHandoffExpiryLabel(expiresAt: string | undefined): string | null {
  if (!expiresAt) return null
  const diff = new Date(expiresAt).getTime() - Date.now()
  if (diff <= 0) return 'Expired'
  const mins = Math.floor(diff / 60000)
  const secs = Math.floor((diff % 60000) / 1000)
  if (mins >= 60) return `${Math.floor(mins / 60)}h ${mins % 60}m`
  if (mins > 0) return `${mins}m`
  return `${secs}s`
}
