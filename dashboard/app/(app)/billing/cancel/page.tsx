import Link from 'next/link'
import { XCircle } from 'lucide-react'
import { Card, CardContent, CardFooter } from '@/components/ui/card'

export default function BillingCancelPage() {
  return (
    <Card className="mx-auto mt-16 w-full max-w-sm text-center">
      <CardContent className="pt-6">
        <XCircle className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
        <h2 className="mb-2 text-xl font-semibold">Checkout cancelled</h2>
        <p className="text-sm text-muted-foreground">
          No changes were made to your subscription.
        </p>
      </CardContent>
      <CardFooter className="justify-center">
        <Link href="/billing" className="font-medium text-foreground underline-offset-4 hover:underline">
          ← Back to billing
        </Link>
      </CardFooter>
    </Card>
  )
}
