import { RequestForm } from '@/components/RequestForm'

export default function NewRequestPage() {
  return (
    <div>
      <h1 className="mb-2 text-2xl font-semibold">New request</h1>
      <p className="mb-6 text-sm text-muted-foreground">
        Tell us what you need — we&apos;ll review and follow up by email when there&apos;s an
        update.
      </p>
      <RequestForm />
    </div>
  )
}
