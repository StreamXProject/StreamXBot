import React, { useEffect, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import {
  Radio,
  MessageSquare,
  ExternalLink,
  Link2,
  Unlink,
  RefreshCw,
  KeyRound,
  CheckCircle2,
  AlertCircle,
  Globe,
  Monitor,
  Eye,
  EyeOff,
  HelpCircle,
  ShieldCheck,
  QrCode,
} from 'lucide-react'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { Switch, Button, TextField, Slider, SegmentedButton, IconButton } from '@/components/md3'
import { DiscordQrModal } from '@/components/settings/DiscordQrModal'
import { useSettingsStore, type DiscordMode } from '@/stores/settingsStore'
import { toast } from '@/stores/uiStore'
import { beginAuth, finishAuth, disconnect as lastfmDisconnect, scrobbleStatus, onScrobbleStatus } from '@/services/lastfm'
import { getDiscordInfo, onDiscordStatus, reconnectDiscord, type DiscordPresenceInfo } from '@/services/discordPresence'
import { saveIntegrationsToServer } from '@/services/integrationsSync'
import { cn } from '@/lib/cn'

export const Route = createFileRoute('/settings/integrations')({
  component: IntegrationsSettings,
})

function useDiscordPresenceInfo(): DiscordPresenceInfo {
  const [info, setInfo] = useState<DiscordPresenceInfo>(getDiscordInfo())
  useEffect(() => onDiscordStatus(setInfo), [])
  return info
}

function useScrobbleStatus() {
  const [, tick] = useState(0)
  useEffect(() => onScrobbleStatus(() => tick((n) => n + 1)), [])
  return scrobbleStatus
}

function IntegrationsSettings() {
  const s = useSettingsStore()
  const set = s.set
  const discord = useDiscordPresenceInfo()
  const scrobbles = useScrobbleStatus()
  const [pendingToken, setPendingToken] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const connected = Boolean(s.lastfmSessionKey)
  const hasKeys = Boolean(s.lastfmApiKey.trim() && s.lastfmApiSecret.trim())

  const [tokenDraft, setTokenDraft] = useState(s.discordUserToken)
  const [showToken, setShowToken] = useState(false)
  const [showTokenHelp, setShowTokenHelp] = useState(false)
  const [qrModalOpen, setQrModalOpen] = useState(false)

  useEffect(() => {
    setTokenDraft(s.discordUserToken)
  }, [s.discordUserToken])

  const connect = async () => {
    setBusy(true)
    try {
      const { token, url } = await beginAuth()
      setPendingToken(token)
      window.open(url, '_blank', 'noopener')
      toast('Approve WebX on Last.fm, then come back and press “I’ve approved”')
    } catch (e) {
      toast(`Last.fm: ${(e as Error).message}`, { variant: 'error' })
    } finally {
      setBusy(false)
    }
  }
  const complete = async () => {
    if (!pendingToken) return
    setBusy(true)
    try {
      const session = await finishAuth(pendingToken)
      setPendingToken(null)
      set('lastfmEnabled', true)
      void saveIntegrationsToServer({
        lastfm: {
          enabled: true,
          session_key: session.key,
          username: session.name,
          api_key: s.lastfmApiKey,
          api_secret: s.lastfmApiSecret,
        },
      })
      toast(`Connected to Last.fm as ${session.name}`)
    } catch (e) {
      toast(`Last.fm: ${(e as Error).message}`, { variant: 'error' })
    } finally {
      setBusy(false)
    }
  }

  return (
    <SettingsPage title="Integrations" description="Synced to your account across devices">
      <SettingsSection title="Last.fm scrobbling" description="Sent at the threshold below · 4 min max">
        <SettingRow
          icon={<Radio />}
          label="Scrobble to Last.fm"
          description={connected ? `Signed in as ${s.lastfmUsername}` : 'Connect an account to enable'}
          control={
            <Switch
              checked={s.lastfmEnabled && connected}
              disabled={!connected}
              onChange={(v) => {
                set('lastfmEnabled', v)
                void saveIntegrationsToServer({ lastfm: { enabled: v } })
              }}
              label="Scrobble to Last.fm"
            />
          }
        />
        <SettingRow
          icon={<KeyRound />}
          label="API credentials"
          description={
            <>
              API account:{' '}
              <a href="https://www.last.fm/api/account/create" target="_blank" rel="noreferrer" className="text-primary hover:underline inline-flex items-center gap-1">
                last.fm/api <ExternalLink className="size-3" />
              </a>{' '}
              · paste key and secret
            </>
          }
          stacked
          control={
            <div className="grid sm:grid-cols-2 gap-3 w-full">
              <TextField
                label="API key"
                value={s.lastfmApiKey}
                onChange={(e) => {
                  const val = e.target.value.trim()
                  set('lastfmApiKey', val)
                  void saveIntegrationsToServer({ lastfm: { api_key: val } })
                }}
                autoComplete="off"
                spellCheck={false}
                className="font-mono"
                disabled={connected}
              />
              <TextField
                label="Shared secret"
                type="password"
                value={s.lastfmApiSecret}
                onChange={(e) => {
                  const val = e.target.value.trim()
                  set('lastfmApiSecret', val)
                  void saveIntegrationsToServer({ lastfm: { api_secret: val } })
                }}
                autoComplete="off"
                spellCheck={false}
                className="font-mono"
                disabled={connected}
              />
            </div>
          }
        />
        <SettingRow
          icon={connected ? <CheckCircle2 className="text-tertiary" /> : <Link2 />}
          label={connected ? 'Account connected' : pendingToken ? 'Waiting for approval' : 'Connect account'}
          description={connected ? 'Revoke on Last.fm to disconnect' : pendingToken ? 'Approve in the Last.fm tab, then confirm' : 'Authorise this browser on Last.fm'}
          control={
            connected ? (
              <Button
                variant="outlined"
                icon={<Unlink />}
                onClick={() => {
                  lastfmDisconnect()
                  void saveIntegrationsToServer({
                    lastfm: {
                      enabled: false,
                      session_key: '',
                      username: '',
                    },
                  })
                  toast('Disconnected from Last.fm')
                }}
              >
                Disconnect
              </Button>
            ) : pendingToken ? (
              <div className="flex gap-2">
                <Button variant="text" onClick={() => setPendingToken(null)}>Cancel</Button>
                <Button loading={busy} onClick={() => void complete()}>I’ve approved</Button>
              </div>
            ) : (
              <Button loading={busy} disabled={!hasKeys} icon={<Link2 />} onClick={() => void connect()}>Connect</Button>
            )
          }
        />
        <SettingRow
          label="Scrobble threshold"
          description={`${Math.round(s.lastfmScrobbleAt * 100)}% of the track`}
          stacked
          control={
            <Slider
              value={Math.round(s.lastfmScrobbleAt * 100)}
              min={10}
              max={95}
              step={5}
              onChange={(v) => {
                const fraction = v / 100
                set('lastfmScrobbleAt', fraction)
                void saveIntegrationsToServer({ lastfm: { scrobble_at: fraction } })
              }}
              aria-label="Scrobble threshold"
              className="w-full"
            />
          }
        />
        <SettingRow
          label="Send “now playing”"
          description="Current track on your profile"
          control={
            <Switch
              checked={s.lastfmNowPlaying}
              onChange={(v) => {
                set('lastfmNowPlaying', v)
                void saveIntegrationsToServer({ lastfm: { now_playing: v } })
              }}
              label="Now playing"
            />
          }
        />
        {(scrobbles.count > 0 || scrobbles.lastError) && (
          <SettingRow
            icon={scrobbles.lastError ? <AlertCircle className="text-error" /> : <CheckCircle2 className="text-tertiary" />}
            label={scrobbles.lastError ? 'Last error' : `${scrobbles.count} scrobbled this session`}
            description={scrobbles.lastError ?? scrobbles.lastScrobbled}
          />
        )}
      </SettingsSection>

      <SettingsSection
        title="Discord Rich Presence"
        description="Track, artwork and timer on your profile"
      >
        <SettingRow
          icon={<MessageSquare />}
          label="Show current song on Discord"
          description={
            <span className="inline-flex items-center gap-1.5">
              <span
                className={cn(
                  'size-2 rounded-full',
                  !s.discordEnabled
                    ? 'bg-outline-variant'
                    : discord.status === 'connected'
                      ? 'bg-tertiary'
                      : discord.status === 'connecting'
                        ? 'bg-outline animate-pulse'
                        : 'bg-error'
                )}
              />
              {!s.discordEnabled
                ? 'Off'
                : discord.status === 'connected'
                  ? discord.username
                    ? `Connected as ${discord.username}`
                    : 'Connected'
                  : discord.status === 'connecting'
                    ? 'Connecting…'
                    : discord.errorMessage || 'Connection error'}
            </span>
          }
          control={
            <div className="flex items-center gap-2">
              {s.discordEnabled && (
                <Button
                  variant="tonal"
                  size="sm"
                  icon={<RefreshCw className="size-3.5" />}
                  onClick={() => {
                    reconnectDiscord()
                    toast('Reconnecting to Discord…')
                  }}
                >
                  Reconnect
                </Button>
              )}
              <Switch
                checked={s.discordEnabled}
                onChange={(v) => {
                  set('discordEnabled', v)
                  void saveIntegrationsToServer({ discord: { enabled: v } })
                }}
                label="Discord Rich Presence"
              />
            </div>
          }
        />

        <SettingRow
          icon={<KeyRound />}
          label="Discord Account"
          description="QR code or user token · synced to your account"
          stacked
          control={
            <div className="flex flex-col gap-3 w-full">
              <div className="flex flex-wrap items-center justify-between gap-3 p-3.5 bg-secondary-container/30 rounded-xs border border-outline-variant/60">
                <div className="flex items-center gap-3">
                  <div className="size-10 rounded-full bg-secondary-container text-on-secondary-container flex items-center justify-center shrink-0">
                    <QrCode className="size-5" />
                  </div>
                  <div className="text-left">
                    <div className="type-title-sm text-on-surface font-semibold">Fast QR Code Login</div>
                    <div className="type-body-sm text-on-surface-variant">Log in automatically by scanning a QR code with the Discord mobile app</div>
                  </div>
                </div>
                <Button
                  variant="filled"
                  icon={<QrCode className="size-4" />}
                  onClick={() => setQrModalOpen(true)}
                >
                  Log in with Discord QR
                </Button>
              </div>

              <div className="text-[12px] font-medium text-on-surface-variant pt-1">
                Or paste Discord User Token manually:
              </div>

              <div className="flex gap-2 w-full">
                <TextField
                  type={showToken ? 'text' : 'password'}
                  value={tokenDraft}
                  onChange={(e) => setTokenDraft(e.target.value)}
                  onBlur={() => {
                    if (tokenDraft.trim() !== s.discordUserToken) {
                      const t = tokenDraft.trim()
                      set('discordUserToken', t)
                      set('discordEnabled', Boolean(t))
                      void saveIntegrationsToServer({
                        discord: {
                          token: t,
                          enabled: Boolean(t),
                        },
                      })
                      toast('Discord token saved')
                    }
                  }}
                  placeholder="Paste user token (e.g. MT... or OT...)"
                  containerClassName="flex-1"
                  className="font-mono text-[13px]"
                  spellCheck={false}
                  autoCapitalize="off"
                  autoComplete="off"
                  trailing={
                    <IconButton
                      size="sm"
                      label={showToken ? 'Hide token' : 'Show token'}
                      onClick={() => setShowToken(!showToken)}
                    >
                      {showToken ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                    </IconButton>
                  }
                />
                {tokenDraft.trim() !== s.discordUserToken && (
                  <Button
                    variant="filled"
                    onClick={() => {
                      const t = tokenDraft.trim()
                      set('discordUserToken', t)
                      set('discordEnabled', Boolean(t))
                      void saveIntegrationsToServer({
                        discord: {
                          token: t,
                          enabled: Boolean(t),
                        },
                      })
                      toast('Discord token saved')
                    }}
                    className="h-14"
                  >
                    Save
                  </Button>
                )}
                {Boolean(s.discordUserToken) && tokenDraft.trim() === s.discordUserToken && (
                  <Button
                    variant="outlined"
                    icon={<Unlink className="size-4" />}
                    onClick={() => {
                      set('discordUserToken', '')
                      set('discordEnabled', false)
                      setTokenDraft('')
                      void saveIntegrationsToServer({
                        discord: {
                          token: '',
                          enabled: false,
                        },
                      })
                      toast('Discord disconnected')
                    }}
                    className="h-14"
                  >
                    Disconnect
                  </Button>
                )}
              </div>
              <div className="flex items-center justify-between text-[12px] text-on-surface-variant px-1">
                <span className="inline-flex items-center gap-1 text-tertiary">
                  <ShieldCheck className="size-3.5" /> Synced to your account profile in MongoDB
                </span>
                <button
                  type="button"
                  onClick={() => setShowTokenHelp(!showTokenHelp)}
                  className="text-primary hover:underline inline-flex items-center gap-1"
                >
                  <HelpCircle className="size-3.5" /> How do I get my Discord token manually?
                </button>
              </div>

              {showTokenHelp && (
                <div className="rounded-xs bg-surface-highest/80 border border-outline-variant/60 p-3.5 text-[12px] leading-relaxed text-on-surface flex flex-col gap-2">
                  <p className="font-semibold text-primary">How to obtain your Discord Token manually:</p>
                  <ol className="list-decimal list-inside space-y-1 text-on-surface-variant">
                    <li>Open <a href="https://discord.com/app" target="_blank" rel="noreferrer" className="text-primary underline">discord.com/app</a> in Chrome, Firefox, or Brave.</li>
                    <li>Press <kbd className="px-1 py-0.5 rounded bg-surface-variant font-mono text-[11px]">Ctrl+Shift+I</kbd> (or <kbd className="px-1 py-0.5 rounded bg-surface-variant font-mono text-[11px]">Cmd+Option+I</kbd> on Mac) to open DevTools.</li>
                    <li>Switch to the <strong>Console</strong> tab, paste this one-liner, and press Enter:</li>
                  </ol>
                  <code className="block rounded bg-surface-lowest p-2 font-mono text-[11px] select-all break-all text-on-surface border border-outline-variant/40">
                    {`(webpackChunkdiscord_app.push([[''],{},e=>{m=[];for(let c in e.c)m.push(e.c[c])}]),m).find(m=>m?.exports?.default?.getToken!==void 0).exports.default.getToken()`}
                  </code>
                  <p className="text-[11px] text-on-surface-variant">
                    Or open the <strong>Network</strong> tab in DevTools, filter by <code className="font-mono bg-surface-lowest px-1 rounded">/api</code>, click any Discord request, and copy the value of the <code className="font-mono bg-surface-lowest px-1 rounded">authorization</code> header under Request Headers.
                  </p>
                </div>
              )}
            </div>
          }
        />

        <SettingRow
          label="Include album artwork"
          description="Cover as large image"
          control={
            <Switch
              checked={s.discordShowArtwork}
              onChange={(v) => {
                set('discordShowArtwork', v)
                void saveIntegrationsToServer({ discord: { show_artwork: v } })
              }}
              label="Include artwork"
            />
          }
        />
      </SettingsSection>
      <DiscordQrModal open={qrModalOpen} onClose={() => setQrModalOpen(false)} />
    </SettingsPage>
  )
}
