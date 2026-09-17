'use client'

import { useState, useEffect } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import useSWR from 'swr'
import Link from 'next/link'
import { ArrowLeft, Pencil } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ActiveSwitch } from '@/components/ActiveSwitch'
import { InstrumentTypeBadge } from '@/components/InstrumentTypeBadge'
import { useToast } from '@/components/ToastProvider'
import { useUser } from '@/lib/user-context'
import {
  formatExecutionModel,
  formatTickSize,
  formatTradingHours,
  parseExchangeSymbol,
  profileCurrency,
} from '@/lib/marketDisplay'
import {
  adminListMarketProfiles,
  adminUpdateMarketProfile,
  adminListInstruments,
  adminCreateInstrument,
  adminUpdateInstrument,
} from '@/lib/api'
import type { MarketProfile, Instrument } from '@/lib/types'

const INSTRUMENT_TYPES = ['EQUITY', 'INDEX', 'FUTURES', 'OPTIONS'] as const

export default function AdminInstrumentsPage() {
  const { user } = useUser()
  const router = useRouter()
  const searchParams = useSearchParams()
  const { toast } = useToast()

  useEffect(() => {
    if (user.role !== 'admin') router.replace('/webhooks')
  }, [user.role, router])

  const prefillProfile = searchParams.get('prefill_profile') ?? ''
  const prefillSymbol = searchParams.get('prefill_symbol') ?? ''
  const prefillExchange = searchParams.get('prefill_exchange') ?? ''

  const { data: profiles, mutate: mutateProfiles } = useSWR(
    '/v1/admin/market-profiles',
    adminListMarketProfiles,
  )
  const [selectedProfile, setSelectedProfile] = useState<string>('')

  useEffect(() => {
    if (!profiles || profiles.length === 0 || selectedProfile) return
    const wanted =
      prefillProfile && profiles.some(p => p.code === prefillProfile)
        ? prefillProfile
        : profiles[0].code
    setSelectedProfile(wanted)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [profiles, selectedProfile])

  const { data: instruments, mutate: mutateInstruments } = useSWR(
    selectedProfile ? `/v1/admin/instruments?market_profile=${selectedProfile}` : null,
    () => adminListInstruments(selectedProfile),
  )

  const selectedCurrency = profileCurrency(profiles, selectedProfile)

  const [editingProfileCode, setEditingProfileCode] = useState<string | null>(null)
  const [profileEdit, setProfileEdit] = useState({
    base_currency: '',
    timezone: '',
    slippage_bps: '',
    fee_bps: '',
  })
  const [savingProfile, setSavingProfile] = useState(false)

  const initialSymbolInput =
    prefillSymbol && prefillExchange
      ? `${prefillExchange.toUpperCase()}:${prefillSymbol.toUpperCase()}`
      : prefillSymbol
        ? `NSE:${prefillSymbol.toUpperCase()}`
        : ''

  const [newInstr, setNewInstr] = useState({
    symbolInput: initialSymbolInput,
    name: '',
    instrument_type: 'EQUITY',
    tick_size: '0.05',
    lot_size: '1',
  })
  const [savingInstr, setSavingInstr] = useState(false)

  const [editingInstrId, setEditingInstrId] = useState<string | null>(null)
  const [instrEdit, setInstrEdit] = useState({ tick_size: '', lot_size: '' })
  const [savingInstrId, setSavingInstrId] = useState<string | null>(null)

  if (user.role !== 'admin') return null

  function startEditProfile(p: MarketProfile) {
    setEditingProfileCode(p.code)
    setProfileEdit({
      base_currency: p.base_currency,
      timezone: p.timezone,
      slippage_bps: String(p.slippage_bps),
      fee_bps: String(p.fee_bps),
    })
  }

  async function handleSaveProfile(p: MarketProfile) {
    if (!profileEdit.base_currency.trim() || !profileEdit.timezone.trim()) return
    const slippageBps = parseFloat(profileEdit.slippage_bps)
    const feeBps = parseFloat(profileEdit.fee_bps)
    if (isNaN(slippageBps) || slippageBps < 0 || isNaN(feeBps) || feeBps < 0) {
      toast('Slippage and fee must be zero or greater', 'error')
      return
    }
    setSavingProfile(true)
    try {
      await adminUpdateMarketProfile(p.code, {
        base_currency: profileEdit.base_currency.trim(),
        timezone: profileEdit.timezone.trim(),
        slippage_bps: slippageBps,
        fee_bps: feeBps,
      })
      setEditingProfileCode(null)
      mutateProfiles()
      toast(`${p.name} updated`, 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to update market', 'error')
    } finally {
      setSavingProfile(false)
    }
  }

  async function handleToggleProfileActive(p: MarketProfile) {
    try {
      await adminUpdateMarketProfile(p.code, { active: !p.active })
      mutateProfiles()
      toast(`${p.name} ${!p.active ? 'activated' : 'deactivated'}`, 'success')
    } catch {
      toast('Failed to update market', 'error')
    }
  }

  async function handleCreateInstrument(e: React.FormEvent) {
    e.preventDefault()
    if (!selectedProfile || !newInstr.name.trim()) return

    const parsed = parseExchangeSymbol(newInstr.symbolInput)
    if (!parsed) {
      toast('Enter symbol as EXCHANGE:SYMBOL (e.g. NSE:WIPRO)', 'error')
      return
    }

    setSavingInstr(true)
    try {
      await adminCreateInstrument({
        market_profile_code: selectedProfile,
        exchange: parsed.exchange,
        symbol: parsed.symbol,
        name: newInstr.name.trim(),
        instrument_type: newInstr.instrument_type,
        tick_size: parseFloat(newInstr.tick_size) || 0.05,
        lot_size: parseFloat(newInstr.lot_size) || 1,
      })
      setNewInstr({
        symbolInput: '',
        name: '',
        instrument_type: 'EQUITY',
        tick_size: '0.05',
        lot_size: '1',
      })
      mutateInstruments()
      toast(`${parsed.exchange}:${parsed.symbol} added`, 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to create instrument', 'error')
    } finally {
      setSavingInstr(false)
    }
  }

  function startEditInstrument(i: Instrument) {
    setEditingInstrId(i.id)
    setInstrEdit({ tick_size: String(i.tick_size), lot_size: String(i.lot_size) })
  }

  async function handleSaveInstrument(i: Instrument) {
    const tickSize = parseFloat(instrEdit.tick_size)
    const lotSize = parseFloat(instrEdit.lot_size)
    if (!tickSize || tickSize <= 0 || !lotSize || lotSize <= 0) {
      toast('Tick size and lot size must be greater than zero', 'error')
      return
    }
    setSavingInstrId(i.id)
    try {
      await adminUpdateInstrument(i.id, { tick_size: tickSize, lot_size: lotSize })
      setEditingInstrId(null)
      mutateInstruments()
      toast(`${i.exchange}:${i.symbol} updated`, 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to update instrument', 'error')
    } finally {
      setSavingInstrId(null)
    }
  }

  async function handleToggleInstrumentActive(i: Instrument) {
    try {
      await adminUpdateInstrument(i.id, { active: !i.active })
      mutateInstruments()
      toast(`${i.exchange}:${i.symbol} ${!i.active ? 'activated' : 'deactivated'}`, 'success')
    } catch {
      toast('Failed to update instrument', 'error')
    }
  }

  return (
    <div className="max-w-4xl space-y-6">
      <div className="flex items-center gap-3">
        <Link href="/admin" className="text-muted-foreground hover:text-foreground">
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1 className="text-2xl font-semibold">Markets &amp; Instruments</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Market metadata used by paper trading, order validation, and risk checks.
          </p>
        </div>
      </div>

      {/* ── Markets ───────────────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Markets</CardTitle>
          <CardDescription>
            Define market characteristics such as currency, timezone, trading hours, and
            paper-trading execution settings. System-defined markets cannot be renamed.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <p className="mb-4 text-sm text-muted-foreground">
            Execution costs apply only during Paper Trading and simulate slippage and fees.{' '}
            <span className="text-foreground">0 = disabled.</span>
          </p>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-xs uppercase text-muted-foreground">
                  <th className="px-2 py-2">Name</th>
                  <th className="px-2 py-2">Currency</th>
                  <th className="px-2 py-2">Timezone</th>
                  <th className="px-2 py-2">Trading hours</th>
                  <th className="px-2 py-2">Execution model</th>
                  <th className="px-2 py-2">Status</th>
                  <th className="px-2 py-2 w-12"></th>
                </tr>
              </thead>
              <tbody>
                {(profiles ?? []).map(p => {
                  const isEditing = editingProfileCode === p.code
                  return (
                    <tr key={p.code} className="border-b last:border-0">
                      <td className="px-2 py-2 font-medium">{p.name}</td>
                      <td className="px-2 py-2">
                        {isEditing ? (
                          <Input
                            className="w-20"
                            value={profileEdit.base_currency}
                            onChange={e =>
                              setProfileEdit(v => ({ ...v, base_currency: e.target.value }))
                            }
                          />
                        ) : (
                          p.base_currency
                        )}
                      </td>
                      <td className="px-2 py-2 text-muted-foreground">
                        {isEditing ? (
                          <Input
                            className="w-40"
                            value={profileEdit.timezone}
                            onChange={e =>
                              setProfileEdit(v => ({ ...v, timezone: e.target.value }))
                            }
                          />
                        ) : (
                          p.timezone
                        )}
                      </td>
                      <td className="px-2 py-2 text-muted-foreground whitespace-nowrap">
                        {formatTradingHours(p.trading_days)}
                      </td>
                      <td className="px-2 py-2">
                        {isEditing ? (
                          <div className="flex gap-1">
                            <Input
                              type="number"
                              step="0.01"
                              min="0"
                              className="w-20"
                              title="Slippage (bps)"
                              placeholder="Slip"
                              value={profileEdit.slippage_bps}
                              onChange={e =>
                                setProfileEdit(v => ({ ...v, slippage_bps: e.target.value }))
                              }
                            />
                            <Input
                              type="number"
                              step="0.01"
                              min="0"
                              className="w-20"
                              title="Fee (bps)"
                              placeholder="Fee"
                              value={profileEdit.fee_bps}
                              onChange={e =>
                                setProfileEdit(v => ({ ...v, fee_bps: e.target.value }))
                              }
                            />
                          </div>
                        ) : (
                          <span className="text-muted-foreground">
                            {formatExecutionModel(p.slippage_bps, p.fee_bps)}
                          </span>
                        )}
                      </td>
                      <td className="px-2 py-2">
                        <ActiveSwitch
                          active={p.active}
                          onToggle={() => handleToggleProfileActive(p)}
                          disabled={isEditing}
                          label={`${p.name} active`}
                        />
                      </td>
                      <td className="px-2 py-2">
                        {isEditing ? (
                          <div className="flex gap-1">
                            <Button
                              size="sm"
                              onClick={() => handleSaveProfile(p)}
                              disabled={savingProfile}
                            >
                              Save
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => setEditingProfileCode(null)}
                              disabled={savingProfile}
                            >
                              Cancel
                            </Button>
                          </div>
                        ) : (
                          <Button
                            size="sm"
                            variant="ghost"
                            className="h-8 w-8 p-0"
                            onClick={() => startEditProfile(p)}
                            aria-label={`Edit ${p.name}`}
                          >
                            <Pencil className="h-4 w-4" />
                          </Button>
                        )}
                      </td>
                    </tr>
                  )
                })}
                {(profiles ?? []).length === 0 && (
                  <tr>
                    <td colSpan={7} className="px-2 py-6 text-center text-muted-foreground">
                      No markets configured yet.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {/* ── Instrument catalog ──────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <CardTitle className="text-lg">Instrument catalog</CardTitle>
              <CardDescription className="mt-1.5">
                Defines tradable symbols for Paper Trading. Orders are validated against this
                catalog — tick size and lot size are enforced automatically.
              </CardDescription>
            </div>
            <Select
              className="w-48 shrink-0"
              value={selectedProfile}
              onChange={e => setSelectedProfile(e.target.value)}
              aria-label="Market"
            >
              {(profiles ?? []).map(p => (
                <option key={p.code} value={p.code}>
                  {p.name}
                </option>
              ))}
            </Select>
          </div>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-xs uppercase text-muted-foreground">
                  <th className="px-2 py-2">Symbol</th>
                  <th className="px-2 py-2">Name</th>
                  <th className="px-2 py-2">Type</th>
                  <th className="px-2 py-2">Tick size</th>
                  <th className="px-2 py-2">Lot size</th>
                  <th className="px-2 py-2">Status</th>
                  <th className="px-2 py-2 w-12"></th>
                </tr>
              </thead>
              <tbody>
                {(instruments ?? []).map(i => {
                  const isEditing = editingInstrId === i.id
                  return (
                    <tr key={i.id} className="border-b last:border-0">
                      <td className="px-2 py-2 font-mono text-xs">
                        {i.exchange}:{i.symbol}
                      </td>
                      <td className="px-2 py-2">{i.name}</td>
                      <td className="px-2 py-2">
                        <InstrumentTypeBadge type={i.instrument_type} />
                      </td>
                      <td className="px-2 py-2">
                        {isEditing ? (
                          <Input
                            type="number"
                            step="0.01"
                            className="w-24"
                            value={instrEdit.tick_size}
                            onChange={e =>
                              setInstrEdit(v => ({ ...v, tick_size: e.target.value }))
                            }
                          />
                        ) : (
                          formatTickSize(i.tick_size, selectedCurrency)
                        )}
                      </td>
                      <td className="px-2 py-2">{i.lot_size}</td>
                      <td className="px-2 py-2">
                        <ActiveSwitch
                          active={i.active}
                          onToggle={() => handleToggleInstrumentActive(i)}
                          disabled={isEditing}
                          label={`${i.symbol} active`}
                        />
                      </td>
                      <td className="px-2 py-2">
                        {isEditing ? (
                          <div className="flex gap-1">
                            <Button
                              size="sm"
                              onClick={() => handleSaveInstrument(i)}
                              disabled={savingInstrId === i.id}
                            >
                              Save
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => setEditingInstrId(null)}
                              disabled={savingInstrId === i.id}
                            >
                              Cancel
                            </Button>
                          </div>
                        ) : (
                          <Button
                            size="sm"
                            variant="ghost"
                            className="h-8 w-8 p-0"
                            onClick={() => startEditInstrument(i)}
                            aria-label={`Edit ${i.exchange}:${i.symbol}`}
                          >
                            <Pencil className="h-4 w-4" />
                          </Button>
                        )}
                      </td>
                    </tr>
                  )
                })}
                {(instruments ?? []).length === 0 && (
                  <tr>
                    <td colSpan={7} className="px-2 py-6 text-center text-muted-foreground">
                      No instruments for this market yet.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {/* ── Add instrument ────────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Add instrument</CardTitle>
          <CardDescription>
            Add a symbol to the catalog for the selected market.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            id="add-instrument-form"
            onSubmit={handleCreateInstrument}
            className="flex flex-wrap items-end gap-3"
          >
            <div className="grid gap-1 min-w-[140px] flex-1">
              <label className="text-xs font-medium text-muted-foreground">Symbol</label>
              <Input
                placeholder="NSE:WIPRO"
                value={newInstr.symbolInput}
                onChange={e =>
                  setNewInstr(v => ({ ...v, symbolInput: e.target.value.toUpperCase() }))
                }
              />
              <p className="text-xs text-muted-foreground">EXCHANGE:SYMBOL format</p>
            </div>
            <div className="grid gap-1 min-w-[160px] flex-1">
              <label className="text-xs font-medium text-muted-foreground">Name</label>
              <Input
                placeholder="Wipro Ltd"
                value={newInstr.name}
                onChange={e => setNewInstr(v => ({ ...v, name: e.target.value }))}
              />
            </div>
            <div className="grid gap-1">
              <label className="text-xs font-medium text-muted-foreground">Type</label>
              <Select
                className="w-28"
                value={newInstr.instrument_type}
                onChange={e => setNewInstr(v => ({ ...v, instrument_type: e.target.value }))}
              >
                {INSTRUMENT_TYPES.map(t => (
                  <option key={t} value={t}>
                    {t === 'FUTURES' ? 'FUTURE' : t === 'OPTIONS' ? 'OPTION' : t}
                  </option>
                ))}
              </Select>
            </div>
            <div className="grid gap-1">
              <label className="text-xs font-medium text-muted-foreground">Tick size</label>
              <Input
                type="number"
                step="0.01"
                className="w-24"
                value={newInstr.tick_size}
                onChange={e => setNewInstr(v => ({ ...v, tick_size: e.target.value }))}
              />
            </div>
            <div className="grid gap-1">
              <label className="text-xs font-medium text-muted-foreground">Lot size</label>
              <Input
                type="number"
                step="1"
                className="w-20"
                value={newInstr.lot_size}
                onChange={e => setNewInstr(v => ({ ...v, lot_size: e.target.value }))}
              />
            </div>
          </form>
        </CardContent>
        <CardFooter>
          <Button
            type="submit"
            form="add-instrument-form"
            disabled={savingInstr || !selectedProfile || !newInstr.symbolInput.trim()}
          >
            {savingInstr ? 'Adding…' : 'Add instrument'}
          </Button>
        </CardFooter>
      </Card>
    </div>
  )
}
