import { useEffect, useState } from 'react'
import { Loader2, Wifi, WifiOff } from 'lucide-react'
import type { WsStatus } from '@/lib/ws'

// A WebSocket reconnect after a brief blip (network hiccup, tab backgrounded,
// a backend redeploy) is normal and usually resolves within a second or two
// — it says nothing about whether the broker itself is connected. Flashing
// an amber "Reconnecting…" for every such blip reads as an alarm next to an
// otherwise-healthy broker status. Only show it once the reconnect has been
// genuinely stuck for a bit; a fast one just keeps showing "connected".
const RECONNECTING_GRACE_MS = 2000

export function WsIndicator({
  status,
  connectedLabel = 'Live',
}: {
  status: WsStatus
  connectedLabel?: string
}) {
  const [genuinelyReconnecting, setGenuinelyReconnecting] = useState(false)

  useEffect(() => {
    if (status !== 'reconnecting') {
      setGenuinelyReconnecting(false)
      return
    }
    const timer = setTimeout(() => setGenuinelyReconnecting(true), RECONNECTING_GRACE_MS)
    return () => clearTimeout(timer)
  }, [status])

  if (status === 'connected' || (status === 'reconnecting' && !genuinelyReconnecting)) {
    return (
      <div className="flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs">
        <Wifi className="h-3.5 w-3.5 text-green-500" />
        <span className="text-green-600">{connectedLabel}</span>
      </div>
    )
  }
  if (status === 'connecting' || genuinelyReconnecting) {
    return (
      <div className="flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs">
        <Loader2 className="h-3.5 w-3.5 animate-spin text-amber-500" />
        <span className="text-amber-600">
          {status === 'connecting' ? 'Connecting…' : 'Reconnecting…'}
        </span>
      </div>
    )
  }
  return (
    <div className="flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs">
      <WifiOff className="h-3.5 w-3.5 text-muted-foreground" />
      <span className="text-muted-foreground">Disconnected</span>
    </div>
  )
}
