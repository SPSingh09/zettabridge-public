'use client'

import { useState, useEffect, useRef } from 'react'
import { useRouter } from 'next/navigation'
import { CheckCircle, Copy, ExternalLink, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { apiFetch } from '@/lib/api'
import { useUser } from '@/lib/user-context'
import {
  INDIAN_PRODUCT_OPTIONS,
  buildCredentialProductPayload,
  parseCredentialProducts,
  type IndianProduct,
} from '@/lib/credentialProducts'
import type { BrokerCredential, BrokerType, AccountMode } from '@/lib/types'

const BROKER_OPTIONS: { value: BrokerType; label: string }[] = [
  { value: 'mt5_cloud', label: 'MetaTrader 5 (MetaApi Cloud)' },
  { value: 'zerodha', label: 'Zerodha Kite' },
  { value: 'angel', label: 'Angel One' },
  { value: 'dhan', label: 'Dhan' },
]

// Shown in create-mode dropdown only; other brokers remain in BROKER_OPTIONS for edit/display.
const BROKER_OPTIONS_VISIBLE_ON_CREATE: BrokerType[] = ['zerodha']

const FORMAT_HINTS: Record<BrokerType, string> = {
  mt5_cloud: 'auth_token:account_id',
  zerodha: 'api_key:api_secret',
  angel: 'api_key:client_code:jwt',
  dhan: 'client_id:access_token',
}

const INDIAN_BROKERS: BrokerType[] = ['zerodha', 'angel', 'dhan']

interface FormValues {
  broker_type: BrokerType | ''
  account_label: string
  account_mode: AccountMode
  execution_mode: 'user_api_oauth' | 'publisher'
  raw_creds: string
  exchange: string
  products: IndianProduct[]
  algo_id: string
  market_protection: string
}

function toForm(cred: BrokerCredential): FormValues {
  return {
    broker_type: cred.broker_type,
    account_label: cred.account_label ?? '',
    account_mode: 'live',
    execution_mode: (cred.execution_mode as 'user_api_oauth' | 'publisher') ?? 'user_api_oauth',
    raw_creds: '',
    exchange: cred.exchange || 'NSE',
    products: parseCredentialProducts(cred),
    algo_id: cred.algo_id ?? '',
    market_protection: cred.market_protection != null ? String(cred.market_protection) : '2',
  }
}

const defaultForm: FormValues = {
  broker_type: '',
  account_label: '',
  account_mode: 'live',
  execution_mode: 'publisher',
  raw_creds: '',
  exchange: 'NSE',
  products: ['MIS'],
  algo_id: '',
  market_protection: '2',
}

interface Props {
  mode: 'create' | 'edit'
  credential?: BrokerCredential
}

export function CredentialForm({ mode, credential }: Props) {
  const router = useRouter()
  const { user } = useUser()
  const canLive = user.live_trading_allowed
  const canMultiProduct = user.plan === 'pro_plus'
  const [form, setForm] = useState<FormValues>(credential ? toForm(credential) : defaultForm)
  const [error, setError] = useState<string | null>(null)
  const [connectError, setConnectError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [connecting, setConnecting] = useState(false)
  const [callbackURL, setCallbackURL] = useState<string | null>(null)
  const [publisherCallbackURL, setPublisherCallbackURL] = useState<string | null>(null)
  const [oauthEnabled, setOauthEnabled] = useState(true)
  const [publisherEnabled, setPublisherEnabled] = useState(true)
  const [copied, setCopied] = useState(false)
  const [copiedPublisher, setCopiedPublisher] = useState(false)
  const callbackFetched = useRef(false)

  function toggleProduct(product: IndianProduct) {
    setForm(prev => {
      const selected = prev.products.includes(product)
        ? prev.products.filter(p => p !== product)
        : [...prev.products, product]
      return {
        ...prev,
        products: selected.length > 0 ? selected : prev.products,
      }
    })
  }

  function setSingleProduct(product: IndianProduct) {
    setForm(prev => ({ ...prev, products: [product] }))
  }

  function indianProductFields() {
    return buildCredentialProductPayload(form.products, canMultiProduct)
  }

  function set<K extends keyof FormValues>(key: K, val: FormValues[K]) {
    setForm(prev => ({ ...prev, [key]: val }))
  }

  const isIndian =
    form.broker_type !== '' && INDIAN_BROKERS.includes(form.broker_type as BrokerType)
  const isZerodha = form.broker_type === 'zerodha'
  const isFreeNoLiveZerodha = isZerodha && user.plan === 'free'
  const isZerodhaNoOAuth = isFreeNoLiveZerodha
  const isPublisherMode = isZerodha && form.execution_mode === 'publisher'

  // Fetch Kite Connect callback URLs once when zerodha live mode is shown.
  useEffect(() => {
    if (!isZerodha || isZerodhaNoOAuth || callbackFetched.current) return
    callbackFetched.current = true
    apiFetch<{
      callback_url: string
      publisher_callback_url: string
      oauth_enabled: boolean
      publisher_enabled: boolean
    }>('/v1/credentials/zerodha/connect-info')
      .then(data => {
        setCallbackURL(data.callback_url)
        setPublisherCallbackURL(data.publisher_callback_url)
        setOauthEnabled(data.oauth_enabled)
        setPublisherEnabled(data.publisher_enabled)
      })
      .catch(() => {})
  }, [isZerodha, isZerodhaNoOAuth])

  async function handleZerodhaConnect(credId: string) {
    setError(null)
    setConnecting(true)
    try {
      const { redirect_url } = await apiFetch<{ redirect_url: string }>(
        `/v1/credentials/zerodha/connect?id=${credId}`,
      )
      window.location.href = redirect_url
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to initiate Zerodha OAuth')
      setConnecting(false)
    }
  }

  // "Save & Connect" for live paid Zerodha create: create credential then start OAuth.
  async function handleSaveAndConnect() {
    if (!form.raw_creds.trim()) {
      setError('API Key and Secret are required (api_key:api_secret)')
      return
    }
    setError(null)
    setSubmitting(true)
    const payload: Record<string, unknown> = {
      broker_type: form.broker_type,
      account_label: form.account_label,
      account_mode: form.account_mode,
      execution_mode: 'user_api_oauth',
      raw_creds: form.raw_creds.trim(),
      exchange: form.exchange,
      ...indianProductFields(),
      market_protection: parseFloat(form.market_protection) || 0,
    }
    if (form.algo_id.trim()) payload.algo_id = form.algo_id.trim()
    try {
      const cred = await apiFetch<BrokerCredential>('/v1/credentials', {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      await handleZerodhaConnect(cred.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create credential')
      setSubmitting(false)
    }
  }

  async function handleCopyCallback() {
    if (!callbackURL) return
    try {
      await navigator.clipboard.writeText(callbackURL)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // ignore
    }
  }

  async function handleCopyPublisherCallback() {
    if (!publisherCallbackURL) return
    try {
      await navigator.clipboard.writeText(publisherCallbackURL)
      setCopiedPublisher(true)
      setTimeout(() => setCopiedPublisher(false), 2000)
    } catch {
      // ignore
    }
  }

  async function handleReconnect() {
    setConnectError(null)
    // If this credential has never successfully connected (no connected_at), it has no saved creds.
    // Require the user to fill in api_key:api_secret before we can proceed.
    if (!credential?.connected_at && !form.raw_creds.trim()) {
      setConnectError('Enter your api_key:api_secret in the field above, then click Reconnect')
      return
    }
    setConnecting(true)
    // Always save the current form settings (exchange, product, etc.)
    // so changes are not lost when reconnecting without new API credentials.
    const payload: Record<string, unknown> = {
      broker_type: form.broker_type,
      account_label: form.account_label,
      account_mode: form.account_mode,
      execution_mode: form.execution_mode,
      exchange: form.exchange,
      ...indianProductFields(),
      market_protection: parseFloat(form.market_protection) || 0,
    }
    if (form.algo_id.trim()) payload.algo_id = form.algo_id.trim()
    if (form.raw_creds.trim()) payload.raw_creds = form.raw_creds.trim()
    try {
      await apiFetch<BrokerCredential>(`/v1/credentials/${credential!.id}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      })
    } catch (err) {
      setConnectError(err instanceof Error ? err.message : 'Failed to save credentials')
      setConnecting(false)
      return
    }
    try {
      const { redirect_url } = await apiFetch<{ redirect_url: string }>(
        `/v1/credentials/zerodha/connect?id=${credential!.id}`,
      )
      window.location.href = redirect_url
    } catch (err) {
      setConnectError(err instanceof Error ? err.message : 'Failed to initiate Zerodha OAuth')
      setConnecting(false)
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.broker_type) {
      setError('Please select a broker type')
      return
    }
    setError(null)
    setSubmitting(true)

    const payload: Record<string, unknown> = {
      broker_type: form.broker_type,
      account_label: form.account_label,
      account_mode: form.account_mode,
    }

    if (isZerodha) {
      payload.execution_mode = isZerodhaNoOAuth ? 'user_api_oauth' : form.execution_mode
      if (isPublisherMode) {
        // Publisher mode: raw_creds is just the API key (required on create, optional on edit).
        if (mode === 'create' && !form.raw_creds.trim()) {
          setError('Publisher App API Key is required')
          setSubmitting(false)
          return
        }
        if (form.raw_creds.trim()) payload.raw_creds = form.raw_creds.trim()
      } else {
        // user_api_oauth edit: include raw_creds if the user wants to update their API key/secret.
        if (mode === 'edit' && form.raw_creds.trim()) {
          payload.raw_creds = form.raw_creds.trim()
        }
      }
    } else {
      if (form.raw_creds.trim()) {
        payload.raw_creds = form.raw_creds.trim()
      } else if (mode === 'create') {
        setError('Credentials are required')
        setSubmitting(false)
        return
      }
    }

    if (isIndian) {
      payload.exchange = form.exchange
      Object.assign(payload, indianProductFields())
      if (form.algo_id.trim()) payload.algo_id = form.algo_id.trim()
      payload.market_protection = parseFloat(form.market_protection) || 0
    }

    try {
      if (mode === 'create') {
        await apiFetch<BrokerCredential>('/v1/credentials', {
          method: 'POST',
          body: JSON.stringify(payload),
        })
      } else {
        await apiFetch<BrokerCredential>(`/v1/credentials/${credential!.id}`, {
          method: 'PUT',
          body: JSON.stringify(payload),
        })
      }
      router.push('/credentials')
      router.refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
      setSubmitting(false)
    }
  }

  // Free-plan users can't create a new live broker credential.
  if (mode === 'create' && !canLive) {
    return (
      <Card className="max-w-2xl">
        <CardHeader>
          <CardTitle className="text-base">Live broker connections require an upgrade</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Live broker connections require a Pro plan. To simulate trading for free, create a{' '}
            <a href="/paper-accounts/new" className="text-primary underline">Paper Trading Account</a>{' '}
            instead — or{' '}
            <a href="/billing" className="text-primary underline">upgrade</a>{' '}
            to connect a live broker.
          </p>
        </CardContent>
      </Card>
    )
  }

  return (
    <form onSubmit={handleSubmit} className="max-w-2xl space-y-6">
      {error && (
        <div className="rounded-md bg-destructive/10 px-4 py-2 text-sm text-destructive">{error}</div>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Broker</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-1.5">
            <Label htmlFor="broker_type">Broker *</Label>
            <Select
              id="broker_type"
              required
              value={form.broker_type}
              onChange={e => set('broker_type', e.target.value as BrokerType)}
              disabled={mode === 'edit'}
            >
              <option value="">Select broker…</option>
              {BROKER_OPTIONS.filter(o =>
                mode === 'create'
                  ? BROKER_OPTIONS_VISIBLE_ON_CREATE.includes(o.value)
                  : true,
              ).map(o => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </Select>
            {mode === 'edit' && (
              <p className="text-xs text-muted-foreground">Broker cannot be changed after creation</p>
            )}
          </div>

          {mode === 'create' && (
            <p className="text-xs text-muted-foreground">
              {isZerodha ? (
                <>
                  Choose how ZettaBridge should connect with Zerodha. For simulated testing
                  without broker execution,{' '}
                  <a href="/paper-accounts/new" className="text-primary underline">
                    create a Paper Trading Account
                  </a>{' '}
                  instead.
                </>
              ) : (
                <>
                  Choose the broker you want to connect for live execution. This connection can be
                  used only after credentials are added and webhook guards are configured. Need
                  simulated trading?{' '}
                  <a href="/paper-accounts/new" className="text-primary underline">
                    Create a Paper Trading Account
                  </a>{' '}
                  instead.
                </>
              )}
            </p>
          )}

          <div className="grid gap-1.5">
            <Label htmlFor="account_label">Label (optional)</Label>
            <Input
              id="account_label"
              placeholder={isZerodha ? 'My Zerodha Live account' : 'My Live account'}
              value={form.account_label}
              onChange={e => set('account_label', e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {isZerodha
                ? isPublisherMode
                  ? 'Give this connection a friendly name, e.g. "Zerodha Publisher Live". This helps when you have multiple broker connections.'
                  : 'Give this connection a friendly name, e.g. "Zerodha OAuth Live". This helps when you have multiple broker connections.'
                : 'Give this connection a friendly name so you can identify it later.'}
            </p>
          </div>
        </CardContent>
      </Card>

      {isZerodha && !isZerodhaNoOAuth && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Execution mode</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col gap-3">
              <label
                className={`flex items-start gap-2.5 ${publisherEnabled ? 'cursor-pointer' : 'cursor-not-allowed opacity-50'}`}
              >
                <input
                  type="radio"
                  name="execution_mode"
                  value="publisher"
                  checked={form.execution_mode === 'publisher'}
                  onChange={() => set('execution_mode', 'publisher')}
                  disabled={!publisherEnabled}
                  className="mt-0.5 shrink-0"
                />
                <div>
                  <span className="text-sm font-medium">
                    Kite Publisher{' '}
                    {!publisherEnabled && (
                      <span className="text-xs font-normal text-muted-foreground">(Disabled by admin)</span>
                    )}
                  </span>
                  <p className="text-xs text-muted-foreground">
                    ZettaBridge prepares a Kite basket. You review and submit each order inside Kite before execution. No API secret, order-placement permission, or daily token refresh is required.
                  </p>
                </div>
              </label>
              <label
                className={`flex items-start gap-2.5 ${oauthEnabled ? 'cursor-pointer' : 'cursor-not-allowed opacity-50'}`}
              >
                <input
                  type="radio"
                  name="execution_mode"
                  value="user_api_oauth"
                  checked={form.execution_mode === 'user_api_oauth'}
                  onChange={() => set('execution_mode', 'user_api_oauth')}
                  disabled={!oauthEnabled}
                  className="mt-0.5 shrink-0"
                />
                <div>
                  <span className="text-sm font-medium">
                    User API OAuth{' '}
                    {!oauthEnabled && (
                      <span className="text-xs font-normal text-muted-foreground">(Disabled by admin)</span>
                    )}
                  </span>
                  <p className="text-xs text-muted-foreground">
                    Use your Kite Connect app to authorize ZettaBridge for direct API execution. Orders can be placed through the Kite Connect API after daily Kite login and token refresh, subject to webhook secret verification, symbol allowlists, quantity rules, action guards, and broker-side checks.
                  </p>
                </div>
              </label>
            </div>

            {isPublisherMode && (
              <div className="mt-3 rounded-md border border-blue-200 bg-blue-50/60 dark:border-blue-800 dark:bg-blue-950/30 px-3 py-2.5 text-xs text-blue-800 dark:text-blue-300">
                Publisher mode is a user-confirmed handoff flow. ZettaBridge prepares a Kite basket for review inside Kite. Publisher does not return fill price, fill status, rejection reason, or P&amp;L to ZettaBridge.
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {isIndian && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Indian market settings</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label htmlFor="exchange">Exchange *</Label>
                <Select
                  id="exchange"
                  value={form.exchange}
                  onChange={e => set('exchange', e.target.value)}
                >
                  <option value="NSE">NSE</option>
                  <option value="BSE">BSE</option>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label>{canMultiProduct ? 'Products *' : 'Product *'}</Label>
                {canMultiProduct ? (
                  <div className="space-y-2 rounded-md border px-3 py-2.5">
                    {INDIAN_PRODUCT_OPTIONS.map(option => (
                      <label
                        key={option.value}
                        className={`flex items-start gap-2 text-sm ${option.disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'}`}
                      >
                        <input
                          type="checkbox"
                          className="mt-0.5 h-4 w-4 rounded border-input"
                          checked={form.products.includes(option.value)}
                          onChange={() => toggleProduct(option.value)}
                          disabled={option.disabled}
                        />
                        <span>{option.label}</span>
                      </label>
                    ))}
                  </div>
                ) : (
                  <Select
                    id="product"
                    value={form.products[0] ?? 'MIS'}
                    onChange={e => setSingleProduct(e.target.value as IndianProduct)}
                  >
                    {INDIAN_PRODUCT_OPTIONS.map(option => (
                      <option key={option.value} value={option.value} disabled={option.disabled}>
                        {option.label}
                      </option>
                    ))}
                  </Select>
                )}
                {canMultiProduct && (
                  <p className="text-xs text-muted-foreground">
                    Pro Plus can enable MIS and CNC on one broker connection. NRML will be
                    available once F&amp;O trading is supported. When more than one product is
                    enabled, each webhook signal must include{' '}
                    <code className="font-mono">&quot;product&quot;</code>.
                  </p>
                )}
              </div>
            </div>

            {!isPublisherMode && (
              <div className="grid gap-1.5">
                <Label htmlFor="market_protection">Market protection / fallback slippage %</Label>
                <Input
                  id="market_protection"
                  type="number"
                  min="0"
                  max="100"
                  step="0.1"
                  placeholder="2"
                  value={form.market_protection}
                  onChange={e => set('market_protection', e.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  For MARKET orders, this value is sent as Zerodha market protection where
                  supported. For LIMIT orders without a signal price, ZettaBridge may use LTP ±
                  this percentage to calculate a fallback limit price.
                </p>
              </div>
            )}

            <div className="grid gap-1.5">
              <Label htmlFor="algo_id">{isPublisherMode ? 'SEBI Algo ID' : 'SEBI Algo ID (optional)'}</Label>
              <Input
                id="algo_id"
                placeholder={isPublisherMode ? 'Not required for Publisher handoff' : 'ALGO123456'}
                value={isPublisherMode ? '' : form.algo_id}
                onChange={e => set('algo_id', e.target.value)}
                disabled={isPublisherMode}
              />
              <p className="text-xs text-muted-foreground">
                {isPublisherMode
                  ? 'Kite Publisher is a user-confirmed basket flow. ZettaBridge does not place orders directly in Publisher mode.'
                  : 'Required for live Indian market algo orders where applicable under SEBI, exchange, or broker rules.'}
              </p>
            </div>
          </CardContent>
        </Card>
      )}

      {isZerodha ? (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              {mode === 'create'
                ? (isPublisherMode ? 'Connect with Zerodha Kite Publisher' : 'Connect with Zerodha')
                : (isPublisherMode ? 'Zerodha Kite Publisher connection' : 'Zerodha connection')}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {isZerodhaNoOAuth ? (
              mode === 'create' ? (
                <>
                  <p className="text-sm text-muted-foreground">
                    Your plan does not include live order routing, so no Zerodha login is needed. You can still add Zerodha to configure your settings — all orders will run as paper trades until you upgrade.
                  </p>
                  <Button type="submit" disabled={submitting}>
                    {submitting && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}
                    Create
                  </Button>
                </>
              ) : (
                <p className="text-sm text-muted-foreground">
                  Your plan does not include live order routing, so no Zerodha login is needed.
                  All orders run as paper trades.{' '}
                  <a href="/billing" className="text-primary underline">Upgrade</a>{' '}
                  to connect a real Zerodha account.
                </p>
              )
            ) : (
              <>
                {isPublisherMode ? (
                  // Publisher mode: redirect URL + single API key, no OAuth
                  <>
                    {/* Step 1: Configure redirect URL in Kite Publisher app */}
                    <div className="rounded-lg border border-border bg-muted/30 p-4 space-y-3">
                      <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">Step 1 — Set up your Kite Publisher app</p>
                      <p className="text-sm text-muted-foreground">
                        Create a Kite Publisher app on{' '}
                        <a
                          href="https://developers.kite.trade"
                          target="_blank"
                          rel="noopener noreferrer"
                          className="text-primary underline inline-flex items-center gap-0.5"
                        >
                          developers.kite.trade <ExternalLink className="h-3 w-3" />
                        </a>
                        {' '}and add this redirect URL for the current environment:
                      </p>
                      <div className="flex items-center gap-2">
                        <code className="flex-1 rounded bg-background border border-border px-3 py-1.5 text-xs font-mono text-foreground break-all">
                          {publisherCallbackURL ?? 'Loading…'}
                        </code>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={!publisherCallbackURL}
                          onClick={handleCopyPublisherCallback}
                          className="shrink-0"
                        >
                          {copiedPublisher
                            ? <CheckCircle className="h-3.5 w-3.5 text-green-600" />
                            : <Copy className="h-3.5 w-3.5" />}
                        </Button>
                      </div>
                    </div>

                    {/* Step 2: Enter API key */}
                    <div className="space-y-2">
                      <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">
                        Step 2 — Enter your Publisher App API key
                      </p>
                    </div>
                    <div className="grid gap-1.5">
                      <Label htmlFor="raw_creds">
                        {mode === 'create' ? 'Publisher App API Key *' : 'Publisher App API Key (leave blank to keep existing)'}
                      </Label>
                      <Input
                        id="raw_creds"
                        placeholder="api_key"
                        value={form.raw_creds}
                        onChange={e => set('raw_creds', e.target.value)}
                        className="font-mono text-xs"
                      />
                      <p className="text-xs text-muted-foreground">
                        Copy the API key from your Kite Publisher app. API secret is not required and should not be entered for Publisher mode.
                      </p>
                    </div>
                  </>
                ) : (
                  // user_api_oauth: Step 1 callback URL + Step 2 API key/secret
                  <>
                    <div className="rounded-md border border-blue-200 bg-blue-50/60 dark:border-blue-800 dark:bg-blue-950/30 px-3 py-2.5 text-xs text-blue-800 dark:text-blue-300">
                      Zerodha OAuth uses secure broker authorization. You will be redirected to
                      Zerodha to approve access. ZettaBridge does not ask for or store your
                      Zerodha password.
                    </div>

                    {/* Step 1: Create Kite Connect app and configure callback URL */}
                    <div className="rounded-lg border border-border bg-muted/30 p-4 space-y-3">
                      <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">Step 1 — Set up your Kite Connect app</p>
                      <p className="text-sm text-muted-foreground">
                        Create your own Kite Connect app on{' '}
                        <a
                          href="https://developers.kite.trade"
                          target="_blank"
                          rel="noopener noreferrer"
                          className="text-primary underline inline-flex items-center gap-0.5"
                        >
                          developers.kite.trade <ExternalLink className="h-3 w-3" />
                        </a>
                        {' '}and set the redirect URL for the current environment:
                      </p>
                      <div className="flex items-center gap-2">
                        <code className="flex-1 rounded bg-background border border-border px-3 py-1.5 text-xs font-mono text-foreground break-all">
                          {callbackURL ?? 'Loading…'}
                        </code>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={!callbackURL}
                          onClick={handleCopyCallback}
                          className="shrink-0"
                        >
                          {copied
                            ? <CheckCircle className="h-3.5 w-3.5 text-green-600" />
                            : <Copy className="h-3.5 w-3.5" />}
                        </Button>
                      </div>
                    </div>

                    {/* Step 2: Enter API key and secret */}
                    <div className="space-y-2">
                      <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">
                        Step 2 — Enter your API key and secret
                      </p>
                      <div className="grid gap-1.5">
                        <Label htmlFor="raw_creds">
                          {mode === 'create' ? 'API Key : API Secret *' : 'API Key : API Secret (leave blank to keep existing)'}
                        </Label>
                        <Textarea
                          id="raw_creds"
                          placeholder="api_key:api_secret"
                          value={form.raw_creds}
                          onChange={e => set('raw_creds', e.target.value)}
                          className="font-mono text-xs"
                          rows={2}
                        />
                        <p className="text-xs text-muted-foreground">
                          Format:{' '}
                          <code className="rounded bg-muted px-1 py-0.5">api_key:api_secret</code>
                          {' '}— copy both from your Kite Connect app dashboard, separated by a colon.
                          The API secret is encrypted at rest and is never shown again after
                          saving.
                        </p>
                      </div>
                    </div>

                    <div className="rounded-md border border-amber-200 bg-amber-50/60 dark:border-amber-800 dark:bg-amber-950/30 px-3 py-2.5 text-xs text-amber-800 dark:text-amber-300">
                      <p className="font-medium">Live execution notice</p>
                      <p className="mt-1">
                        User API OAuth can place real orders directly in the connected Zerodha
                        account when a valid webhook signal is received. Review symbol allowlists,
                        allowed actions, quantity rules, product settings, market protection, SEBI
                        Algo ID, and webhook secret before enabling live webhooks.
                      </p>
                    </div>

                    {/* Action buttons */}
                    {mode === 'create' ? (
                      <div className="flex gap-3">
                        <Button
                          type="button"
                          onClick={handleSaveAndConnect}
                          disabled={submitting || connecting}
                        >
                          {(submitting || connecting) && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}
                          {connecting ? 'Redirecting…' : submitting ? 'Saving…' : 'Save & Connect with Zerodha'}
                        </Button>
                      </div>
                    ) : (
                      <div className="space-y-2">
                        <p className="text-xs text-muted-foreground">
                          Kite tokens expire daily at 6:00 AM IST. Click Reconnect to refresh your access token.
                          If your API key or secret has changed, fill it in above — Reconnect will save and connect in one step.
                        </p>
                        <Button
                          type="button"
                          variant="outline"
                          onClick={handleReconnect}
                          disabled={connecting}
                        >
                          {connecting
                            ? <Loader2 className="mr-1 h-4 w-4 animate-spin" />
                            : <ExternalLink className="mr-1 h-4 w-4" />}
                          {connecting ? 'Redirecting…' : 'Reconnect with Zerodha'}
                        </Button>
                        {connectError && (
                          <p className="text-sm text-destructive">{connectError}</p>
                        )}
                      </div>
                    )}
                  </>
                )}
              </>
            )}
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              {mode === 'create' ? 'Credentials' : 'Update credentials (optional)'}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-1.5">
              <Label htmlFor="raw_creds">
                {mode === 'create' ? 'Credentials *' : 'New credentials (leave blank to keep existing)'}
              </Label>
              <Textarea
                id="raw_creds"
                placeholder={
                  form.broker_type
                    ? FORMAT_HINTS[form.broker_type as BrokerType]
                    : 'Select a broker to see the format…'
                }
                value={form.raw_creds}
                onChange={e => set('raw_creds', e.target.value)}
                className="font-mono text-xs"
                rows={2}
              />
              {form.broker_type && FORMAT_HINTS[form.broker_type as BrokerType] && (
                <p className="text-xs text-muted-foreground">
                  Format:{' '}
                  <code className="rounded bg-muted px-1 py-0.5">
                    {FORMAT_HINTS[form.broker_type as BrokerType]}
                  </code>
                  {' '}— values separated by colons, no spaces
                </p>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Bottom action bar: submit + cancel.
          Zerodha user_api_oauth create has its own button inside the card above.
          Publisher mode create and all edit modes use this button. */}
      <div className="flex gap-3">
        {(!isZerodha || mode === 'edit' || isPublisherMode) && (
          <Button type="submit" disabled={submitting}>
            {submitting && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}
            {mode === 'create'
              ? (isPublisherMode ? 'Save Publisher Credential' : 'Add credential')
              : 'Save changes'}
          </Button>
        )}
        <Button type="button" variant="outline" onClick={() => router.push('/credentials')}>
          Cancel
        </Button>
      </div>
    </form>
  )
}
