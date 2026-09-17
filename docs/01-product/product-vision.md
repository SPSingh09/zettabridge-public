# Product Vision
**Last updated: 2026-07-07**

---

## The Problem

Retail algorithmic traders in India and globally face a hard integration gap. Strategy platforms like TradingView generate high-quality trade signals — but turning those signals into real broker orders requires custom code, manual intervention, or expensive third-party tools that are either too rigid, too complex, or not built for Indian broker APIs.

The problem has three layers:

1. **Execution gap.** TradingView can send a webhook when a signal fires, but it cannot place orders. Every trader must bridge this gap themselves, usually with fragile scripts running on personal machines.

2. **Compliance gap.** Live Indian trading on NSE/BSE requires SEBI-mandated Algo-ID tagging on every API order. Most homebrew scripts and existing tools ignore this requirement, putting traders at regulatory risk.

3. **Operations gap.** A signal reaching a broker while the trader is asleep, the server is down, or the broker session has expired is a missed trade at best and a stuck position at worst. There is no robust, always-on, observable execution layer available to retail traders without significant engineering effort.

---

## Purpose

ZettaBridge is a **multi-tenant algorithmic trading execution bridge** that eliminates the integration, compliance, and operations gaps between strategy platforms and brokers — both **paper (simulated)** and **live**.

It gives retail traders and developers a reliable, SEBI-compliant, always-on execution layer without writing and maintaining their own broker integration code.

---

## Target Users

### 1. Retail algo trader (primary)
- Trades Indian equities (NSE/BSE) via Zerodha, Angel One, or Dhan — or practices on **paper accounts** first
- Uses TradingView for charting and strategy alerts
- Has an active Kite Connect / SmartAPI / Dhan API subscription for live trading
- Wants automated execution but has limited engineering bandwidth
- Pain: building and maintaining a broker integration is outside their core skill

### 2. Paper / strategy developer (primary)
- Tests strategies on simulated paper accounts before risking capital
- Uses the **free** or **paper** plan for unlimited (or quota-limited) paper trades
- Routes webhooks to `paper_account_id` for full pipeline validation without live credentials
- Pain: backtesting in TradingView does not prove webhook-to-execution reliability

### 3. Developer / algo engineer (secondary)
- Builds trading strategies programmatically (Pine Script, Python, custom signals)
- Wants a managed broker API layer so they can focus on strategy logic
- Comfortable with REST APIs and webhooks; wants structured error codes and observability
- Pain: broker APIs differ per broker; maintaining multiple adapters is ongoing overhead

### 4. Platform admin (internal)
- The operator of the ZettaBridge platform
- Needs to manage user plans, suspend/unsuspend accounts, oversee billing, and respond to incidents
- Uses the admin panel and Prometheus/Grafana/Telegram for platform operations

> **Removed persona:** Household / team trader with shared org resources. Multi-user org product removed from API/UI (2026).

---

## Value Proposition

| For | ZettaBridge provides |
|-----|---------------------|
| Retail traders | Zero-code webhook-to-broker execution without running your own server |
| Strategy testers | Paper accounts and paper webhooks for risk-free pipeline validation |
| Indian traders | SEBI Algo-ID compliance built-in; live Indian broker support (Zerodha day-0 on ECS) |
| Developers | Structured REST API, WebSocket trade feed, observable error codes, multi-broker adapters |
| All users | Credentials encrypted at rest, never visible in logs or API responses |

---

## Product Principles

1. **Execution bridge only.** ZettaBridge places orders; it does not advise on what to trade, generate signals, or provide investment recommendations. This boundary must be clear in the product, terms of service, and all user-facing copy.

2. **Compliance first.** SEBI Algo-ID is enforced before any Indian live trading is opened to users. No exceptions. Indian live trading must never be marketed before compliance is confirmed.

3. **Fail closed.** When configuration is missing or ambiguous (no Algo-ID, suspended account, paused webhook), the system rejects rather than guesses. Silent failures that result in unexpected trades are worse than explicit errors.

4. **Credentials never leave the vault.** Broker API keys are encrypted with AES-256-GCM at rest and decrypted only in worker memory at execution time. They are never returned by any GET endpoint, never logged, and never stored on trade rows.

5. **Transparency on failure.** Every rejected order has a structured `error_code`. Users must be able to understand why an order was not placed without contacting support.

6. **Self-serve by default.** Any user should be able to register, verify, create a paper account, create a webhook, and start receiving automated paper trades without platform admin involvement.

7. **Paper before live.** New users start on paper webhooks (free tier includes 1 paper account, 1 paper webhook, 10 trades/month). Live trading requires `pro` or `pro_plus` with explicit live broker credentials (`account_mode: live` only).

8. **Conservative defaults.** New webhooks are active but rate-limited. Dedup returns 409 on duplicate signals. Plan and webhook guards enforce caps before execution.

---

## Plan tiers (current)

| Plan | Best for |
|------|----------|
| **free** | Try paper trading: 1 paper account, 1 paper webhook, 10 paper trades/month |
| **paper** | Serious paper testing: 3 accounts, 5 webhooks, unlimited paper trades, guards + notifications |
| **pro** | Single live broker: 1 live webhook, 1 credential, plus paper capacity |
| **pro_plus** | Multiple live brokers: 5 live webhooks, 3 credentials, multi-product (MIS/CNC/NRML) |

Source of truth: `internal/plan/plan.go`, [STATUS.md](../STATUS.md).

---

## Long-Term Vision

### Near term (beta → GA, 2026)
- Closed beta with paper and live traders on staging ECS (paper + Zerodha adapters)
- Stable, observable, SEBI-compliant live Indian trading
- Self-serve billing when payment gateway becomes available (`paper`, `pro`, `pro_plus`)
- Load-tested to 500 concurrent webhooks with p99 < 100ms

### Medium term (post-GA, 2026–2027)
- Additional Indian broker integrations enabled on ECS (Angel, Dhan; code exists)
- Options and futures order types (currently equities only for Indian brokers)
- Per-user trade alert notifications (email, Telegram) on fill/reject — notifications on paid plans today
- Richer P&L analytics: time-series charts, Sharpe ratio, maximum drawdown
- Platform audit logs with export for tax and compliance use

### Long term (v2+)
- Multi-region deployment for global traders (Singapore, London)
- Position sync: poll broker for live positions and reconcile against trade table
- Automated broker token refresh (OAuth flow per broker)
- Strategy marketplace: shareable webhook templates with one-click fork
- Mobile dashboard (read-only trade monitoring)
- Team/org features (if revisited — removed from v1)

---

## What ZettaBridge Is Not

- Not a broker, SEBI-registered intermediary, or investment adviser
- Not a trading strategy platform or signal generator
- Not a portfolio management system or tax filing tool
- Not a replacement for broker risk controls (stop-loss, circuit breakers are set at the broker level)
- Not a high-frequency trading platform (rate limits and broker API quotas bound execution volume)
- Not a demo-broker simulator for users (`BROKER_MODE=mock` is server ops config only; users use paper accounts)

These boundaries are fixed for the foreseeable future. They should be reflected clearly in all marketing copy, terms of service, and risk disclosure documents before public launch.
