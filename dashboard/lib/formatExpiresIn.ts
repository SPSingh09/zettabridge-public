/** Formats an ISO expiry timestamp as human-readable remaining time. */
export function formatExpiresIn(iso: string | undefined): string | null {
  if (!iso) return null
  const ms = new Date(iso).getTime() - Date.now()
  if (Number.isNaN(ms)) return null
  if (ms <= 0) return 'Expired'

  const totalMinutes = Math.floor(ms / (1000 * 60))
  const days = Math.floor(totalMinutes / (60 * 24))
  const hours = Math.floor((totalMinutes % (60 * 24)) / 60)
  const minutes = totalMinutes % 60

  const parts: string[] = []
  if (days > 0) parts.push(`${days} day${days !== 1 ? 's' : ''}`)
  if (hours > 0) parts.push(`${hours} hour${hours !== 1 ? 's' : ''}`)
  if (days === 0 && hours === 0 && minutes > 0) {
    parts.push(`${minutes} minute${minutes !== 1 ? 's' : ''}`)
  }
  if (parts.length === 0) return 'Less than a minute'
  return parts.join(' ')
}
