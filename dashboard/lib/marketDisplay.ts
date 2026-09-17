import type { Instrument, MarketProfile, TradingSchedule } from '@/lib/types'

const CURRENCY_SYMBOLS: Record<string, string> = {
  INR: '₹',
  USD: '$',
  EUR: '€',
  GBP: '£',
}

const DAY_ORDER = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'] as const
const DAY_LABELS: Record<string, string> = {
  mon: 'Mon',
  tue: 'Tue',
  wed: 'Wed',
  thu: 'Thu',
  fri: 'Fri',
  sat: 'Sat',
  sun: 'Sun',
}

/** Parses TradingView-style EXCHANGE:SYMBOL input. */
export function parseExchangeSymbol(raw: string): { exchange: string; symbol: string } | null {
  const trimmed = raw.trim().toUpperCase()
  const idx = trimmed.indexOf(':')
  if (idx <= 0 || idx >= trimmed.length - 1) return null
  return {
    exchange: trimmed.slice(0, idx),
    symbol: trimmed.slice(idx + 1),
  }
}

export function formatTickSize(value: number, currency: string): string {
  const sym = CURRENCY_SYMBOLS[currency.toUpperCase()]
  if (sym) return `${sym}${value}`
  return String(value)
}

export function formatTradingHours(schedule: TradingSchedule | null | undefined): string {
  if (!schedule || Object.keys(schedule).length === 0) return '24/7'

  const entries = DAY_ORDER.flatMap(day => {
    const windows = schedule[day]
    if (!windows?.length) return []
    return [{ day, windows }]
  })
  if (entries.length === 0) return '24/7'

  const firstWindow = entries[0].windows[0]
  const sameWindow = entries.every(
    e =>
      e.windows.length === 1 &&
      e.windows[0].start === firstWindow.start &&
      e.windows[0].end === firstWindow.end,
  )

  if (sameWindow && entries.length >= 5) {
    const days = entries.map(e => e.day)
    const isWeekdays =
      days.length === 5 &&
      days.every((d, i) => d === DAY_ORDER[i])
    if (isWeekdays) {
      return `Mon–Fri ${firstWindow.start}–${firstWindow.end}`
    }
    const first = DAY_LABELS[days[0]] ?? days[0]
    const last = DAY_LABELS[days[days.length - 1]] ?? days[days.length - 1]
    return `${first}–${last} ${firstWindow.start}–${firstWindow.end}`
  }

  return entries
    .map(
      ({ day, windows }) =>
        `${DAY_LABELS[day] ?? day} ${windows.map(w => `${w.start}–${w.end}`).join(', ')}`,
    )
    .join(' · ')
}

export function formatExecutionModel(slippageBps: number, feeBps: number): string {
  if (slippageBps === 0 && feeBps === 0) return 'None'
  const parts: string[] = []
  if (slippageBps > 0) parts.push(`${slippageBps} bps slippage`)
  if (feeBps > 0) parts.push(`${feeBps} bps fee`)
  return parts.join(' · ')
}

export type InstrumentTypeVariant = 'equity' | 'index' | 'future' | 'option'

export function instrumentTypeVariant(type: Instrument['instrument_type']): InstrumentTypeVariant {
  switch (type) {
    case 'INDEX':
      return 'index'
    case 'FUTURES':
      return 'future'
    case 'OPTIONS':
      return 'option'
    default:
      return 'equity'
  }
}

export function instrumentTypeLabel(type: Instrument['instrument_type']): string {
  switch (type) {
    case 'FUTURES':
      return 'FUTURE'
    case 'OPTIONS':
      return 'OPTION'
    default:
      return type
  }
}

export function profileCurrency(profiles: MarketProfile[] | undefined, code: string): string {
  return profiles?.find(p => p.code === code)?.base_currency ?? 'INR'
}

export function profileName(profiles: MarketProfile[] | undefined, code: string): string {
  return profiles?.find(p => p.code === code)?.name ?? code
}

/** Formats a monetary amount with locale grouping and currency symbol when known. */
export function formatCurrencyAmount(amount: number, currency: string): string {
  const sym = CURRENCY_SYMBOLS[currency.toUpperCase()]
  const formatted = amount.toLocaleString('en-IN', {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  })
  if (sym) return `${sym}${formatted}`
  return `${currency} ${formatted}`
}

/** Parses a display balance string (may include ₹, commas) into a number. */
export function parseCurrencyInput(raw: string): number {
  const cleaned = raw.replace(/[^\d.]/g, '')
  return parseFloat(cleaned) || 0
}

/** Live format while typing — keeps digits/decimals, adds grouping and symbol. */
export function formatCurrencyInput(raw: string, currency: string): string {
  const cleaned = raw.replace(/[^\d.]/g, '')
  if (!cleaned) return ''
  const parts = cleaned.split('.')
  const whole = parts[0].replace(/^0+(?=\d)/, '') || '0'
  const grouped = Number(whole).toLocaleString('en-IN')
  const sym = CURRENCY_SYMBOLS[currency.toUpperCase()] ?? ''
  if (parts.length > 1) {
    return `${sym}${grouped}.${parts[1].slice(0, 2)}`
  }
  return `${sym}${grouped}`
}
