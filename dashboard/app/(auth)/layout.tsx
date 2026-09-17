import Image from 'next/image'

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-muted/30 px-4">
      <div className="mb-8 flex items-center gap-3">
        <Image src="/logo.png" alt="ZettaBridge logo" width={40} height={40} className="h-10 w-auto" priority />
        <span className="text-2xl font-bold tracking-tight">
          Zetta<span className="text-primary">Bridge</span>
        </span>
      </div>
      {children}
    </div>
  )
}
