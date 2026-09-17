'use client'

import { useState, useEffect } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import useSWR from 'swr'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { createSymbolRequest, listActiveMarketProfiles } from '@/lib/api'
import { REQUEST_TYPE_LABELS, type RequestType } from '@/lib/requestDisplay'

interface FormValues {
  market_profile_code: string
  exchange: string
  symbol: string
  reason: string
}

const defaultForm: FormValues = {
  market_profile_code: '',
  exchange: 'NSE',
  symbol: '',
  reason: '',
}

const REQUEST_TYPES: RequestType[] = ['instrument', 'feature', 'broker', 'other']

export function RequestForm() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [requestType, setRequestType] = useState<RequestType>('instrument')
  const [form, setForm] = useState<FormValues>(defaultForm)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const { data: profiles } = useSWR('/v1/market-profiles', listActiveMarketProfiles)

  useEffect(() => {
    const symbol = searchParams.get('symbol')
    const profile = searchParams.get('profile')
    const exchange = searchParams.get('exchange')
    if (symbol || profile || exchange) {
      setRequestType('instrument')
      setForm(prev => ({
        ...prev,
        symbol: symbol ?? prev.symbol,
        market_profile_code: profile ?? prev.market_profile_code,
        exchange: exchange ?? prev.exchange ?? 'NSE',
      }))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (profiles && profiles.length > 0 && !form.market_profile_code) {
      set('market_profile_code', profiles[0].code)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [profiles])

  function set<K extends keyof FormValues>(key: K, val: FormValues[K]) {
    setForm(prev => ({ ...prev, [key]: val }))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)

    if (requestType !== 'instrument') return

    if (!form.market_profile_code || !form.symbol.trim()) {
      setError('Market and trading symbol are required')
      return
    }

    const reason = form.reason.trim() || 'Needed for paper trading.'

    setSubmitting(true)
    try {
      await createSymbolRequest({
        market_profile_code: form.market_profile_code,
        exchange: form.exchange.trim() || 'NSE',
        symbol: form.symbol.trim(),
        reason,
      })
      router.push('/requests')
      router.refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
      setSubmitting(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="max-w-2xl space-y-6">
      {error && (
        <div className="rounded-md bg-destructive/10 px-4 py-2 text-sm text-destructive">
          {error}
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Request type</CardTitle>
          <CardDescription>Choose what you need help with.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          {REQUEST_TYPES.map(type => {
            const enabled = type === 'instrument'
            return (
              <label
                key={type}
                className={`flex items-center gap-3 rounded-md border p-3 text-sm ${
                  enabled ? 'cursor-pointer has-[:checked]:border-primary has-[:checked]:bg-primary/5' : 'opacity-60'
                }`}
              >
                <input
                  type="radio"
                  name="request_type"
                  value={type}
                  checked={requestType === type}
                  disabled={!enabled}
                  onChange={() => enabled && setRequestType(type)}
                />
                <span className="flex flex-1 items-center justify-between gap-2">
                  <span className="font-medium">{REQUEST_TYPE_LABELS[type]}</span>
                  {!enabled && (
                    <Badge variant="secondary" className="text-xs">
                      Coming soon
                    </Badge>
                  )}
                </span>
              </label>
            )
          })}
        </CardContent>
      </Card>

      {requestType === 'instrument' && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Instrument request</CardTitle>
            <CardDescription>
              Request a new tradable instrument for Paper Trading. Once approved by an
              administrator, the instrument becomes available automatically. You&apos;ll be
              notified when it&apos;s ready.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-1.5">
              <Label htmlFor="market">Market</Label>
              <Select
                id="market"
                value={form.market_profile_code}
                onChange={e => set('market_profile_code', e.target.value)}
                disabled={!profiles || profiles.length === 0}
              >
                {(profiles ?? []).map(p => (
                  <option key={p.code} value={p.code}>
                    {p.name}
                  </option>
                ))}
              </Select>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-1.5">
                <Label htmlFor="exchange">Exchange</Label>
                <Input
                  id="exchange"
                  placeholder="NSE"
                  value={form.exchange}
                  onChange={e => set('exchange', e.target.value.toUpperCase())}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="symbol">Trading symbol</Label>
                <Input
                  id="symbol"
                  placeholder="WIPRO"
                  value={form.symbol}
                  onChange={e => set('symbol', e.target.value.toUpperCase())}
                />
              </div>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="reason">Reason (optional)</Label>
              <Textarea
                id="reason"
                rows={3}
                placeholder="Needed for paper trading."
                value={form.reason}
                onChange={e => set('reason', e.target.value)}
              />
            </div>
          </CardContent>
        </Card>
      )}

      <div className="flex gap-3">
        <Button type="submit" disabled={submitting || requestType !== 'instrument'}>
          {submitting && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}
          Submit request
        </Button>
        <Button type="button" variant="outline" onClick={() => router.push('/requests')}>
          Cancel
        </Button>
      </div>
    </form>
  )
}
