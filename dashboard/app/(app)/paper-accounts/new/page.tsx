import { PaperAccountForm } from '@/components/PaperAccountForm'

export default function NewPaperAccountPage() {
  return (
    <div>
      <h1 className="mb-6 text-2xl font-semibold">Create Paper Account</h1>
      <PaperAccountForm mode="create" />
    </div>
  )
}
