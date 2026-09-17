'use client'

import { createContext, useCallback, useContext, useState } from 'react'
import { AlertCircle, CheckCircle, X } from 'lucide-react'

type ToastVariant = 'default' | 'success' | 'error'

interface Toast {
  id: number
  message: string
  variant: ToastVariant
}

interface ToastCtx {
  toast: (message: string, variant?: ToastVariant) => void
}

const ToastContext = createContext<ToastCtx>({ toast: () => {} })

let seq = 0

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])

  const dismiss = useCallback((id: number) => {
    setToasts(prev => prev.filter(t => t.id !== id))
  }, [])

  const toast = useCallback(
    (message: string, variant: ToastVariant = 'default') => {
      const id = ++seq
      setToasts(prev => [...prev, { id, message, variant }])
      setTimeout(() => dismiss(id), 5_000)
    },
    [dismiss],
  )

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}
      <div
        aria-live="polite"
        aria-label="Notifications"
        className="fixed bottom-4 right-4 z-50 flex flex-col gap-2"
      >
        {toasts.map(t => (
          <div
            key={t.id}
            className="flex max-w-sm items-start gap-3 rounded-lg border bg-background px-4 py-3 shadow-lg text-sm"
          >
            {t.variant === 'success' && (
              <CheckCircle className="mt-0.5 h-4 w-4 flex-shrink-0 text-green-600" />
            )}
            {t.variant === 'error' && (
              <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0 text-destructive" />
            )}
            <span className="flex-1">{t.message}</span>
            <button
              aria-label="Dismiss"
              onClick={() => dismiss(t.id)}
              className="text-muted-foreground hover:text-foreground"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast() {
  return useContext(ToastContext)
}
