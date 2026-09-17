import { Badge } from '@/components/ui/badge'

export function ActionBadge({ action }: { action: string }) {
  const variant = action === 'BUY' ? 'default' : action === 'SELL' ? 'destructive' : 'secondary'
  return <Badge variant={variant}>{action}</Badge>
}
