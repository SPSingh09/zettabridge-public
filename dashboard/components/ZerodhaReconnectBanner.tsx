'use client'

import useSWR from 'swr'
import { AlertTriangle, RefreshCw, Loader2 } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { apiFetch } from '@/lib/api'
import { useState } from 'react'
import { useToast } from '@/components/ToastProvider'

export interface ZerodhaCredStatus {
  credential_id: string
  account_label: string
  status: 'valid' | 'expires_soon' | 'expired'
  needs_reconnect: boolean
}

export const ZERODHA_STATUS_KEY = '/v1/credentials/zerodha/status'

export function useZerodhaStatus() {
  return useSWR<ZerodhaCredStatus[]>(
    ZERODHA_STATUS_KEY,
    (url: string) => apiFetch<ZerodhaCredStatus[]>(url),
    { refreshInterval: 60_000, revalidateOnMount: true },
  )
}

export function ZerodhaReconnectBanner() {
  const { data } = useZerodhaStatus()
  const { data: connectInfo } = useSWR(
    '/v1/credentials/zerodha/connect-info',
    (url: string) => apiFetch<{ oauth_enabled: boolean }>(url),
  )
  const [connecting, setConnecting] = useState<string | null>(null)
  const { toast } = useToast()

  // Admin has turned OAuth off — prompting to reconnect a disabled feature is confusing.
  if (connectInfo?.oauth_enabled === false) return null

  if (!data || data.length === 0) return null

  const needsReconnect = data.filter(c => c.needs_reconnect)
  if (needsReconnect.length === 0) return null

  async function handleReconnect(credId: string) {
    setConnecting(credId)
    try {
      const { redirect_url } = await apiFetch<{ redirect_url: string }>(
        `/v1/credentials/zerodha/connect?id=${credId}`,
      )
      window.location.href = redirect_url
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to initiate Zerodha reconnect', 'error')
      setConnecting(null)
    }
  }

  return (
    <>
      {needsReconnect.map(cred => (
        <Alert
          key={cred.credential_id}
          variant={cred.status === 'expired' ? 'destructive' : 'warning'}
          className="rounded-none border-x-0 border-t-0"
        >
          <AlertTriangle className="h-4 w-4" />
          <AlertDescription className="flex items-center justify-between">
            <span>
              {cred.status === 'expired'
                ? `Zerodha "${cred.account_label}" connection has expired — live orders will fail.`
                : `Zerodha "${cred.account_label}" token expires at 6:00 AM IST — reconnect after 6 AM.`}
            </span>
            {cred.status === 'expired' && (
              <Button
                size="sm"
                variant="destructive"
                className="ml-4 shrink-0"
                disabled={connecting === cred.credential_id}
                onClick={() => handleReconnect(cred.credential_id)}
              >
                {connecting === cred.credential_id
                  ? <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                  : <RefreshCw className="mr-1 h-3 w-3" />}
                Reconnect
              </Button>
            )}
          </AlertDescription>
        </Alert>
      ))}
    </>
  )
}
