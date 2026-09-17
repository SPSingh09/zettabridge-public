'use client'

import { useState, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { Loader2, TriangleAlert } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { apiFetch } from '@/lib/api'
import { useUser } from '@/lib/user-context'
import {
  decodeDestinationId,
  encodeCredDestination,
  encodePaperDestination,
  formatCredentialLabel,
  formatPaperAccountLabel,
  isPaperDestination as isPaperDestinationValue,
} from '@/lib/executionDestination'
import type { Webhook, BrokerCredential, PaperAccount } from '@/lib/types'

const ACTIONS = ['BUY', 'SELL', 'CLOSE']

const MT5_BROKERS = ['mt5_cloud']
const INDIAN_BROKERS = ['zerodha', 'angel', 'dhan']

const BROKER_LABELS: Record<string, string> = {
  mt5_cloud: 'MetaTrader 5',
  zerodha: 'Zerodha',
  angel: 'Angel One',
  dhan: 'Dhan',
}

// Existing 'demo' credentials (pre-dating Paper Trading accounts) keep showing
// Demo; new credential creation no longer offers Demo (see CredentialForm.tsx).
function credOptionLabel(c: BrokerCredential): string {
  // Keep the user's own credential name first so multiple credentials for
  // the same broker stay distinguishable in the dropdown — this was lost
  // when the mode-specific labels below were added and needs to stay.
  const name = c.account_label
  // Publisher mode is manual-confirmation only — "Live" here could read as
  // "orders are placed live automatically", which is the opposite of how
  // Publisher actually works. Say so directly instead.
  if (c.execution_mode === 'publisher') {
    return name ? `${name} · Publisher · Manual` : 'Publisher · Manual'
  }
  // OAuth/direct-API credentials place real broker orders automatically —
  // spell that out in the destination label itself, not just in copy below
  // it, so it reads correctly even collapsed in the dropdown.
  if (c.broker_type === 'zerodha') {
    const mode = c.account_mode === 'live' ? 'OAuth · Live' : 'OAuth · Demo'
    return name ? `${name} · ${mode}` : mode
  }
  return formatCredentialLabel(c, { brokerLabels: BROKER_LABELS })
}

function lotSizeProps(brokerType: string | undefined, isPublisher: boolean, isLiveOAuth: boolean) {
  // MT5/forex is the only broker that actually deals in "lots" — default to
  // the plain share-quantity framing everywhere else, including before a
  // destination is picked (this product is Indian-equity/Zerodha-first).
  if (brokerType && MT5_BROKERS.includes(brokerType)) {
    return {
      label: 'Default quantity in lots',
      step: '0.01',
      min: '0',
      hint: '0 = each signal must include quantity. Any non-zero value is always used, even when the signal includes quantity.',
      placeholder: '0.01',
    }
  }
  if (isPublisher) {
    // Publisher only ever creates explicit BUY/SELL basket items — avoid
    // "entry and exit" framing, which could read as ZettaBridge managing
    // exits automatically (it doesn't; the user confirms every basket).
    return {
      label: 'Default quantity',
      step: '1',
      min: '0',
      hint: '0 = each signal must include quantity. Any non-zero value is used for every BUY/SELL basket item, even when the signal includes quantity. Use valid lot size for F&O instruments.',
      placeholder: '1',
    }
  }
  if (isLiveOAuth) {
    // OAuth/direct-API places orders automatically for every allowed action,
    // including CLOSE — spell that out since a non-zero override here
    // silently changes exit order sizing too.
    return {
      label: 'Default quantity',
      step: '1',
      min: '0',
      hint: '0 = each signal must include quantity. Any non-zero value configured here overrides the signal quantity for all BUY, SELL, and CLOSE actions. Use valid lot size for F&O instruments.',
      placeholder: '1',
    }
  }
  return {
    label: 'Default quantity',
    step: '1',
    min: '0',
    hint: '0 = each signal must include quantity. Any non-zero value configured here overrides the signal quantity for all BUY, SELL, and CLOSE actions. Use valid lot size for F&O instruments.',
    placeholder: '1',
  }
}

function parseAllowedSymbols(raw: string): string[] {
  return raw
    .split(',')
    .map(s => s.trim().toUpperCase())
    .filter(Boolean)
}

function formatAllowedSymbols(wh: Webhook): string {
  if (wh.allowed_symbols?.length) {
    return wh.allowed_symbols.join(', ')
  }
  if (wh.symbol) {
    return wh.symbol
  }
  return ''
}

interface FormValues {
  label: string
  destination: string
  lot_size: string
  default_order_type: string
  allowed_actions: string[]
  allowed_symbols: string
  dedup_window_sec: string
  required_comment: string
}

function toForm(wh: Webhook): FormValues {
  return {
    label: wh.label,
    destination: wh.paper_account_id
      ? encodePaperDestination(wh.paper_account_id)
      : encodeCredDestination(wh.broker_cred_id ?? ''),
    lot_size: String(wh.lot_size),
    default_order_type: wh.default_order_type || 'LIMIT',
    allowed_actions: wh.allowed_actions ?? ACTIONS,
    allowed_symbols: formatAllowedSymbols(wh),
    dedup_window_sec: String(wh.dedup_window_sec ?? 0),
    required_comment: wh.required_comment ?? '',
  }
}

function makeDefaultForm(isPaid: boolean): FormValues {
  return {
    label: '',
    destination: '',
    lot_size: '0',
    default_order_type: 'LIMIT',
    allowed_actions: ACTIONS,
    allowed_symbols: '',
    dedup_window_sec: isPaid ? '60' : '0',
    required_comment: '',
  }
}

interface Props {
  mode: 'create' | 'edit'
  webhook?: Webhook
  // Set only on the create-mode "go live" flow (Phase 8): pre-fills every
  // field from an existing paper webhook except `destination` (left blank —
  // never silently default to a live credential) and `label` (suffixed so
  // the two webhooks are easy to tell apart in the list).
  cloneFrom?: Webhook
}

function toCloneForm(wh: Webhook): FormValues {
  return {
    ...toForm(wh),
    destination: '',
    label: wh.label ? `${wh.label} (Live)` : 'Live',
  }
}

export function WebhookForm({ mode, webhook, cloneFrom }: Props) {
  const router = useRouter()
  const { user } = useUser()
  const isPaid = user.plan !== 'free'
  const [creds, setCreds] = useState<BrokerCredential[]>([])
  const [paperAccounts, setPaperAccounts] = useState<PaperAccount[]>([])
  const [form, setForm] = useState<FormValues>(
    webhook ? toForm(webhook) : cloneFrom ? toCloneForm(cloneFrom) : makeDefaultForm(isPaid)
  )
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [showSwitchConfirm, setShowSwitchConfirm] = useState(false)

  useEffect(() => {
    apiFetch<BrokerCredential[]>('/v1/credentials').then(setCreds).catch(() => {})
    apiFetch<PaperAccount[]>('/v1/paper-accounts').then(setPaperAccounts).catch(() => {})
  }, [])

  const isPaperDestination = isPaperDestinationValue(form.destination)
  // Paper Trading is a flat, plan-independent tier — none of its guard
  // configuration is gated by the live-trading plan (see backend
  // EnforceAdvancedGuardsPlan, which exempts paper webhooks the same way).
  const guardsUnlocked = isPaid || isPaperDestination
  const selectedCred = isPaperDestination
    ? undefined
    : creds.find(c => encodeCredDestination(c.id) === form.destination)
  const selectedPaperAccount = isPaperDestination
    ? paperAccounts.find(a => encodePaperDestination(a.id) === form.destination)
    : undefined
  const activeCreds = creds.filter(c => c.status !== 'paused')
  // Paused credentials can't be newly selected (they may be admin-disabled or
  // manually paused), but a webhook already pointed at one must keep showing
  // it — as a disabled option — so the current selection still renders.
  const selectableCreds = creds.filter(
    c => c.status !== 'paused' || encodeCredDestination(c.id) === form.destination
  )
  // Paper Trading MVP is Indian Equity only — treat it like an Indian broker
  // for copy/placeholders. Also defaults to Indian-equity copy before a
  // destination is picked at all (no selectedCred yet) — this product is
  // Zerodha/Indian-equity-first, so forex placeholders should only show once
  // an MT5 credential is actually selected, not as the initial blank state.
  const isIndianBroker =
    isPaperDestination || !selectedCred || INDIAN_BROKERS.includes(selectedCred.broker_type)
  const isPublisherCred = selectedCred?.execution_mode === 'publisher'
  // A live (non-publisher) broker credential is selected — orders are placed
  // automatically, unlike Publisher's manual-confirmation basket flow.
  const isLiveOAuth = !!selectedCred && !isPublisherCred
  // Paper Trading is Indian Equity — pass a non-MT5 sentinel so lotSizeProps
  // shows the share-quantity hint, not the "lots" hint.
  const lsp = lotSizeProps(
    isPaperDestination ? 'indian_equity' : selectedCred?.broker_type,
    isPublisherCred,
    isLiveOAuth
  )

  // Strip CLOSE from allowed_actions whenever a publisher credential is selected.
  useEffect(() => {
    if (isPublisherCred && form.allowed_actions.includes('CLOSE')) {
      set('allowed_actions', form.allowed_actions.filter(a => a !== 'CLOSE'))
    }
  }, [isPublisherCred]) // eslint-disable-line react-hooks/exhaustive-deps

  // Publisher webhooks are LIMIT-only.
  useEffect(() => {
    if (isPublisherCred && form.default_order_type !== 'LIMIT') {
      set('default_order_type', 'LIMIT')
    }
  }, [isPublisherCred]) // eslint-disable-line react-hooks/exhaustive-deps

  function set<K extends keyof FormValues>(key: K, val: FormValues[K]) {
    setForm(prev => ({ ...prev, [key]: val }))
  }

  function toggleAction(action: string) {
    const next = form.allowed_actions.includes(action)
      ? form.allowed_actions.filter(a => a !== action)
      : [...form.allowed_actions, action]
    set('allowed_actions', next)
  }

  // In edit mode, the original destination the webhook loaded with — used to
  // detect a genuine destination change and gate it behind confirmation.
  const originalDestination = webhook ? toForm(webhook).destination : ''
  const destinationChanged = mode === 'edit' && form.destination !== originalDestination && !!form.destination

  function destinationSwitchWarning(): string {
    const wasPaperBefore = isPaperDestinationValue(originalDestination)
    const target = isPaperDestination
      ? (selectedPaperAccount ? formatPaperAccountLabel(selectedPaperAccount) : 'the selected paper account')
      : (selectedCred ? credOptionLabel(selectedCred) : 'the selected broker credential')
    if (wasPaperBefore && !isPaperDestination) {
      return `This webhook will stop executing on Paper Trading and start placing real orders through ${target} instead. Continue?`
    }
    if (!wasPaperBefore && isPaperDestination) {
      return `This webhook will stop placing real orders and switch to simulated Paper Trading (${target}) instead. Continue?`
    }
    return `This webhook's execution destination will change to ${target}. Continue?`
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.destination) {
      setError('Please select an execution destination')
      return
    }
    if (!form.required_comment.trim()) {
      setError('Webhook secret is required — it prevents unauthorized signals from reaching your destination')
      return
    }
    if (parseAllowedSymbols(form.allowed_symbols).length === 0) {
      setError('At least one allowed symbol is required')
      return
    }
    setError(null)
    if (destinationChanged) {
      setShowSwitchConfirm(true)
      return
    }
    submitPayload()
  }

  async function submitPayload() {
    setSubmitting(true)

    const syms = parseAllowedSymbols(form.allowed_symbols)
    const payload: Record<string, unknown> = {
      label: form.label,
      lot_size: Number(form.lot_size) || 0,
      default_order_type: isPublisherCred ? 'LIMIT' : form.default_order_type,
      allowed_symbols: syms,
    }
    if (isPaperDestination) {
      payload.paper_account_id = decodeDestinationId(form.destination)
    } else {
      payload.broker_cred_id = decodeDestinationId(form.destination)
    }

    // Only send allowed_actions if the user explicitly chose some
    if (form.allowed_actions.length > 0) {
      payload.allowed_actions = form.allowed_actions
    }

    if (Number(form.dedup_window_sec) > 0)
      payload.dedup_window_sec = Number(form.dedup_window_sec)
    if (form.required_comment.trim())
      payload.required_comment = form.required_comment.trim()

    try {
      if (mode === 'create') {
        const created = await apiFetch<Webhook>('/v1/webhooks', {
          method: 'POST',
          body: JSON.stringify(payload),
        })
        // The list endpoint never returns the raw token (it's only stored as a hash).
        // Stash it in sessionStorage so the webhooks list page can show the copy URL button.
        if (created?.id && created?.token) {
          try { sessionStorage.setItem('newWebhookToken', JSON.stringify({ id: created.id, token: created.token })) } catch { /* ignore */ }
        }
      } else {
        await apiFetch<Webhook>(`/v1/webhooks/${webhook!.id}`, {
          method: 'PUT',
          body: JSON.stringify(payload),
        })
      }
      router.push('/webhooks')
      router.refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
      setSubmitting(false)
    }
  }

  return (
    <>
    <form onSubmit={handleSubmit} className="max-w-2xl space-y-6">
      {error && (
        <div className="rounded-md bg-destructive/10 px-4 py-2 text-sm text-destructive">{error}</div>
      )}

      {cloneFrom && (
        <Alert variant="warning">
          <TriangleAlert className="h-4 w-4" />
          <AlertDescription>
            Live trading can result in financial loss. Review all symbols, quantities, order
            types, and risk limits before enabling live execution. Cloned from your paper webhook
            "{cloneFrom.label || 'Unnamed'}" — every field below is editable; pick a live broker
            credential to activate.
          </AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">General</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-1.5">
            <Label htmlFor="label">Webhook name</Label>
            <Input
              id="label"
              placeholder="TradingView Indian Equity"
              value={form.label}
              onChange={e => set('label', e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              A friendly name to identify this endpoint in your dashboard.
            </p>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="destination">Execution destination *</Label>
            <Select
              id="destination"
              required
              value={form.destination}
              onChange={e => set('destination', e.target.value)}
            >
              <option value="">Select a destination…</option>
              {paperAccounts.length > 0 && (
                <optgroup label="Paper account">
                  {paperAccounts.map(a => (
                    <option key={a.id} value={encodePaperDestination(a.id)}>
                      {a.label || 'Unnamed account'}
                    </option>
                  ))}
                </optgroup>
              )}
              {selectableCreds.length > 0 && (
                <optgroup label="Live broker">
                  {selectableCreds.map(c => {
                    const isCurrent = encodeCredDestination(c.id) === form.destination
                    const isPaused = c.status === 'paused'
                    return (
                      <option
                        key={c.id}
                        value={encodeCredDestination(c.id)}
                        disabled={isPaused}
                      >
                        {credOptionLabel(c)}{isPaused && isCurrent ? ' (Paused)' : ''}
                      </option>
                    )
                  })}
                </optgroup>
              )}
            </Select>
            {creds.length === 0 && paperAccounts.length === 0 && (
              <p className="text-xs text-amber-600">
                No execution destinations yet.{' '}
                <Link href="/paper-accounts/new" className="underline">Add a paper trading account</Link>{' '}
                or <Link href="/credentials/new" className="underline">a live broker credential</Link>.
              </p>
            )}
            {activeCreds.length === 0 && creds.length > 0 && (
              <p className="text-xs text-amber-600">
                All of your live broker connections are paused.{' '}
                <Link href="/credentials" className="underline">Resume one on Broker Accounts</Link>{' '}
                before selecting it here.
              </p>
            )}
            {cloneFrom && creds.length === 0 && (
              <p className="text-xs text-amber-600">
                You don't have a live broker credential yet.{' '}
                <Link href="/credentials/new" className="underline">Create one first</Link>, then come back
                here to finish going live.
              </p>
            )}
            {isPaperDestination && (
              <div className="rounded-md border border-blue-200 bg-blue-50/60 dark:border-blue-800 dark:bg-blue-950/30 px-3 py-2.5 text-sm text-blue-900 dark:text-blue-200">
                <p className="font-medium">Paper Account · Simulated Execution</p>
                {selectedPaperAccount && (
                  <p className="mt-0.5 text-xs text-blue-800/80 dark:text-blue-300/80">
                    Account: {selectedPaperAccount.label || 'Unnamed account'}
                  </p>
                )}
                <p className="mt-1.5 text-xs text-blue-800 dark:text-blue-300">
                  Signals are executed inside ZettaBridge for testing and strategy validation. No
                  real broker orders are placed.
                </p>
                <details className="mt-2 text-xs text-blue-800 dark:text-blue-300">
                  <summary className="cursor-pointer font-medium text-blue-900 hover:underline dark:text-blue-200">
                    Learn more
                  </summary>
                  <ul className="mt-2 list-disc space-y-1.5 pl-4">
                    <li>
                      Paper trading is simulated — it does not connect to Zerodha, Angel, Dhan,
                      or MT5.
                    </li>
                    <li>
                      Include a <code className="font-mono">&#34;price&#34;</code> field in each
                      signal (e.g. TradingView&apos;s <code className="font-mono">{'{{close}}'}</code>)
                      {' '}for LIMIT orders.
                    </li>
                    <li>
                      Open positions and MARKET fills can use live market data when available.
                    </li>
                  </ul>
                </details>
              </div>
            )}
            {selectedCred && !isPublisherCred && (
              <>
                <p className="text-xs text-muted-foreground">
                  Credential: {selectedCred.account_label || credOptionLabel(selectedCred)}
                </p>
                <p className="text-xs text-muted-foreground">
                  Signals are routed to the connected broker account and can place live orders
                  only after webhook validation, symbol allowlist checks, action checks, quantity
                  checks, and secret verification pass.
                </p>
              </>
            )}
            {isPublisherCred && (
              <div className="rounded-md border border-blue-200 bg-blue-50/60 dark:border-blue-800 dark:bg-blue-950/30 px-3 py-2.5 text-xs text-blue-800 dark:text-blue-300">
                <p className="font-medium">Kite Publisher</p>
                <p className="mt-1">
                  Signals create Kite baskets for manual review. ZettaBridge does not place
                  orders automatically; the user must review and confirm the basket inside Kite.
                </p>
              </div>
            )}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="allowed_symbols">Allowed symbols *</Label>
            <Input
              id="allowed_symbols"
              required
              placeholder={isIndianBroker ? 'RELIANCE, INFY, TCS' : 'EURUSD, GBPUSD'}
              value={form.allowed_symbols}
              onChange={e => set('allowed_symbols', e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {!isIndianBroker
                ? 'Comma-separated symbols this webhook accepts. Every signal symbol must match one of these entries.'
                : isLiveOAuth
                  ? 'Comma-separated Zerodha tradingsymbols allowed for this webhook, e.g. RELIANCE, INFY, TCS. Every incoming signal symbol must exactly match one of these entries before any broker order is attempted.'
                  : isPaperDestination
                    ? 'Comma-separated trading symbols allowed for this webhook, e.g. RELIANCE, INFY, TCS. Every incoming signal symbol must exactly match one of these entries.'
                    : 'Comma-separated Zerodha tradingsymbols allowed for this webhook, e.g. RELIANCE, INFY, TCS. Every incoming signal symbol must exactly match one of these entries.'}
            </p>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="lot_size">{lsp.label}</Label>
            <Input
              id="lot_size"
              type="number"
              step={lsp.step}
              min={lsp.min}
              placeholder={lsp.placeholder}
              value={form.lot_size}
              onChange={e => set('lot_size', e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{lsp.hint}</p>
          </div>

          {isIndianBroker && !isPublisherCred && (
            <div className="grid gap-1.5">
              <Label htmlFor="default_order_type">Order type</Label>
              <Select
                id="default_order_type"
                value={form.default_order_type}
                onChange={e => set('default_order_type', e.target.value)}
              >
                <option value="LIMIT">LIMIT</option>
                <option value="MARKET">MARKET</option>
              </Select>
              <p className="text-xs text-muted-foreground">
                {isPaperDestination
                  ? 'LIMIT uses the price provided in the signal. MARKET is simulated using the latest available quote.'
                  : 'LIMIT uses the signal price. If no price is provided, ZettaBridge may calculate a limit price using LTP ± configured slippage. MARKET sends a market order directly to the broker and should be used only when you understand the execution risk.'}
              </p>
            </div>
          )}

          {isPublisherCred && (
            <div className="grid gap-1.5">
              <Label htmlFor="default_order_type">Order type</Label>
              <Select id="default_order_type" value="LIMIT" disabled>
                <option value="LIMIT">LIMIT</option>
              </Select>
              <p className="text-xs text-muted-foreground">
                Publisher mode currently supports LIMIT orders only. Market orders are disabled for safer manual review. Every signal sent to this webhook must include a limit price.
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Guards (optional)</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-2">
            <Label>Allowed signal actions</Label>
            <div className="flex gap-4">
              {ACTIONS.filter(action => !(isPublisherCred && action === 'CLOSE')).map(action => (
                <label key={action} className="flex cursor-pointer items-center gap-1.5 text-sm">
                  <input
                    type="checkbox"
                    checked={form.allowed_actions.includes(action)}
                    onChange={() => toggleAction(action)}
                    className="h-4 w-4 rounded border-input"
                  />
                  {action}
                </label>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">
              {isPublisherCred
                ? 'Publisher mode supports only explicit BUY and SELL basket handoffs. The signal must include side, symbol, limit price, and quantity when default quantity is 0.'
                : isPaperDestination
                  ? 'All checked = allow BUY, SELL, and CLOSE signals. CLOSE reduces or exits the simulated paper position using the symbol, quantity, and price rules configured for this webhook.'
                  : 'All checked = allow BUY, SELL, and CLOSE signals. CLOSE is converted into the required opposite-side broker order using the symbol, quantity, and price rules configured for this webhook.'}
            </p>
          </div>

          <div className="grid gap-1.5">
            <div className="flex items-center gap-2">
              <Label htmlFor="dedup_window_sec">Dedup window (sec)</Label>
              {!guardsUnlocked && <span className="rounded bg-amber-100 px-1.5 py-0.5 text-xs font-medium text-amber-700">Pro</span>}
            </div>
            <Input
              id="dedup_window_sec"
              type="number"
              min="0"
              disabled={!guardsUnlocked}
              value={form.dedup_window_sec}
              onChange={e => set('dedup_window_sec', e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Suppress duplicate signals received within this many seconds. Set 0 to disable.
            </p>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="required_comment">Webhook secret *</Label>
            <Input
              id="required_comment"
              required
              placeholder="e.g. my-secret-2024"
              value={form.required_comment}
              onChange={e => set('required_comment', e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Your webhook URL is public. Anyone who knows it could send a signal
              {isPaperDestination ? ' to this paper account' : isPublisherCred ? ' and create a Kite basket' : ''}.
              Set a secret passphrase and include it in every alert payload as{' '}
              <code className="font-mono">&#34;comment&#34;: &#34;your-secret&#34;</code>.
              Signals without the exact passphrase are rejected before {isPaperDestination ? 'execution' : 'broker routing'}.
            </p>
          </div>

          {isPublisherCred && (
            <div className="rounded-md border border-amber-200 bg-amber-50/60 dark:border-amber-800 dark:bg-amber-950/30 px-3 py-2.5 text-xs text-amber-800 dark:text-amber-300">
              For safety, ZettaBridge allows only one active pending Publisher basket per credential. Complete, cancel, or let the current basket expire before creating a new one.
            </div>
          )}
        </CardContent>
      </Card>

      {isLiveOAuth && (
        <div className="rounded-md border border-amber-200 bg-amber-50/60 dark:border-amber-800 dark:bg-amber-950/30 px-3 py-2.5 text-xs text-amber-800 dark:text-amber-300">
          <p className="font-medium">Live execution notice</p>
          <p className="mt-1">
            This webhook can place real orders in the connected broker account when a valid
            signal is received. Review symbol allowlist, quantity, order type, allowed actions,
            and webhook secret before enabling.
          </p>
        </div>
      )}

      <div className="flex gap-3">
        <Button type="submit" disabled={submitting}>
          {submitting && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}
          {mode === 'create' ? 'Create Webhook' : 'Save changes'}
        </Button>
        <Button type="button" variant="outline" onClick={() => router.push('/webhooks')}>
          Cancel
        </Button>
      </div>
    </form>

    <ConfirmDialog
      open={showSwitchConfirm}
      title="Switch execution destination?"
      description={destinationSwitchWarning()}
      confirmLabel="Switch & Save"
      variant="destructive"
      onConfirm={() => { setShowSwitchConfirm(false); submitPayload() }}
      onCancel={() => setShowSwitchConfirm(false)}
    />
    </>
  )
}
