import { redirect } from 'next/navigation'

export default function SupportNewRedirectPage({
  searchParams,
}: {
  searchParams: Record<string, string | string[] | undefined>
}) {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(searchParams)) {
    if (typeof value === 'string') params.set(key, value)
    else if (Array.isArray(value) && value[0]) params.set(key, value[0])
  }
  const q = params.toString()
  redirect(q ? `/requests/new?${q}` : '/requests/new')
}
