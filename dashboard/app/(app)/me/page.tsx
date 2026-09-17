'use client'

import { useState, useEffect } from 'react'
import { User, Shield, Calendar, Send } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { PlanBadge } from '@/components/PlanBadge'
import { useUser } from '@/lib/user-context'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getTelegramSettings, updateTelegramSettings } from '@/lib/api'
import type { TelegramSettings } from '@/lib/types'

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between py-2 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium">{children}</span>
    </div>
  )
}

function TelegramCard() {
  const [settings, setSettings] = useState<TelegramSettings | null>(null)
  const [chatId, setChatId] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  useEffect(() => {
    getTelegramSettings()
      .then(s => { setSettings(s); setChatId(s.chat_id) })
      .catch(() => {})
  }, [])

  async function handleSave() {
    setSaving(true); setError(''); setSuccess('')
    try {
      const updated = await updateTelegramSettings(chatId.trim())
      setSettings(updated)
      setSuccess(updated.connected ? 'Telegram connected.' : 'Telegram disconnected.')
    } catch (e: any) {
      setError(e.message ?? 'Failed to save.')
    } finally {
      setSaving(false)
    }
  }

  async function handleDisconnect() {
    setSaving(true); setError(''); setSuccess('')
    try {
      const updated = await updateTelegramSettings('')
      setSettings(updated)
      setChatId('')
      setSuccess('Telegram disconnected.')
    } catch (e: any) {
      setError(e.message ?? 'Failed to disconnect.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Send className="h-4 w-4 text-muted-foreground" />
          <CardTitle className="text-base">Telegram Alerts</CardTitle>
        </div>
        <CardDescription className="text-xs">
          Receive real-time trade alerts in Telegram for every fill, rejection, or cancellation.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Status</span>
          <span className={settings?.connected ? 'text-green-600 font-medium' : 'text-muted-foreground'}>
            {settings?.connected ? '● Connected' : '○ Not connected'}
          </span>
        </div>

        <div className="space-y-1.5">
          <Label htmlFor="telegram-chat-id" className="text-xs">Chat ID</Label>
          <Input
            id="telegram-chat-id"
            placeholder="e.g. 123456789"
            value={chatId}
            onChange={e => setChatId(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            Start a chat with{' '}
            <a
              href="https://t.me/userinfobot"
              target="_blank"
              rel="noopener noreferrer"
              className="underline"
            >
              @userinfobot
            </a>{' '}
            to get your chat ID, then paste it here.
          </p>
        </div>

        {error && <p className="text-xs text-destructive">{error}</p>}
        {success && <p className="text-xs text-green-600">{success}</p>}

        <div className="flex gap-2">
          <Button size="sm" onClick={handleSave} disabled={saving}>
            {saving ? 'Saving…' : 'Save'}
          </Button>
          {settings?.connected && (
            <Button size="sm" variant="outline" onClick={handleDisconnect} disabled={saving}>
              Disconnect
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

export default function ProfilePage() {
  const { user } = useUser()
  const canUseTelegram = user.plan !== 'free'

  return (
    <div className="max-w-lg">
      <h1 className="mb-6 text-2xl font-semibold">Profile</h1>

      <div className="space-y-4">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <User className="h-4 w-4 text-muted-foreground" />
              <CardTitle className="text-base">Account</CardTitle>
            </div>
          </CardHeader>
          <CardContent className="divide-y">
            <Row label="Email">{user.email}</Row>
            <Row label="Role">{user.role}</Row>
            <Row label="Status">
              <span className={user.status === 'active' ? 'text-green-600' : 'text-destructive'}>
                {user.status}
              </span>
            </Row>
            <Row label="Email verified">
              {user.email_verified ? (
                <span className="text-green-600">Verified</span>
              ) : (
                <span className="text-amber-600">Unverified</span>
              )}
            </Row>
            <Row label="Member since">
              {new Date(user.created_at).toLocaleDateString()}
            </Row>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <Shield className="h-4 w-4 text-muted-foreground" />
              <CardTitle className="text-base">Plan</CardTitle>
            </div>
          </CardHeader>
          <CardContent className="divide-y">
            <Row label="Current plan">
              <PlanBadge plan={user.plan} />
            </Row>
            <Row label="Billing source">{user.billing_source}</Row>
            <Row label="Max paper webhooks">{user.max_paper_webhooks}</Row>
            {user.live_trading_allowed && (
              <Row label="Max live webhooks">{user.max_live_webhooks}</Row>
            )}
            <Row label="Max paper accounts">{user.max_paper_accounts}</Row>
            {user.live_trading_allowed && (
              <Row label="Max live broker accounts">{user.max_broker_creds}</Row>
            )}
            {user.max_paper_trades_per_month != null && (
              <Row label="Paper trades this month">
                {user.paper_trades_used_this_month} / {user.max_paper_trades_per_month}
              </Row>
            )}
          </CardContent>
        </Card>

        {canUseTelegram && <TelegramCard />}

        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <Calendar className="h-4 w-4 text-muted-foreground" />
              <CardTitle className="text-base">User ID</CardTitle>
            </div>
          </CardHeader>
          <CardContent>
            <p className="font-mono text-xs text-muted-foreground break-all">{user.id}</p>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
