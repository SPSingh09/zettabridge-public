// API envelope
export type ApiEnvelope<T> = { data: T }

// ── Auth ──────────────────────────────────────────────────────────────────────

export interface LoginResponse {
  token: string
  expires_in: number
  org_id?: string
  org_role?: string
}

export interface RegisterResponse {
  id: string
  email: string
  email_verification_required?: boolean
}

export interface VerifyEmailResponse {
  verified: boolean
  email: string
}

export interface ResendVerificationResponse {
  message: string
}

// ── Telegram settings ─────────────────────────────────────────────────────────

export interface TelegramSettings {
  chat_id: string
  connected: boolean
}

// ── User / Me ─────────────────────────────────────────────────────────────────

export type Plan = 'free' | 'paper' | 'pro' | 'pro_plus'
export type UserRole = 'user' | 'admin'
export type UserStatus = 'active' | 'suspended'
export type BillingSource = 'free' | 'stripe' | 'admin'

export interface GetMeResponse {
  id: string
  email: string
  plan: Plan
  role: UserRole
  status: UserStatus
  email_verified: boolean
  email_verified_at?: string
  created_at: string
  billing_source: BillingSource
  org_id?: string
  billing_plan?: Plan
  // plan limits
  max_webhooks: number
  max_paper_webhooks: number
  max_live_webhooks: number
  max_broker_creds: number
  max_paper_accounts: number
  max_paper_trades_per_month: number | null
  paper_webhooks_used: number
  live_webhooks_used: number
  paper_trades_used_this_month: number
  paper_trade_quota_exceeded: boolean
  live_trading_allowed: boolean
  orders_per_sec: number | null
  orders_per_sec_enforced: boolean
  broker_cred_orders_per_sec: number
  // stripe
  stripe_customer_id?: string
  stripe_subscription_id?: string
  // downgrade state
  has_auto_paused_items: boolean
}

// ── Invites ───────────────────────────────────────────────────────────────────

export interface InvitePreviewResponse {
  org_name: string
  email: string
  role: string
  expires_at: string
}

export interface AcceptInviteResponse {
  token: string
  expires_in: number
  org_id: string
  org_role: string
  user_id: string
  email: string
}

// ── Billing ───────────────────────────────────────────────────────────────────

export interface BillingPlanEntry {
  plan: Plan
  name: string
  price_label?: string
  max_paper_accounts: number
  max_paper_webhooks: number
  max_live_webhooks: number
  max_brokers: number
  max_paper_trades_per_month?: number | null
  max_webhooks: number
  orders_per_sec?: number | null
  live_trading: boolean
  audit_logs: boolean
  multi_product_credentials?: boolean
}

export interface BillingPlansResponse {
  billing_enabled: boolean
  plans: BillingPlanEntry[]
  checkout: {
    success_url: string
    cancel_url: string
    cancel_immediate: boolean
    success_url_placeholder: string
  }
}

// ── Trades ────────────────────────────────────────────────────────────────────

export interface Trade {
  id: string
  webhook_id: string
  webhook_label?: string
  signal: string
  symbol: string
  lot_size: number
  broker_order?: string
  signal_key?: string
  comment?: string
  algo_id?: string
  status: string
  fill_price?: number
  order_type?: string
  product?: string
  error?: string
  error_code?: string
  created_at: string
  publisher_order_expires_at?: string
}

export interface PublisherOrder {
  id: string
  status: string
  expires_at: string
  handoff?: {
    provider: string
    method: string
    action: string
    fields: {
      api_key: string
      data: string
      state: string
    }
  }
}

// ── Webhooks ──────────────────────────────────────────────────────────────────

export interface Webhook {
  id: string
  user_id: string
  org_id?: string
  created_by: string
  token?: string   // only present right after create or rotate-token; absent from list responses
  label: string
  status: 'active' | 'paused'
  auto_paused: boolean
  admin_disabled: boolean
  broker_cred_id?: string
  paper_account_id?: string
  symbol: string
  lot_size: number
  max_risk_pct: number
  sl_points: number
  tp_points: number
  created_at: string
  allowed_actions: string[]
  allowed_symbols: string[]
  max_lot_size: number
  rate_limit_per_sec: number
  dedup_window_sec: number
  timezone: string
  required_comment: string
  default_order_type?: string
}

// ── Credentials ───────────────────────────────────────────────────────────────

export type BrokerType = 'mt5_cloud' | 'zerodha' | 'angel' | 'dhan'
export type AccountMode = 'live'

// Matches the BrokerCredential struct from the list endpoint (uses account_label, not label)
export interface BrokerCredential {
  id: string
  user_id?: string
  org_id?: string
  created_by?: string
  broker_type: BrokerType
  account_label: string
  account_mode: AccountMode | 'demo'
  exchange: string
  product: string
  products?: string[]
  algo_id?: string
  /** @deprecated Order type is configured on webhooks only */
  order_type?: 'MARKET' | 'LIMIT'
  market_protection: number
  execution_mode?: string
  status: 'active' | 'paused'
  auto_paused: boolean
  admin_disabled: boolean
  connected_at?: string
}

// ── Paper Trading Accounts ────────────────────────────────────────────────────

export interface PaperAccount {
  id: string
  user_id: string
  label: string
  market_profile: 'indian_equity' | 'indian_fno' | 'forex' | 'crypto_spot'
  base_currency: string
  starting_balance: number
  cash_balance: number
  exchange: 'NSE' | 'BSE'
  default_product: 'MIS' | 'CNC' | 'NRML'
  status: 'active' | 'paused' | 'closed'
  created_at: string
  updated_at: string
}

export interface PaperPosition {
  id: string
  user_id: string
  paper_account_id: string
  symbol: string
  exchange: string
  product: string
  quantity: number // signed: + long / - short / 0 closed
  avg_entry_price: number
  last_price: number
  unrealized_pnl: number
  created_at: string
  updated_at: string
}

export interface PaperOrderWithPnL {
  id: string
  user_id: string
  paper_account_id: string
  webhook_id?: string
  signal_id?: string
  symbol: string
  exchange: string
  side: 'BUY' | 'SELL' | 'CLOSE'
  order_type: 'MARKET' | 'LIMIT'
  product: string
  quantity: number
  requested_price?: number
  fill_price?: number
  status: 'RECEIVED' | 'VALIDATED' | 'FILLED' | 'REJECTED' | 'CANCELLED'
  reason?: string
  created_at: string
  updated_at: string
  realized_pnl?: number // present only for orders that produced a trade
}

export interface PaperLinkedWebhook {
  id: string
  label: string
  status: string
}

export interface PaperPnLSummary {
  paper_account_id: string
  starting_balance: number
  cash_balance: number
  equity: number
  realized_pnl: number
  unrealized_pnl: number
  total_pnl: number
  trade_count: number
  by_status: Record<string, number>
  notional: number
  fill_rate?: number
  win_rate?: number
  total_fees: number

  // Account Health
  account_health_status: 'healthy' | 'paused' | 'warning'
  buying_power: number
  used_margin: number

  // Today's Performance
  today_pnl?: number
  today_trades: number
  today_win_rate?: number
  today_fees: number
  today_notional: number

  // Trade Distribution
  buy_orders: number
  sell_orders: number
  close_orders: number
  filled: number
  rejected: number
  cancelled: number
  duplicate_suppressed: number

  // Win/Loss distribution
  largest_win?: number
  largest_loss?: number
  average_win?: number
  average_loss?: number
  gross_profit: number
  gross_loss: number
  profit_factor?: number
  expectancy?: number

  // Drawdown
  peak_equity: number
  current_drawdown: number
  max_drawdown: number

  // Exposure & positions
  exposure: number
  exposure_pct?: number
  largest_position: number
  long_positions: number
  short_positions: number
  open_positions: number
  open_orders: number
  open_risk: number
  today_risk: number
  last_market_update_at?: string

  // Diagnostics
  linked_webhooks?: PaperLinkedWebhook[]
  signals_received: number
  signals_accepted: number
  signals_rejected: number
  webhook_fill_rate?: number
  last_webhook_signal_at?: string

  // Market info
  market_profile_name?: string
  trading_session?: 'open' | 'closed'
  market_data_provider?: string
}

export interface PaginatedPaperOrders {
  orders: PaperOrderWithPnL[]
  total: number
}

export interface PaperAccountSnapshot {
  id: string
  user_id: string
  paper_account_id: string
  equity: number
  cash_balance: number
  unrealized_pnl: number
  realized_pnl: number
  created_at: string
}

export interface VerifyResult {
  valid: boolean
  broker_type: string
  account_mode: string
  exchange: string
  product: string
  needs_oauth?: boolean
  message?: string
}

// ── P&L ───────────────────────────────────────────────────────────────────────

export interface PnLSummary {
  webhook_id?: string
  org_id?: string
  trade_count: number
  by_status: Record<string, number>
  notional: number
  priced_trades: number
  fill_rate?: number
  win_rate?: number

  // Signal Processing
  total_signals?: number
  accepted_signals?: number
  rejected_signals?: number
  duplicate_suppressed?: number
  rate_limited_signals?: number
  paused_signals?: number
  queue_full_signals?: number

  // Broker Execution
  submitted_to_broker?: number
  filled?: number
  broker_rejected?: number
  zettabridge_rejected?: number
  cancelled?: number
  pending_confirmation?: number
  avg_latency_ms?: number
  broker_success_rate?: number

  // Orders by side
  buy_orders?: number
  sell_orders?: number
  close_orders?: number

  // Risk & Validation
  rejection_reasons?: Record<string, number>

  // Recent Activity
  last_signal_at?: string
  last_broker_response_at?: string
  last_rejection_reason?: string
  last_rejection_at?: string

  // Broker Health
  broker_type?: string
  execution_mode?: string
  zerodha_connection_status?: 'valid' | 'expires_soon' | 'expired'

  // Paper trading destination context (broker_type/execution_mode are empty
  // for paper webhooks since there's no broker credential).
  paper_account_label?: string
  market_profile_name?: string
}

// ── Admin ─────────────────────────────────────────────────────────────────────

export interface AdminUser {
  id: string
  email: string
  plan: Plan
  role: UserRole
  status: UserStatus
  email_verified_at?: string
  billing_source: BillingSource
  created_at: string
  webhook_count: number
  broker_count: number
}

export interface AdminUserList {
  users: AdminUser[]
  total: number
  page: number
  limit: number
  total_pages: number
}

export interface MarketDataProviderSetting {
  provider: string
  available: string[]
  fyers_configured: boolean
}

export interface FyersStatus {
  app_configured: boolean
  connected: boolean
  token_expired: boolean
  pin_set: boolean
  access_token_expires_at?: string
  refresh_token_expires_at?: string
  connected_at?: string
}

export interface MarketDataIntervalSetting {
  interval_seconds: number
  min_seconds: number
  max_seconds: number
}

export interface TimeWindow {
  start: string
  end: string
}

export type TradingSchedule = Record<string, TimeWindow[]>

export interface MarketProfile {
  id: string
  code: string
  name: string
  base_currency: string
  timezone: string
  trading_days: TradingSchedule | null
  active: boolean
  slippage_bps: number
  fee_bps: number
  created_at: string
  updated_at: string
}

export interface Instrument {
  id: string
  market_profile_code: string
  exchange: string
  symbol: string
  name: string
  instrument_type: 'EQUITY' | 'INDEX' | 'FUTURES' | 'OPTIONS'
  tick_size: number
  lot_size: number
  active: boolean
  created_at: string
  updated_at: string
}

export type SymbolRequestStatus = 'pending' | 'accepted' | 'rejected' | 'resolved'

export interface SymbolRequest {
  id: string
  user_id: string
  market_profile_code: string
  exchange: string
  symbol: string
  reason: string
  status: SymbolRequestStatus
  admin_note: string
  reviewed_by?: string
  reviewed_at?: string
  resolved_instrument_id?: string
  resolved_at?: string
  created_at: string
  updated_at: string
}
