'use client'

import { cn } from '@/lib/utils'

interface Props {
  active: boolean
  onToggle: () => void
  disabled?: boolean
  label?: string
}

export function ActiveSwitch({ active, onToggle, disabled, label }: Props) {
  return (
    <div className="flex items-center gap-2">
      <button
        type="button"
        role="switch"
        aria-checked={active}
        aria-label={label ?? (active ? 'Active' : 'Inactive')}
        disabled={disabled}
        onClick={onToggle}
        className={cn(
          'relative inline-flex h-5 w-9 shrink-0 rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
          active ? 'bg-emerald-600' : 'bg-muted-foreground/30',
          disabled && 'cursor-not-allowed opacity-50',
        )}
      >
        <span
          className={cn(
            'pointer-events-none block h-4 w-4 rounded-full bg-white shadow transition-transform',
            active ? 'translate-x-4' : 'translate-x-0',
          )}
        />
      </button>
      <span
        className={cn(
          'text-xs font-medium',
          active ? 'text-emerald-700 dark:text-emerald-400' : 'text-muted-foreground',
        )}
      >
        {active ? 'Active' : 'Inactive'}
      </span>
    </div>
  )
}
