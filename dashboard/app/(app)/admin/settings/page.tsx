'use client'

import { useState, useEffect } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import useSWR from 'swr'
import Link from 'next/link'
import { ArrowLeft, CircleHelp } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useToast } from '@/components/ToastProvider'
import { useUser } from '@/lib/user-context'
import { formatExpiresIn } from '@/lib/formatExpiresIn'
import {
  adminGetMarketDataProvider,
  adminSetMarketDataProvider,
  adminGetMarketDataInterval,
  adminSetMarketDataInterval,
  adminGetFyersStatus,
  adminFyersConnect,
  adminSetFyersPin,
  adminGetMarketHoursEnforcement,
  adminSetMarketHoursEnforcement,
  adminGetZerodhaOAuthEnabled,
  adminSetZerodhaOAuthEnabled,
  adminGetZerodhaPublisherEnabled,
  adminSetZerodhaPublisherEnabled,
} from '@/lib/api'

const PROVIDER_OPTIONS: Record<
  string,
  { title: string; description: string }
> = {
  signal: {
    title: 'Signal Feed',
    description: 'Prices update only when webhook signals are received.',
  },
  fyers: {
    title: 'FYERS Live Data',
    description:
      'Continuously updates unrealized P&L using live FYERS market data.',
  },
}

function ConnectionStatus({
  connected,
  tokenExpired,
  loading,
}: {
  connected?: boolean
  tokenExpired?: boolean
  loading: boolean
}) {
  if (loading) {
    return <span className="text-sm text-muted-foreground">Loading…</span>
  }
  if (connected && !tokenExpired) {
    return (
      <span className="inline-flex items-center gap-2 text-sm font-medium text-emerald-700 dark:text-emerald-400">
        <span className="h-2 w-2 rounded-full bg-emerald-500" aria-hidden />
        Healthy
      </span>
    )
  }
  if (tokenExpired) {
    return (
      <span className="inline-flex items-center gap-2 text-sm font-medium text-amber-700 dark:text-amber-400">
        <span className="h-2 w-2 rounded-full bg-amber-500" aria-hidden />
        Reconnect required
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-2 text-sm text-muted-foreground">
      <span className="h-2 w-2 rounded-full bg-muted-foreground/40" aria-hidden />
      Not connected
    </span>
  )
}

export default function AdminSettingsPage() {
  const { user } = useUser()
  const router = useRouter()
  const searchParams = useSearchParams()
  const { toast } = useToast()
  const [saving, setSaving] = useState(false)
  const [selected, setSelected] = useState<string>('')
  const [connecting, setConnecting] = useState(false)
  const [pin, setPin] = useState('')
  const [savingPin, setSavingPin] = useState(false)
  const [intervalInput, setIntervalInput] = useState('')
  const [savingInterval, setSavingInterval] = useState(false)
  const [savingHours, setSavingHours] = useState(false)
  const [savingZerodhaOAuth, setSavingZerodhaOAuth] = useState(false)
  const [savingZerodhaPublisher, setSavingZerodhaPublisher] = useState(false)

  useEffect(() => {
    if (user.role !== 'admin') router.replace('/webhooks')
  }, [user.role, router])

  const { data, mutate, isLoading } = useSWR(
    '/v1/admin/settings/market-data-provider',
    adminGetMarketDataProvider,
  )
  const { data: fyersStatus, mutate: mutateFyers, isLoading: fyersLoading } = useSWR(
    '/v1/admin/fyers/status',
    adminGetFyersStatus,
  )
  const { data: hoursSetting, mutate: mutateHours, isLoading: hoursLoading } = useSWR(
    '/v1/admin/settings/market-hours-enforcement',
    adminGetMarketHoursEnforcement,
  )
  const { data: intervalSetting, mutate: mutateInterval, isLoading: intervalLoading } = useSWR(
    '/v1/admin/settings/market-data-interval',
    adminGetMarketDataInterval,
  )
  const { data: zerodhaOAuthSetting, mutate: mutateZerodhaOAuth, isLoading: zerodhaOAuthLoading } = useSWR(
    '/v1/admin/settings/zerodha-oauth-enabled',
    adminGetZerodhaOAuthEnabled,
  )
  const { data: zerodhaPublisherSetting, mutate: mutateZerodhaPublisher, isLoading: zerodhaPublisherLoading } = useSWR(
    '/v1/admin/settings/zerodha-publisher-enabled',
    adminGetZerodhaPublisherEnabled,
  )

  useEffect(() => {
    if (data) setSelected(data.provider)
  }, [data])

  useEffect(() => {
    if (intervalSetting) setIntervalInput(String(intervalSetting.interval_seconds))
  }, [intervalSetting])

  useEffect(() => {
    const fyersParam = searchParams.get('fyers')
    if (fyersParam === 'connected') {
      toast('FYERS account connected', 'success')
      mutateFyers()
      router.replace('/admin/settings')
    } else if (fyersParam === 'error') {
      toast(
        `Connection failed: ${searchParams.get('reason') ?? 'please try again'}`,
        'error',
      )
      router.replace('/admin/settings')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchParams])

  if (user.role !== 'admin') return null

  async function handleSaveProvider() {
    if (!selected) return
    setSaving(true)
    try {
      await adminSetMarketDataProvider(selected)
      mutate()
      toast('Market data provider updated', 'success')
    } catch {
      toast('Failed to update market data provider', 'error')
    } finally {
      setSaving(false)
    }
  }

  async function handleConnectFyers() {
    setConnecting(true)
    try {
      const { redirect_url } = await adminFyersConnect()
      window.location.href = redirect_url
    } catch {
      toast('Failed to start account connection', 'error')
      setConnecting(false)
    }
  }

  async function handleToggleMarketHours() {
    if (!hoursSetting) return
    setSavingHours(true)
    try {
      await adminSetMarketHoursEnforcement(!hoursSetting.enabled)
      mutateHours()
      toast(
        `Market hours enforcement ${!hoursSetting.enabled ? 'enabled' : 'disabled'}`,
        'success',
      )
    } catch {
      toast('Failed to update market hours enforcement', 'error')
    } finally {
      setSavingHours(false)
    }
  }

  async function handleToggleZerodhaOAuth() {
    if (!zerodhaOAuthSetting) return
    setSavingZerodhaOAuth(true)
    try {
      await adminSetZerodhaOAuthEnabled(!zerodhaOAuthSetting.enabled)
      mutateZerodhaOAuth()
      toast(
        `Zerodha OAuth ${!zerodhaOAuthSetting.enabled ? 'enabled' : 'disabled'}`,
        'success',
      )
    } catch {
      toast('Failed to update Zerodha OAuth setting', 'error')
    } finally {
      setSavingZerodhaOAuth(false)
    }
  }

  async function handleToggleZerodhaPublisher() {
    if (!zerodhaPublisherSetting) return
    setSavingZerodhaPublisher(true)
    try {
      await adminSetZerodhaPublisherEnabled(!zerodhaPublisherSetting.enabled)
      mutateZerodhaPublisher()
      toast(
        `Zerodha Kite Publisher ${!zerodhaPublisherSetting.enabled ? 'enabled' : 'disabled'}`,
        'success',
      )
    } catch {
      toast('Failed to update Zerodha Publisher setting', 'error')
    } finally {
      setSavingZerodhaPublisher(false)
    }
  }

  async function handleSaveInterval() {
    if (!intervalSetting) return
    const seconds = parseInt(intervalInput, 10)
    if (!Number.isFinite(seconds)) return
    setSavingInterval(true)
    try {
      await adminSetMarketDataInterval(seconds)
      mutateInterval()
      toast('Refresh interval updated', 'success')
    } catch {
      toast('Failed to update refresh interval', 'error')
    } finally {
      setSavingInterval(false)
    }
  }

  async function handleSavePin() {
    if (!pin.trim()) return
    setSavingPin(true)
    try {
      await adminSetFyersPin(pin.trim())
      setPin('')
      mutateFyers()
      toast('Trading PIN updated', 'success')
    } catch {
      toast('Failed to update trading PIN', 'error')
    } finally {
      setSavingPin(false)
    }
  }

  const accessExpiresIn = formatExpiresIn(fyersStatus?.access_token_expires_at)

  return (
    <div className="max-w-2xl space-y-6">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Link href="/admin" className="text-muted-foreground hover:text-foreground">
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1 className="text-2xl font-semibold">Platform Settings</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Configure paper trading and market data for this deployment.
            </p>
          </div>
        </div>
        <Link href="/admin/instruments" className="text-sm text-primary hover:underline shrink-0">
          Markets &amp; Instruments →
        </Link>
      </div>

      {/* ── Paper Trading ─────────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Paper Trading</CardTitle>
          <CardDescription>Rules that apply to simulated order execution.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <div className="mb-2 flex flex-wrap items-center gap-2">
              <span className="text-sm font-medium">Market hours enforcement</span>
              {!hoursLoading && (
                <Badge variant={hoursSetting?.enabled ? 'default' : 'secondary'}>
                  {hoursSetting?.enabled ? 'Enabled' : 'Disabled'}
                </Badge>
              )}
              <button
                type="button"
                className="text-muted-foreground hover:text-foreground"
                title="Market hours are configured per Market Profile."
                aria-label="Market hours are configured per Market Profile."
              >
                <CircleHelp className="h-4 w-4" />
              </button>
            </div>
            <p className="text-sm text-muted-foreground">
              Reject Paper Trading orders outside configured market hours.
            </p>
          </div>
        </CardContent>
        <CardFooter>
          {hoursLoading ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : (
            <Button
              onClick={handleToggleMarketHours}
              disabled={savingHours}
              variant={hoursSetting?.enabled ? 'outline' : 'default'}
            >
              {savingHours
                ? 'Saving…'
                : hoursSetting?.enabled
                  ? 'Disable enforcement'
                  : 'Enable enforcement'}
            </Button>
          )}
        </CardFooter>
      </Card>

      {/* ── Zerodha Integration ──────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Zerodha Integration</CardTitle>
          <CardDescription>
            Enable or disable Zerodha&apos;s two live execution paths independently.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b pb-6">
            <div>
              <div className="mb-2 flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium">Zerodha OAuth</span>
                {!zerodhaOAuthLoading && (
                  <Badge variant={zerodhaOAuthSetting?.enabled ? 'default' : 'secondary'}>
                    {zerodhaOAuthSetting?.enabled ? 'Enabled' : 'Disabled'}
                  </Badge>
                )}
              </div>
              <p className="text-sm text-muted-foreground">
                Kite Connect login, new/existing credential order execution, and cancellation.
              </p>
            </div>
            {zerodhaOAuthLoading ? (
              <p className="text-sm text-muted-foreground">Loading…</p>
            ) : (
              <Button
                onClick={handleToggleZerodhaOAuth}
                disabled={savingZerodhaOAuth}
                variant={zerodhaOAuthSetting?.enabled ? 'outline' : 'default'}
              >
                {savingZerodhaOAuth
                  ? 'Saving…'
                  : zerodhaOAuthSetting?.enabled
                    ? 'Disable OAuth'
                    : 'Enable OAuth'}
              </Button>
            )}
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <div className="mb-2 flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium">Zerodha Kite Publisher</span>
                {!zerodhaPublisherLoading && (
                  <Badge variant={zerodhaPublisherSetting?.enabled ? 'default' : 'secondary'}>
                    {zerodhaPublisherSetting?.enabled ? 'Enabled' : 'Disabled'}
                  </Badge>
                )}
              </div>
              <p className="text-sm text-muted-foreground">
                New Publisher-mode credentials and new basket order placement.
              </p>
            </div>
            {zerodhaPublisherLoading ? (
              <p className="text-sm text-muted-foreground">Loading…</p>
            ) : (
              <Button
                onClick={handleToggleZerodhaPublisher}
                disabled={savingZerodhaPublisher}
                variant={zerodhaPublisherSetting?.enabled ? 'outline' : 'default'}
              >
                {savingZerodhaPublisher
                  ? 'Saving…'
                  : zerodhaPublisherSetting?.enabled
                    ? 'Disable Publisher'
                    : 'Enable Publisher'}
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

      {/* ── Market Data ───────────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Market Data</CardTitle>
          <CardDescription>
            Determines how paper positions receive live price updates. Market data is
            separate from live order execution — it never places trades.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div>
            <p className="mb-3 text-sm font-medium">Provider</p>
            {isLoading ? (
              <p className="text-sm text-muted-foreground">Loading…</p>
            ) : (
              <div className="space-y-2">
                {(data?.available ?? []).map(p => {
                  const option = PROVIDER_OPTIONS[p]
                  return (
                    <label
                      key={p}
                      className="flex cursor-pointer items-start gap-3 rounded-md border p-3 text-sm has-[:checked]:border-primary has-[:checked]:bg-primary/5"
                    >
                      <input
                        type="radio"
                        name="provider"
                        value={p}
                        checked={selected === p}
                        onChange={() => setSelected(p)}
                        className="mt-1"
                      />
                      <span>
                        <span className="font-medium">{option?.title ?? p}</span>
                        <br />
                        <span className="text-muted-foreground">
                          {option?.description ?? ''}
                        </span>
                      </span>
                    </label>
                  )
                })}
              </div>
            )}

            {selected === 'fyers' && !data?.fyers_configured && (
              <Alert variant="warning" className="mt-3">
                <AlertDescription>
                  Connect FYERS below to enable live price updates. Until then, prices
                  update only when webhook signals include a price.
                </AlertDescription>
              </Alert>
            )}

            <div className="mt-3">
              <Button
                onClick={handleSaveProvider}
                disabled={saving || selected === data?.provider || isLoading}
              >
                {saving ? 'Saving…' : 'Save provider'}
              </Button>
            </div>
          </div>

          <div className="border-t pt-6">
            <p className="mb-1 text-sm font-medium">Refresh interval</p>
            <p className="mb-3 text-sm text-muted-foreground">
              Lower intervals provide more responsive unrealized P&amp;L updates but
              increase market data usage.
            </p>
            {intervalLoading ? (
              <p className="text-sm text-muted-foreground">Loading…</p>
            ) : (
              <div className="flex flex-wrap items-end gap-3">
                <div>
                  <div className="flex items-center gap-2">
                    <Input
                      type="number"
                      min={intervalSetting?.min_seconds}
                      max={intervalSetting?.max_seconds}
                      value={intervalInput}
                      onChange={e => setIntervalInput(e.target.value)}
                      className="max-w-[100px]"
                      aria-label="Refresh interval in seconds"
                    />
                    <span className="text-sm text-muted-foreground">seconds</span>
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Allowed: {intervalSetting?.min_seconds}–{intervalSetting?.max_seconds}{' '}
                    seconds
                  </p>
                </div>
                <Button
                  onClick={handleSaveInterval}
                  disabled={
                    savingInterval ||
                    !intervalInput ||
                    Number(intervalInput) === intervalSetting?.interval_seconds
                  }
                >
                  {savingInterval ? 'Saving…' : 'Save interval'}
                </Button>
              </div>
            )}
          </div>

          <div className="border-t pt-6">
            <p className="mb-1 text-sm font-medium">Connections</p>
            <p className="mb-4 text-sm text-muted-foreground">
              Live market data providers linked to this deployment.
            </p>

            <div className="rounded-md border p-4 space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="font-medium">FYERS</span>
                <ConnectionStatus
                  connected={fyersStatus?.connected}
                  tokenExpired={fyersStatus?.token_expired}
                  loading={fyersLoading}
                />
              </div>

              {fyersLoading ? (
                <p className="text-sm text-muted-foreground">Loading…</p>
              ) : !fyersStatus?.app_configured ? (
                <Alert variant="warning">
                  <AlertDescription>
                    FYERS is not configured for this deployment. Set the required
                    environment variables before connecting an account.
                  </AlertDescription>
                </Alert>
              ) : (
                <>
                  {accessExpiresIn && (
                    <div className="text-sm">
                      <span className="text-muted-foreground">Access token · </span>
                      <span className="font-medium">
                        {fyersStatus?.token_expired ? 'Expired' : `Expires in ${accessExpiresIn}`}
                      </span>
                    </div>
                  )}

                  {fyersStatus?.token_expired && (
                    <Alert variant="warning">
                      <AlertDescription>
                        Your FYERS access token has expired. Reconnect to restore live
                        price updates.
                      </AlertDescription>
                    </Alert>
                  )}

                  <Button onClick={handleConnectFyers} disabled={connecting}>
                    {connecting
                      ? 'Redirecting…'
                      : fyersStatus?.connected || fyersStatus?.token_expired
                        ? 'Reconnect account'
                        : 'Connect account'}
                  </Button>
                </>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* ── Credentials ───────────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Credentials</CardTitle>
          <CardDescription>
            Secrets used to maintain market data connections.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {fyersLoading ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : !fyersStatus?.app_configured ? (
            <p className="text-sm text-muted-foreground">
              Available after FYERS is configured on this deployment.
            </p>
          ) : (
            <div className="space-y-3">
              <div>
                <p className="text-sm font-medium">Trading PIN</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Stored securely. Used only to maintain your FYERS connection when
                  supported. If automatic renewal fails, you&apos;ll be notified to
                  reconnect.
                </p>
                {fyersStatus?.pin_set && (
                  <p className="mt-2 font-mono text-sm tracking-widest text-muted-foreground">
                    ••••
                  </p>
                )}
              </div>
              <div className="flex flex-wrap gap-2">
                <Input
                  type="password"
                  placeholder={fyersStatus?.pin_set ? 'New PIN' : '4-digit PIN'}
                  value={pin}
                  onChange={e => setPin(e.target.value)}
                  className="max-w-[160px]"
                  autoComplete="off"
                />
                <Button
                  variant="outline"
                  onClick={handleSavePin}
                  disabled={savingPin || !pin.trim()}
                >
                  {savingPin ? 'Saving…' : fyersStatus?.pin_set ? 'Update PIN' : 'Save PIN'}
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
