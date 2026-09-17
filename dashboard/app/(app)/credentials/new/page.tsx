import { CredentialForm } from '@/components/CredentialForm'

export default function NewCredentialPage() {
  return (
    <div>
      <h1 className="mb-6 text-2xl font-semibold">Add Zerodha Connection</h1>
      <CredentialForm mode="create" />
    </div>
  )
}
