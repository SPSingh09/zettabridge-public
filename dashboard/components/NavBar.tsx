'use client'

import Link from 'next/link'
import Image from 'next/image'
import { usePathname, useRouter } from 'next/navigation'
import { LogOut } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PlanBadge } from '@/components/PlanBadge'
import { useUser } from '@/lib/user-context'
import { apiFetch } from '@/lib/api'
import { clearToken } from '@/lib/auth'

const baseNavLinks = [
  { href: '/webhooks', label: 'Webhooks' },
  { href: '/credentials', label: 'Broker Accounts' },
  { href: '/paper-accounts', label: 'Paper Accounts' },
  { href: '/requests', label: 'Requests' },
  { href: '/billing', label: 'Billing' },
  { href: 'https://zettabridge.net/docs', label: 'Help', external: true },
]

export function NavBar() {
  const { user } = useUser()

  const navLinks = [
    ...baseNavLinks,
    ...(user.role === 'admin' ? [{ href: '/admin', label: 'Admin' }] : []),
  ]
  const pathname = usePathname()
  const router = useRouter()

  async function handleLogout() {
    try {
      await apiFetch('/v1/auth/logout', { method: 'POST' })
    } catch {
      // ignore errors — clear token regardless
    }
    clearToken()
    router.push('/login')
  }

  return (
    <header className="sticky top-0 z-50 w-full border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container flex h-14 items-center gap-6">
        <Link href="/webhooks" className="flex items-center">
          <Image src="/logo.png" alt="ZettaBridge" width={140} height={32} className="h-8 w-auto" priority />
        </Link>

        <nav className="flex items-center gap-1">
          {navLinks.map(({ href, label, external }) =>
            external ? (
              <a
                key={href}
                href={href}
                target="_blank"
                rel="noopener noreferrer"
                className="rounded-md px-3 py-1.5 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground text-muted-foreground"
              >
                {label}
              </a>
            ) : (
              <Link
                key={href}
                href={href}
                className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground ${
                  pathname.startsWith(href)
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground'
                }`}
              >
                {label}
              </Link>
            )
          )}
        </nav>

        <div className="ml-auto flex items-center gap-3">
          <Link href="/me" className="text-sm text-muted-foreground hover:text-foreground">
            {user.email}
          </Link>
          <PlanBadge plan={user.plan} />
          <Button variant="ghost" size="icon" onClick={handleLogout} title="Logout">
            <LogOut className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </header>
  )
}
