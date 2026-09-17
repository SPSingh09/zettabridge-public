'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import useSWR from 'swr'
import { Tag, Check, X, Loader2 } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { useToast } from '@/components/ToastProvider'
import { useUser } from '@/lib/user-context'
import { adminListSymbolRequests, adminAcceptSymbolRequest, adminRejectSymbolRequest } from '@/lib/api'
import type { SymbolRequest } from '@/lib/types'

// Admin-only banner surfacing pending symbol requests on every dashboard
// page (mirrors ZerodhaReconnectBanner's shape). Accept navigates to the
// admin instruments page pre-filled with this request's symbol/profile/
// exchange — resolution itself happens automatically once that form is
// saved (see AdminCreateInstrument's auto-resolve on the backend).
export function PendingSymbolRequestsBanner() {
  const { user } = useUser()
  const router = useRouter()
  const { toast } = useToast()
  const { data, mutate } = useSWR(
    user.role === 'admin' ? '/v1/admin/symbol-requests?status=pending' : null,
    () => adminListSymbolRequests('pending'),
    { refreshInterval: 60_000 },
  )
  const [acceptingId, setAcceptingId] = useState<string | null>(null)
  const [rejecting, setRejecting] = useState<SymbolRequest | null>(null)
  const [rejectNote, setRejectNote] = useState('')
  const [rejectingBusy, setRejectingBusy] = useState(false)

  if (user.role !== 'admin' || !data || data.length === 0) return null

  async function handleAccept(r: SymbolRequest) {
    setAcceptingId(r.id)
    try {
      await adminAcceptSymbolRequest(r.id)
      await mutate()
      const params = new URLSearchParams({
        prefill_symbol: r.symbol,
        prefill_profile: r.market_profile_code,
        prefill_exchange: r.exchange,
      })
      router.push(`/admin/instruments?${params.toString()}`)
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to accept symbol request', 'error')
    } finally {
      setAcceptingId(null)
    }
  }

  async function handleReject() {
    if (!rejecting) return
    setRejectingBusy(true)
    try {
      await adminRejectSymbolRequest(rejecting.id, rejectNote.trim())
      setRejecting(null)
      setRejectNote('')
      await mutate()
      toast(`Rejected request for "${rejecting.symbol}"`, 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to reject symbol request', 'error')
    } finally {
      setRejectingBusy(false)
    }
  }

  return (
    <>
      {data.map(r => (
        <Alert key={r.id} variant="warning" className="rounded-none border-x-0 border-t-0">
          <Tag className="h-4 w-4" />
          <AlertDescription className="flex items-center justify-between gap-4">
            <span>
              Symbol request: <strong>{r.symbol}</strong> ({r.exchange || '—'} / {r.market_profile_code}) —
              reason: {r.reason}
            </span>
            <div className="flex shrink-0 gap-2">
              <Button
                size="sm"
                disabled={acceptingId === r.id}
                onClick={() => handleAccept(r)}
              >
                {acceptingId === r.id
                  ? <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                  : <Check className="mr-1 h-3 w-3" />}
                Accept
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => { setRejecting(r); setRejectNote('') }}
              >
                <X className="mr-1 h-3 w-3" />
                Reject
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      ))}

      <ConfirmDialog
        open={!!rejecting}
        title="Reject symbol request?"
        description={`The request for "${rejecting?.symbol}" will be marked rejected and the requesting user notified.`}
        confirmLabel={rejectingBusy ? 'Rejecting…' : 'Reject'}
        variant="destructive"
        onConfirm={handleReject}
        onCancel={() => setRejecting(null)}
      >
        <label className="mb-1 block text-sm font-medium">Note to requester (optional)</label>
        <Textarea
          rows={3}
          value={rejectNote}
          onChange={e => setRejectNote(e.target.value)}
          placeholder="e.g. this symbol isn't supported on this market profile"
        />
      </ConfirmDialog>
    </>
  )
}
