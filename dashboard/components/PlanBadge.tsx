import { Badge } from '@/components/ui/badge'
import type { Plan } from '@/lib/types'

const labels: Record<Plan, string> = {
  free: 'Free',
  paper: 'Paper',
  pro: 'Pro',
  pro_plus: 'Pro Plus',
}

const variants: Record<Plan, 'free' | 'paper' | 'pro' | 'pro_plus'> = {
  free: 'free',
  paper: 'paper',
  pro: 'pro',
  pro_plus: 'pro_plus',
}

export function PlanBadge({ plan }: { plan: Plan }) {
  return <Badge variant={variants[plan]}>{labels[plan]}</Badge>
}
