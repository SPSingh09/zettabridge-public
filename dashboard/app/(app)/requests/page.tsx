'use client'

import useSWR from 'swr'
import Link from 'next/link'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { RequestStatusBadge } from '@/components/RequestStatusBadge'
import { listMySymbolRequests, listActiveMarketProfiles } from '@/lib/api'
import {
  formatRelativeDate,
  requestDisplayId,
  requestTitle,
  REQUEST_TYPE_LABELS,
} from '@/lib/requestDisplay'

function TableSkeleton() {
  return (
    <div className="rounded-md border divide-y">
      {[1, 2].map(i => (
        <div key={i} className="flex items-center gap-4 px-4 py-3.5">
          <Skeleton className="h-4 w-16" />
          <Skeleton className="h-4 w-20" />
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-5 w-20 rounded-full" />
        </div>
      ))}
    </div>
  )
}

export default function RequestsPage() {
  const { data: requests, isLoading } = useSWR('/v1/symbol-requests', listMySymbolRequests)
  const { data: profiles } = useSWR('/v1/market-profiles', listActiveMarketProfiles)
  const list = requests ?? []

  const profileName = (code: string) =>
    profiles?.find(p => p.code === code)?.name ?? code

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">Requests</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Request a new instrument for Paper Trading. Once approved by an administrator, the
            instrument becomes available for simulated trading inside ZettaBridge.
          </p>
        </div>
        <Button asChild>
          <Link href="/requests/new">
            <Plus className="h-4 w-4" /> New instrument request
          </Link>
        </Button>
      </div>

      {!isLoading && list.length > 0 && (
        <p className="mb-4 text-xs text-muted-foreground">
          <span className="font-medium text-foreground">Approved</span>: available for paper
          trading · <span className="font-medium text-foreground">Rejected</span>: not approved
          by admin · <span className="font-medium text-foreground">Closed</span>: request
          resolved without adding a new instrument
        </p>
      )}

      {isLoading ? (
        <TableSkeleton />
      ) : list.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="py-16 text-center">
            <p className="text-sm text-muted-foreground">No requests yet.</p>
            <p className="mt-1 text-sm text-muted-foreground">
              Need a symbol that isn&apos;t available?
            </p>
            <Button asChild className="mt-4">
              <Link href="/requests/new">
                <Plus className="h-4 w-4" /> New instrument request
              </Link>
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="rounded-md border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">
                <th className="px-4 py-3">ID</th>
                <th className="px-4 py-3">Type</th>
                <th className="px-4 py-3">Title</th>
                <th className="px-4 py-3">Market</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Created</th>
                <th className="px-4 py-3">Updated</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {list.map(r => (
                <tr key={r.id} className="hover:bg-muted/30">
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                    {requestDisplayId(r.id)}
                  </td>
                  <td className="px-4 py-3">{REQUEST_TYPE_LABELS.instrument}</td>
                  <td className="px-4 py-3 font-medium">
                    {requestTitle(r.exchange, r.symbol)}
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {profileName(r.market_profile_code)}
                  </td>
                  <td className="px-4 py-3">
                    <RequestStatusBadge status={r.status} />
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {formatRelativeDate(r.created_at)}
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {formatRelativeDate(r.updated_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
