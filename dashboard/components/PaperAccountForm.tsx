'use client'

import { useState, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import useSWR from 'swr'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { apiFetch, listActiveMarketProfiles } from '@/lib/api'
import {
  formatCurrencyInput,
  parseCurrencyInput,
  profileCurrency,
} from '@/lib/marketDisplay'
import type { PaperAccount } from '@/lib/types'

interface FormValues {
  label: string
  starting_balance: string
  exchange: 'NSE' | 'BSE'
  default_product: 'MIS' | 'CNC' | 'NRML'
  market_profile: string
}

const defaultForm: FormValues = {
  label: '',
  starting_balance: '100000',
  exchange: 'NSE',
  default_product: 'MIS',
  market_profile: 'indian_equity',
}

/** Exchanges shown in the UI — add BSE here when fully supported. */
const EXCHANGE_OPTIONS: FormValues['exchange'][] = ['NSE']

const PRODUCT_LABELS: Record<FormValues['default_product'], string> = {
  MIS: 'MIS (Intraday)',
  CNC: 'CNC (Delivery)',
  NRML: 'NRML (Overnight F&O) — coming soon',
}

/** F&O trading isn't supported yet — keep NRML visible but unselectable. */
const DISABLED_PRODUCTS: FormValues['default_product'][] = ['NRML']

interface Props {
  mode: 'create'
}

export function PaperAccountForm({ mode }: Props) {
  const router = useRouter()
  const [form, setForm] = useState<FormValues>(defaultForm)
  const [balanceDisplay, setBalanceDisplay] = useState(() => formatCurrencyInput('100000', 'INR'))
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const { data: profiles } = useSWR('/v1/market-profiles', listActiveMarketProfiles)
  const currency = profileCurrency(profiles, form.market_profile)

  useEffect(() => {
    if (profiles && profiles.length > 0 && !profiles.some(p => p.code === form.market_profile)) {
      set('market_profile', profiles[0].code)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [profiles])

  useEffect(() => {
    setBalanceDisplay(formatCurrencyInput(form.starting_balance, currency))
  }, [currency, form.starting_balance])

  function set<K extends keyof FormValues>(key: K, val: FormValues[K]) {
    setForm(prev => ({ ...prev, [key]: val }))
  }

  function handleBalanceChange(raw: string) {
    const numeric = raw.replace(/[^\d.]/g, '')
    set('starting_balance', numeric)
    setBalanceDisplay(formatCurrencyInput(numeric, currency))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)

    const startingBalance = parseCurrencyInput(form.starting_balance)
    if (!startingBalance || startingBalance <= 0) {
      setError('Starting balance must be greater than zero')
      return
    }

    setSubmitting(true)
    try {
      await apiFetch<PaperAccount>('/v1/paper-accounts', {
        method: 'POST',
        body: JSON.stringify({
          label: form.label,
          starting_balance: startingBalance,
          exchange: form.exchange,
          default_product: form.default_product,
          market_profile: form.market_profile,
        }),
      })
      router.push('/paper-accounts')
      router.refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
      setSubmitting(false)
    }
  }

  const showExchange = EXCHANGE_OPTIONS.length > 1

  return (
    <form onSubmit={handleSubmit} className="max-w-2xl space-y-6">
      {error && (
        <div className="rounded-md bg-destructive/10 px-4 py-2 text-sm text-destructive">{error}</div>
      )}

      <div className="rounded-md border border-blue-200 bg-blue-50/60 dark:border-blue-800 dark:bg-blue-950/30 px-4 py-3 text-sm text-blue-900 dark:text-blue-200 space-y-1">
        <p className="font-medium">Paper Trading runs entirely inside ZettaBridge.</p>
        <p className="text-blue-800/90 dark:text-blue-300/90">
          No live broker connection is used, and no real orders are ever placed.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Account details</CardTitle>
          <CardDescription>
            Set up a simulated account for testing webhook strategies before using live broker
            execution.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-1.5">
            <Label htmlFor="label">Account name</Label>
            <Input
              id="label"
              placeholder="Indian Paper"
              value={form.label}
              onChange={e => set('label', e.target.value)}
            />
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="market_profile">Market</Label>
            <Select
              id="market_profile"
              value={form.market_profile}
              onChange={e => set('market_profile', e.target.value)}
              disabled={!profiles || profiles.length === 0}
            >
              {(profiles ?? []).map(p => (
                <option key={p.code} value={p.code}>
                  {p.name}
                </option>
              ))}
            </Select>
            <p className="text-xs text-muted-foreground">
              Determines currency, trading hours, supported products, and market rules.
            </p>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="starting_balance">Starting balance</Label>
            <Input
              id="starting_balance"
              inputMode="decimal"
              placeholder={formatCurrencyInput('100000', currency)}
              value={balanceDisplay}
              onChange={e => handleBalanceChange(e.target.value)}
            />
          </div>

          {showExchange && (
            <div className="grid gap-1.5">
              <Label htmlFor="exchange">Default exchange</Label>
              <Select
                id="exchange"
                value={form.exchange}
                onChange={e => set('exchange', e.target.value as FormValues['exchange'])}
              >
                {EXCHANGE_OPTIONS.map(ex => (
                  <option key={ex} value={ex}>
                    {ex}
                  </option>
                ))}
              </Select>
            </div>
          )}

          <div className="grid gap-1.5">
            <Label htmlFor="default_product">Default product</Label>
            <Select
              id="default_product"
              value={form.default_product}
              onChange={e => set('default_product', e.target.value as FormValues['default_product'])}
            >
              {(Object.keys(PRODUCT_LABELS) as FormValues['default_product'][]).map(p => (
                <option key={p} value={p} disabled={DISABLED_PRODUCTS.includes(p)}>
                  {PRODUCT_LABELS[p]}
                </option>
              ))}
            </Select>
            <p className="text-xs text-muted-foreground">
              Used when an incoming webhook signal does not specify a product.
            </p>
          </div>
        </CardContent>
      </Card>

      <div className="flex gap-3">
        <Button type="submit" disabled={submitting}>
          {submitting && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}
          {mode === 'create' ? 'Create account' : 'Save changes'}
        </Button>
        <Button type="button" variant="outline" onClick={() => router.push('/paper-accounts')}>
          Cancel
        </Button>
      </div>
    </form>
  )
}
