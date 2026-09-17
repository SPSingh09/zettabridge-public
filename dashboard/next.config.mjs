/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  poweredByHeader: false,
  async rewrites() {
    // INTERNAL_API_URL is baked at build time (ARG in Dockerfile) to the Go server's
    // internal Docker network address. Falls back to localhost for native npm run dev.
    // In production with Caddy, Caddy intercepts /v1/* before it reaches Next.js,
    // so these rewrites only activate in local Docker Compose or native dev.
    const apiUrl = process.env.INTERNAL_API_URL || 'http://localhost:8080'
    return [
      {
        source: '/v1/:path*',
        destination: `${apiUrl}/v1/:path*`,
      },
    ]
  },
}

export default nextConfig
