'use client'

import { clearToken, getToken } from '@/lib/auth'

// In production (ECS/ALB) NEXT_PUBLIC_API_URL is baked in at build time → direct browser-to-API calls.
// In local dev (empty) relative URLs hit the Next.js rewrite which proxies to the local Go server.
const API_URL = process.env.NEXT_PUBLIC_API_URL || ''

// Public ingest base URL — used to build the TradingView webhook URL shown to users.
// In staging/prod this is the API domain (api.staging.zettabridge.net).
// In local dev it falls back to the current window origin (Next.js rewrites /v1/* to Go).
export const INGEST_BASE_URL =
  typeof window !== 'undefined'
    ? (process.env.NEXT_PUBLIC_API_URL || window.location.origin)
    : (process.env.NEXT_PUBLIC_API_URL || '')

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    public readonly error_code?: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export async function apiFetch<T>(
  path: string,
  opts: RequestInit = {},
): Promise<T> {
  const token = getToken()
  const headers = new Headers(opts.headers)
  headers.set('Content-Type', 'application/json')
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(`${API_URL}${path}`, { ...opts, headers })

  if (res.status === 204) return undefined as T

  if (res.status === 401) {
    // Auth endpoints (login, register, etc.) return 401 for wrong credentials —
    // don't redirect, let the caller handle it.
    const isAuthEndpoint = path.startsWith('/v1/auth/')
    if (!isAuthEndpoint) {
      clearToken()
      if (typeof window !== 'undefined') {
        window.location.href = '/login'
      }
    }
    const body401 = await res.json().catch(() => null)
    const msg = body401?.error ?? 'Session expired. Please log in again.'
    throw new ApiError(401, msg)
  }

  const body = await res.json().catch(() => null)

  if (!res.ok) {
    const msg = body?.error ?? `Request failed (${res.status})`
    throw new ApiError(res.status, msg, body?.error_code)
  }

  // Unwrap {data: T} envelope; fall through for bare responses
  return (body?.data ?? body) as T
}

// downloadCsv fetches a CSV export endpoint (which returns a raw file body,
// not the {data: T} JSON envelope apiFetch expects) and triggers a browser
// download. A plain <a href> can't carry the Bearer token, so this fetches
// with the auth header and saves the response as a Blob instead.
export async function downloadCsv(path: string, filename: string): Promise<void> {
  const token = getToken()
  const headers = new Headers()
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(`${API_URL}${path}`, { headers })
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    throw new ApiError(res.status, body?.error ?? `Export failed (${res.status})`, body?.error_code)
  }

  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

// ── P&L ───────────────────────────────────────────────────────────────────────

import type {
  PnLSummary,
  AdminUserList, AdminUser, MarketDataProviderSetting, FyersStatus,
  MarketProfile, Instrument, MarketDataIntervalSetting, SymbolRequest,
} from '@/lib/types'

export const getWebhookPnL = (webhookId: string) =>
  apiFetch<PnLSummary>(`/v1/webhooks/${webhookId}/pnl`)

// ── Telegram settings ─────────────────────────────────────────────────────────

import type { TelegramSettings } from '@/lib/types'

export const getTelegramSettings = () =>
  apiFetch<TelegramSettings>('/v1/settings/telegram')

export const updateTelegramSettings = (chatId: string) =>
  apiFetch<TelegramSettings>('/v1/settings/telegram', {
    method: 'PATCH',
    body: JSON.stringify({ chat_id: chatId }),
  })

// ── Admin ─────────────────────────────────────────────────────────────────────

export const adminListUsers = (params: { page?: number; limit?: number; plan?: string; status?: string; email?: string } = {}) => {
  const q = new URLSearchParams()
  if (params.page) q.set('page', String(params.page))
  if (params.limit) q.set('limit', String(params.limit))
  if (params.plan) q.set('plan', params.plan)
  if (params.status) q.set('status', params.status)
  if (params.email) q.set('email', params.email)
  return apiFetch<AdminUserList>(`/v1/admin/users?${q}`)
}

export const adminPatchUser = (userId: string, body: { status?: string; email_verified?: boolean }) =>
  apiFetch<AdminUser>(`/v1/admin/users/${userId}`, { method: 'PATCH', body: JSON.stringify(body) })

export const adminDeleteUser = (userId: string) =>
  apiFetch<void>(`/v1/admin/users/${userId}`, { method: 'DELETE' })

export const adminSetUserPlan = (userId: string, body: { plan: string }) =>
  apiFetch<AdminUser>(`/v1/admin/users/${userId}/plan`, { method: 'PUT', body: JSON.stringify(body) })

export const adminGetMarketDataProvider = () =>
  apiFetch<MarketDataProviderSetting>('/v1/admin/settings/market-data-provider')

export const adminSetMarketDataProvider = (provider: string) =>
  apiFetch<MarketDataProviderSetting>('/v1/admin/settings/market-data-provider', {
    method: 'PUT',
    body: JSON.stringify({ provider }),
  })

export const adminGetMarketDataInterval = () =>
  apiFetch<MarketDataIntervalSetting>('/v1/admin/settings/market-data-interval')

export const adminSetMarketDataInterval = (intervalSeconds: number) =>
  apiFetch<MarketDataIntervalSetting>('/v1/admin/settings/market-data-interval', {
    method: 'PUT',
    body: JSON.stringify({ interval_seconds: intervalSeconds }),
  })

export const adminGetFyersStatus = () =>
  apiFetch<FyersStatus>('/v1/admin/fyers/status')

export const adminFyersConnect = () =>
  apiFetch<{ redirect_url: string }>('/v1/admin/fyers/connect')

export const adminSetFyersPin = (pin: string) =>
  apiFetch<{ pin_set: boolean }>('/v1/admin/fyers/pin', { method: 'PUT', body: JSON.stringify({ pin }) })

// ── Market profiles / instruments (Phase 7) ───────────────────────────────────

export const listActiveMarketProfiles = () =>
  apiFetch<MarketProfile[]>('/v1/market-profiles')

export const adminListMarketProfiles = () =>
  apiFetch<MarketProfile[]>('/v1/admin/market-profiles')

// Market profiles are pre-configured (seeded via migration, not created by
// admins) — only base_currency, timezone, and active are editable; name and
// code are the profile's stable identity and aren't accepted by the API.
export const adminUpdateMarketProfile = (code: string, body: Partial<{
  base_currency: string
  timezone: string
  active: boolean
  slippage_bps: number
  fee_bps: number
}>) => apiFetch<MarketProfile>(`/v1/admin/market-profiles/${encodeURIComponent(code)}`, { method: 'PATCH', body: JSON.stringify(body) })

export const adminListInstruments = (marketProfileCode?: string) => {
  const q = marketProfileCode ? `?market_profile=${encodeURIComponent(marketProfileCode)}` : ''
  return apiFetch<Instrument[]>(`/v1/admin/instruments${q}`)
}

export const adminCreateInstrument = (body: {
  market_profile_code: string
  exchange: string
  symbol: string
  name: string
  instrument_type?: string
  tick_size?: number
  lot_size?: number
  active?: boolean
}) => apiFetch<Instrument>('/v1/admin/instruments', { method: 'POST', body: JSON.stringify(body) })

export const adminUpdateInstrument = (id: string, body: Partial<{
  name: string
  instrument_type: string
  tick_size: number
  lot_size: number
  active: boolean
}>) => apiFetch<Instrument>(`/v1/admin/instruments/${id}`, { method: 'PATCH', body: JSON.stringify(body) })

export const adminDeleteInstrument = (id: string) =>
  apiFetch<void>(`/v1/admin/instruments/${id}`, { method: 'DELETE' })

export const adminGetMarketHoursEnforcement = () =>
  apiFetch<{ enabled: boolean }>('/v1/admin/settings/market-hours-enforcement')

export const adminSetMarketHoursEnforcement = (enabled: boolean) =>
  apiFetch<{ enabled: boolean }>('/v1/admin/settings/market-hours-enforcement', { method: 'PUT', body: JSON.stringify({ enabled }) })

export const adminGetZerodhaOAuthEnabled = () =>
  apiFetch<{ enabled: boolean }>('/v1/admin/settings/zerodha-oauth-enabled')

export const adminSetZerodhaOAuthEnabled = (enabled: boolean) =>
  apiFetch<{ enabled: boolean }>('/v1/admin/settings/zerodha-oauth-enabled', { method: 'PUT', body: JSON.stringify({ enabled }) })

export const adminGetZerodhaPublisherEnabled = () =>
  apiFetch<{ enabled: boolean }>('/v1/admin/settings/zerodha-publisher-enabled')

export const adminSetZerodhaPublisherEnabled = (enabled: boolean) =>
  apiFetch<{ enabled: boolean }>('/v1/admin/settings/zerodha-publisher-enabled', { method: 'PUT', body: JSON.stringify({ enabled }) })

// ── Symbol requests ────────────────────────────────────────────────────────

export const createSymbolRequest = (body: {
  market_profile_code: string
  exchange?: string
  symbol: string
  reason: string
}) => apiFetch<SymbolRequest>('/v1/symbol-requests', { method: 'POST', body: JSON.stringify(body) })

export const listMySymbolRequests = () =>
  apiFetch<SymbolRequest[]>('/v1/symbol-requests')

export const adminListSymbolRequests = (status?: string) => {
  const q = status ? `?status=${encodeURIComponent(status)}` : ''
  return apiFetch<SymbolRequest[]>(`/v1/admin/symbol-requests${q}`)
}

export const adminAcceptSymbolRequest = (id: string) =>
  apiFetch<SymbolRequest>(`/v1/admin/symbol-requests/${id}/accept`, { method: 'POST' })

export const adminRejectSymbolRequest = (id: string, adminNote?: string) =>
  apiFetch<SymbolRequest>(`/v1/admin/symbol-requests/${id}/reject`, { method: 'POST', body: JSON.stringify({ admin_note: adminNote ?? '' }) })
