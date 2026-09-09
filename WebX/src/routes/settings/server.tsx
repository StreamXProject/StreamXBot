import React, { useEffect, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { Server, Plug, Check, X, Trash2, FlaskConical, Copy, Activity } from 'lucide-react'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { TextField, Button, Switch, Chip } from '@/components/md3'
import { useSettingsStore } from '@/stores/settingsStore'
import { useUiStore, toast } from '@/stores/uiStore'
import { checkHealth, type HealthResult } from '@/api/health'
import { fetchSetupStatus, type SetupStatus } from '@/api/auth'
import { API_ENDPOINTS } from '@/api/endpoints'
import { normalizeBaseUrl } from '@/api/client'
import { useQueryClient } from '@tanstack/react-query'
import { cn } from '@/lib/cn'

export const Route = createFileRoute('/settings/server')({
  component: ServerSettings,
})

const ENDPOINT_GROUPS: Array<[string, Array<[string, string]>]> = [
  ['Catalog', [['Browse', API_ENDPOINTS.BROWSE], ['Search', API_ENDPOINTS.SEARCH], ['Shuffle', API_ENDPOINTS.TRACKS_SHUFFLE], ['Albums', API_ENDPOINTS.ALBUMS], ['Artists', API_ENDPOINTS.ARTISTS], ['Curated mixes', API_ENDPOINTS.PLAYLISTS_AVAILABLE]]],
  ['Tracks', [['Stream', '/tracks/{id}/stream?token=…'], ['Warm cache', '/tracks/{id}/warm'], ['Lyrics', '/tracks/{id}/lyrics?format=json'], ['Download', '/tracks/{id}/download']]],
  ['Account', [['Favourites', API_ENDPOINTS.ME_FAVOURITES], ['Playlists', API_ENDPOINTS.ME_PLAYLISTS], ['History', API_ENDPOINTS.ME_HISTORY], ['Top played', API_ENDPOINTS.ME_TOP_PLAYED], ['Profile', API_ENDPOINTS.AUTH_ME]]],
  ['Auth', [['Login', API_ENDPOINTS.AUTH_LOGIN], ['Server password', API_ENDPOINTS.AUTH_PASSWORD], ['Setup status', API_ENDPOINTS.AUTH_SETUP_STATUS]]],
]

function ServerSettings() {
  const apiBaseUrl = useSettingsStore((s) => s.apiBaseUrl)
  const knownServers = useSettingsStore((s) => s.knownServers)
  const setApiBaseUrl = useSettingsStore((s) => s.setApiBaseUrl)
  const removeKnownServer = useSettingsStore((s) => s.removeKnownServer)
  const demoMode = useSettingsStore((s) => s.demoMode)
  const setSetting = useSettingsStore((s) => s.set)
  const serverStatus = useUiStore((s) => s.serverStatus)
  const setServerStatus = useUiStore((s) => s.setServerStatus)
  const qc = useQueryClient()

  const [draft, setDraft] = useState(apiBaseUrl)
  const [testing, setTesting] = useState(false)
  const [result, setResult] = useState<{ health: HealthResult; setup?: SetupStatus | null } | null>(null)

  useEffect(() => setDraft(apiBaseUrl), [apiBaseUrl])

  const test = async (url = draft) => {
    setTesting(true)
    const base = normalizeBaseUrl(url)
    const health = await checkHealth(base)
    let setup: SetupStatus | null = null
    if (health.ok) {
      try {
        setup = await fetchSetupStatus(base)
      } catch {
        setup = null
      }
    }
    setResult({ health, setup })
    setTesting(false)
    return health.ok
  }

  const save = async () => {
    const base = normalizeBaseUrl(draft)
    if (!base) return
    const ok = await test(base)
    setApiBaseUrl(base)
    setServerStatus(ok ? 'online' : 'offline')
    qc.clear()
    toast(ok ? 'Server saved and reachable' : 'Server saved — not reachable right now', { variant: ok ? 'default' : 'error' })
  }

  const dirty = normalizeBaseUrl(draft) !== normalizeBaseUrl(apiBaseUrl)

  return (
    <SettingsPage title="Server & endpoints" description="WebX talks to a StreamX API server. Point it at your deployment — a local daemon, a LAN box, or a public host.">
      <SettingsSection title="Connection">
        <div className="p-4 space-y-3">
          <TextField
            label="Server address"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && void save()}
            placeholder="http://192.168.1.20:8000"
            leading={<Server />}
            supporting="Include the scheme. No trailing slash needed."
            spellCheck={false}
            autoCapitalize="off"
            className="font-mono"
          />
          <div className="flex flex-wrap items-center gap-2">
            <Button onClick={() => void save()} disabled={!draft.trim() || (!dirty && !result)} loading={testing} icon={<Plug />}>
              {dirty ? 'Save & connect' : 'Reconnect'}
            </Button>
            <Button variant="tonal" onClick={() => void test()} loading={testing} icon={<Activity />}>Test</Button>
            {result && (
              <span className={cn('inline-flex items-center gap-1.5 type-label-lg', result.health.ok ? 'text-tertiary' : 'text-error')}>
                {result.health.ok ? <Check className="size-4" /> : <X className="size-4" />}
                {result.health.ok ? `Online · ${result.health.latencyMs} ms${result.health.version ? ` · v${result.health.version}` : ''}` : result.health.detail}
              </span>
            )}
          </div>
          {result?.setup && (
            <p className="type-body-sm text-on-surface-variant">
              {result.setup.needs_setup ? 'This server has no owner password yet — you can set one from the sign-in screen.' : 'Server is configured.'}
            </p>
          )}
        </div>
        <SettingRow
          label="Current status"
          description={serverStatus === 'online' ? 'Reachable' : serverStatus === 'offline' ? 'Unreachable — pages will show an error state' : 'Checking…'}
          control={<span className={cn('size-3 rounded-full', serverStatus === 'online' ? 'bg-tertiary' : serverStatus === 'offline' ? 'bg-error' : 'bg-outline')} />}
        />
      </SettingsSection>

      {knownServers.length > 1 && (
        <SettingsSection title="Recent servers">
          {knownServers.map((u) => (
            <SettingRow
              key={u}
              label={<span className="font-mono text-[13px]">{u}</span>}
              description={u === apiBaseUrl ? 'Active' : undefined}
              onClick={() => setDraft(u)}
              control={
                <button onClick={(e) => { e.stopPropagation(); removeKnownServer(u) }} aria-label="Forget" className="state-layer size-8 rounded-full inline-flex items-center justify-center text-on-surface-variant"><Trash2 className="size-4" /></button>
              }
            />
          ))}
        </SettingsSection>
      )}

      <SettingsSection title="Offline">
        <SettingRow
          icon={<FlaskConical />}
          label="Demo mode"
          description="Use a bundled sample catalog instead of the server. Handy for trying themes when you're offline."
          control={<Switch checked={demoMode} onChange={(v) => { setSetting('demoMode', v); qc.clear() }} label="Demo mode" />}
        />
      </SettingsSection>

      <section>
        <div className="mb-2 px-1">
          <h2 className="type-title-sm text-primary">Endpoints in use</h2>
          <p className="type-body-sm text-on-surface-variant">Relative to the server address. Authenticated calls send the token as a Bearer header; streams use <code className="font-mono">?token=</code>.</p>
        </div>
        <div className="rounded-lg bg-surface-low p-4 space-y-4">
          {ENDPOINT_GROUPS.map(([g, eps]) => (
            <div key={g}>
              <p className="type-label-md text-on-surface-variant mb-1.5">{g}</p>
              <div className="flex flex-wrap gap-1.5">
                {eps.map(([label, path]) => (
                  <Chip
                    key={path}
                    label={<span><span className="text-on-surface-variant">{label}</span> <span className="font-mono text-[12px]">{path}</span></span>}
                    icon={<Copy className="size-3.5" />}
                    onClick={async () => {
                      await navigator.clipboard.writeText(`${normalizeBaseUrl(apiBaseUrl)}${path}`).catch(() => {})
                      toast('Copied URL')
                    }}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      </section>
    </SettingsPage>
  )
}
