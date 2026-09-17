import type { Trade, PaperPosition } from '@/lib/types'

export type WsStatus = 'connecting' | 'connected' | 'reconnecting' | 'disconnected'

type TradeHandler = (trade: Trade) => void
type PaperMarkHandler = (payload: { paper_account_id: string; positions: PaperPosition[] }) => void
type StatusHandler = (status: WsStatus) => void

class TradeSocket {
  private ws: WebSocket | null = null
  private retryDelay = 1_000
  private closed = false
  private timerId: ReturnType<typeof setTimeout> | null = null

  constructor(
    private readonly token: string,
    private readonly onTrade: TradeHandler,
    private readonly onStatus: StatusHandler,
    private readonly onPaperMark?: PaperMarkHandler,
  ) {}

  connect() {
    if (this.closed) return
    const httpBase = process.env.NEXT_PUBLIC_API_URL || window.location.origin
    const wsBase = httpBase.replace(/^http/, 'ws')
    const url = `${wsBase}/v1/ws/trades?token=${encodeURIComponent(this.token)}`
    this.onStatus('connecting')

    const ws = new WebSocket(url)
    this.ws = ws

    ws.onopen = () => {
      this.retryDelay = 1_000
      this.onStatus('connected')
    }

    ws.onmessage = (evt) => {
      try {
        const msg = JSON.parse(evt.data as string) as {
          type: string
          trade?: Trade
          paper_account_id?: string
          positions?: PaperPosition[]
        }
        if (msg.type === 'trade' && msg.trade) {
          this.onTrade(msg.trade)
        } else if (
          msg.type === 'paper_mark' &&
          msg.paper_account_id &&
          msg.positions &&
          this.onPaperMark
        ) {
          this.onPaperMark({ paper_account_id: msg.paper_account_id, positions: msg.positions })
        }
      } catch {
        // ignore malformed frames
      }
    }

    ws.onerror = () => ws.close()

    ws.onclose = () => {
      if (this.closed) {
        this.onStatus('disconnected')
        return
      }
      this.onStatus('reconnecting')
      this.timerId = setTimeout(() => {
        this.retryDelay = Math.min(this.retryDelay * 2, 30_000)
        this.connect()
      }, this.retryDelay)
    }
  }

  disconnect() {
    this.closed = true
    if (this.timerId !== null) clearTimeout(this.timerId)
    this.ws?.close()
  }
}

export function createTradeSocket(
  token: string,
  onTrade: TradeHandler,
  onStatus: StatusHandler,
  onPaperMark?: PaperMarkHandler,
): TradeSocket {
  return new TradeSocket(token, onTrade, onStatus, onPaperMark)
}
