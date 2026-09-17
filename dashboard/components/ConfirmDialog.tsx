'use client'

import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'

interface Props {
  open: boolean
  title: string
  description: string
  confirmLabel?: string
  variant?: 'default' | 'destructive'
  onConfirm: () => void
  onCancel: () => void
  children?: ReactNode
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = 'Confirm',
  variant = 'default',
  onConfirm,
  onCancel,
  children,
}: Props) {
  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/50" onClick={onCancel} />
      <div className="relative z-10 w-full max-w-md rounded-lg bg-background p-6 shadow-xl">
        <h2 className="mb-2 text-lg font-semibold">{title}</h2>
        <p className="mb-6 text-sm text-muted-foreground">{description}</p>
        {children && <div className="mb-6">{children}</div>}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button variant={variant === 'destructive' ? 'destructive' : 'default'} onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  )
}
