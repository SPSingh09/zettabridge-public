import type { SymbolRequestStatus } from '@/lib/types'

export type RequestType = 'instrument' | 'feature' | 'broker' | 'other'

export const REQUEST_TYPE_LABELS: Record<RequestType, string> = {
  instrument: 'Instrument',
  feature: 'Feature',
  broker: 'Broker',
  other: 'Other',
}

export function requestDisplayId(id: string): string {
  const compact = id.replace(/-/g, '').toUpperCase()
  return `#${compact.slice(-6)}`
}

export function requestTitle(exchange: string, symbol: string): string {
  const sym = symbol.trim().toUpperCase()
  const ex = exchange.trim().toUpperCase()
  if (ex && sym) return `${ex}:${sym}`
  return sym
}

export function statusLabel(status: SymbolRequestStatus): string {
  switch (status) {
    case 'pending':
      return 'Pending'
    case 'accepted':
      return 'Approved'
    case 'rejected':
      return 'Rejected'
    case 'resolved':
      return 'Closed'
    default:
      return status
  }
}

export function formatRelativeDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const now = new Date()
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const startOfDate = new Date(d.getFullYear(), d.getMonth(), d.getDate())
  const diffDays = Math.round((startOfToday.getTime() - startOfDate.getTime()) / 86400000)
  if (diffDays === 0) return 'Today'
  if (diffDays === 1) return 'Yesterday'
  if (diffDays > 1 && diffDays < 7) return `${diffDays} days ago`
  return d.toLocaleDateString()
}
