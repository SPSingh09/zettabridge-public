import type { BrokerCredential, PaperAccount } from './types'

// A webhook's execution destination is either a live broker credential or a
// paper trading account. UI selects encode which one as a single prefixed
// string value so both can live in one <select>; these helpers are the only
// place that prefix format is defined.
export const CRED_PREFIX = 'cred:'
export const PAPER_PREFIX = 'paper:'

export function encodeCredDestination(id: string): string {
  return `${CRED_PREFIX}${id}`
}

export function encodePaperDestination(id: string): string {
  return `${PAPER_PREFIX}${id}`
}

export function isPaperDestination(value: string): boolean {
  return value.startsWith(PAPER_PREFIX)
}

export function decodeDestinationId(value: string): string {
  return isPaperDestination(value) ? value.slice(PAPER_PREFIX.length) : value.slice(CRED_PREFIX.length)
}

export interface ExecutionModeLabels {
  publisher: string
  user_api_oauth: string
}

const DEFAULT_EXECUTION_MODE_LABELS: ExecutionModeLabels = {
  publisher: 'Kite Publisher',
  user_api_oauth: 'OAuth',
}

export interface CredentialLabelOptions {
  /** Full ("MetaTrader 5") or short ("MT5") broker display names — callers pick per context. */
  brokerLabels: Record<string, string>
  executionModeLabels?: ExecutionModeLabels
  /** Publisher credentials render as "Publisher · Live/Demo" with no broker name. Default true. */
  collapsePublisherName?: boolean
}

// formatCredentialLabel builds a display label for a broker credential, e.g.
// "Zerodha · OAuth · Live" or "Publisher · Live". The single shared
// implementation avoids the label-formatting rules drifting between the
// webhook form's dropdown and the webhooks list table.
export function formatCredentialLabel(c: BrokerCredential, opts: CredentialLabelOptions): string {
  const account = c.account_mode === 'live' ? 'Live' : 'Demo'
  const collapsePublisher = opts.collapsePublisherName ?? true
  if (collapsePublisher && c.execution_mode === 'publisher') {
    return `Publisher · ${account}`
  }
  const modeLabels = opts.executionModeLabels ?? DEFAULT_EXECUTION_MODE_LABELS
  const name = c.account_label || opts.brokerLabels[c.broker_type] || c.broker_type
  const mode = c.execution_mode ? modeLabels[c.execution_mode as keyof ExecutionModeLabels] : null
  return mode ? `${name} · ${mode} · ${account}` : `${name} · ${account}`
}

// formatPaperAccountLabel builds a display label for a paper trading account.
export function formatPaperAccountLabel(a: PaperAccount | undefined): string {
  return `${a?.label || 'Paper account'} — Paper Trading`
}
