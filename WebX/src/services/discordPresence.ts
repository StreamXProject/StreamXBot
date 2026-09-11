/**
 * Discord Rich Presence service.
 *
 * Supports two modes:
 *  1. 'gateway' (Metrolist / Kizzy style):
 *     Connects directly to Discord's official Gateway WebSocket (`wss://gateway.discord.gg`).
 *     Works in any browser, mobile PWA, or serverless deployment without the Discord desktop client.
 *  2. 'daemon' (Desktop IPC):
 *     Talks to a local helper daemon (`tools/discord_presence_daemon.py`) at `ws://127.0.0.1:6472`,
 *     which bridges to the local Discord desktop client.
 */
import { useSettingsStore } from '@/stores/settingsStore'
import { audioEngine } from '@/audio/AudioEngine'

export type DiscordStatus = 'off' | 'connecting' | 'connected' | 'error'

export interface DiscordPresenceInfo {
  status: DiscordStatus
  errorMessage?: string | null
  username?: string | null
  mode: 'gateway' | 'daemon'
}

let currentStatus: DiscordStatus = 'off'
let currentError: string | null = null
let currentUsername: string | null = null

const listeners = new Set<(info: DiscordPresenceInfo) => void>()

export function getDiscordStatus(): DiscordStatus {
  return currentStatus
}

export function getDiscordInfo(): DiscordPresenceInfo {
  const mode = useSettingsStore.getState().discordMode || 'gateway'
  return {
    status: currentStatus,
    errorMessage: currentError,
    username: currentUsername,
    mode,
  }
}

export function onDiscordStatus(fn: (info: DiscordPresenceInfo) => void): () => void {
  listeners.add(fn)
  fn(getDiscordInfo())
  return () => listeners.delete(fn)
}

function updateState(status: DiscordStatus, error: string | null = null, username: string | null = currentUsername) {
  currentStatus = status
  currentError = error
  currentUsername = username
  const info = getDiscordInfo()
  listeners.forEach((fn) => fn(info))
}

let ws: WebSocket | null = null
let reconnectTimer: number | null = null
let heartbeatTimer: number | null = null
let backoff = 1000
let started = false
let lastPayloadKey = ''
let lastSentAt = 0
let lastSeq: number | null = null
let isGatewayReady = false

function clearTimers() {
  if (reconnectTimer !== null) {
    window.clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (heartbeatTimer !== null) {
    window.clearInterval(heartbeatTimer)
    heartbeatTimer = null
  }
}

function scheduleReconnect() {
  if (reconnectTimer !== null) return
  reconnectTimer = window.setTimeout(() => {
    reconnectTimer = null
    backoff = Math.min(30_000, backoff * 2)
    connect()
  }, backoff)
}

function disconnect() {
  clearTimers()
  isGatewayReady = false
  if (ws) {
    try {
      const mode = useSettingsStore.getState().discordMode
      if (ws.readyState === WebSocket.OPEN) {
        if (mode === 'gateway') {
          ws.send(JSON.stringify({ op: 3, d: { since: 0, activities: [], status: 'online', afk: false } }))
        } else {
          ws.send(JSON.stringify({ type: 'clear' }))
        }
      }
      ws.close()
    } catch {
      /* ignore */
    }
    ws = null
  }
  updateState('off', null, null)
}

export function formatDiscordAsset(url: string | null | undefined): string | null {
  if (!url || typeof url !== 'string') return null
  const clean = url.trim()
  if (!clean) return null

  // 1. Spotify CDN: https://i.scdn.co/image/<id> or https://image-cdn-ak.spotifycdn.com/image/<id>
  const spotifyMatch = clean.match(/(?:scdn\.co|spotifycdn\.com)\/image\/([a-zA-Z0-9_-]+)/i)
  if (spotifyMatch) {
    return `spotify:${spotifyMatch[1]}`
  }

  // 2. YouTube thumbnail: https://i.ytimg.com/vi/<id>/... or https://img.youtube.com/vi/<id>/...
  const ytMatch = clean.match(/(?:ytimg\.com|youtube\.com)\/vi\/([a-zA-Z0-9_-]+)/i)
  if (ytMatch) {
    return `youtube:${ytMatch[1]}`
  }

  // 3. Already prefixed (spotify:, youtube:, twitch:, mp:)
  if (clean.startsWith('spotify:') || clean.startsWith('youtube:') || clean.startsWith('twitch:') || clean.startsWith('mp:')) {
    return clean
  }

  // 4. Discord CDN or Media proxy
  if (/^https?:\/\/(?:cdn\.discordapp\.com|media\.discordapp\.net)\//i.test(clean)) {
    return clean
      .replace(/^https?:\/\/cdn\.discordapp\.com\//i, 'mp:')
      .replace(/^https?:\/\/media\.discordapp\.net\//i, 'mp:')
  }

  // 5. Fallback: if external URL, use it directly (if public HTTP/HTTPS)
  return clean.startsWith('http')
    ? clean
    : typeof window !== 'undefined'
      ? `${window.location.origin}${clean.startsWith('/') ? '' : '/'}${clean}`
      : clean
}

const assetCache = new Map<string, string>()
const pendingResolutions = new Map<string, Promise<string | null>>()

export async function resolveDiscordAsset(
  url: string | null | undefined,
  appId: string,
  token: string
): Promise<string | null> {
  if (!url || typeof url !== 'string') return null
  const clean = url.trim()
  if (!clean) return null

  // If already formatted (mp:, spotify:, youtube:, twitch:)
  if (
    clean.startsWith('mp:') ||
    clean.startsWith('spotify:') ||
    clean.startsWith('youtube:') ||
    clean.startsWith('twitch:')
  ) {
    return clean
  }

  // Already cached
  const cached = assetCache.get(clean)
  if (cached) return cached

  // Coalesce duplicate requests
  const existing = pendingResolutions.get(clean)
  if (existing) return existing

  const p = (async () => {
    let fullUrl = clean
    if (!fullUrl.startsWith('http')) {
      if (typeof window !== 'undefined' && window.location.origin) {
        fullUrl = `${window.location.origin}${fullUrl.startsWith('/') ? '' : '/'}${fullUrl}`
      }
    }

    const effectiveAppId = appId || '1547543416143876167'

    try {
      // 1. Direct request to Discord's official external-assets endpoint
      let res: Response | null = null
      if (token) {
        try {
          res = await fetch(
            `https://discord.com/api/v9/applications/${effectiveAppId}/external-assets`,
            {
              method: 'POST',
              headers: {
                Authorization: token,
                'Content-Type': 'application/json',
              },
              body: JSON.stringify({ urls: [fullUrl] }),
            }
          )
        } catch {
          res = null
        }
      }

      // 2. If direct fetch failed (CORS or network), fallback to WebX backend proxy
      if ((!res || !res.ok) && token) {
        const apiBase =
          useSettingsStore.getState().apiBaseUrl ||
          (typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8000')
        try {
          res = await fetch(`${apiBase}/api/discord/external-assets`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({
              urls: [fullUrl],
              application_id: effectiveAppId,
              token,
            }),
          })
        } catch {
          res = null
        }
      }

      if (res && res.ok) {
        const data = await res.json()
        if (Array.isArray(data) && data[0]?.external_asset_path) {
          const mpAsset = `mp:${data[0].external_asset_path}`
          assetCache.set(clean, mpAsset)
          return mpAsset
        }
      }
    } catch (err) {
      console.debug('[Discord] Asset resolution error:', err)
    }

    // Fallback: spotify:<id>, youtube:<id>, or clean
    const fallback = formatDiscordAsset(clean)
    if (fallback) {
      assetCache.set(clean, fallback)
    }
    return fallback
  })().finally(() => {
    pendingResolutions.delete(clean)
  })

  pendingResolutions.set(clean, p)
  return p
}

function buildGatewayPresence(
  t: ReturnType<typeof audioEngine.getState>['currentTrack'],
  isPlaying: boolean,
  pr: ReturnType<typeof audioEngine.getProgress>,
  showArt: boolean,
  clientId: string,
  resolvedAsset?: string | null
) {
  if (!t) {
    return {
      op: 3,
      d: {
        since: 0,
        activities: [],
        status: 'online',
        afk: false,
      },
    }
  }

  const duration = Math.round(pr.duration || t.duration_sec || 0)
  const position = Math.round(pr.currentTime || 0)

  const activity: Record<string, unknown> = {
    name: 'WebX',
    type: 2, // 2 = Listening to
    application_id: clientId || '1547543416143876167',
    details: t.title.slice(0, 128),
    state: (isPlaying ? `by ${t.artist}` : `Paused · ${t.artist}`).slice(0, 128),
  }

  if (isPlaying && duration > 0) {
    const startMs = Date.now() - position * 1000
    activity.timestamps = {
      start: startMs,
      end: startMs + duration * 1000,
    }
  }

  if (showArt && t.cover_url) {
    const asset = resolvedAsset || assetCache.get(t.cover_url.trim()) || formatDiscordAsset(t.cover_url)
    if (asset) {
      activity.assets = {
        large_image: asset,
        large_text: (t.album || t.title).slice(0, 128),
      }
    }
  }

  return {
    op: 3,
    d: {
      since: 0,
      activities: [activity],
      status: 'online',
      afk: false,
    },
  }
}

function buildDaemonPayload(
  t: ReturnType<typeof audioEngine.getState>['currentTrack'],
  isPlaying: boolean,
  pr: ReturnType<typeof audioEngine.getProgress>,
  showArt: boolean
) {
  if (!t) return { type: 'clear' }
  const duration = Math.round(pr.duration || t.duration_sec || 0)
  const position = Math.round(pr.currentTime || 0)
  return {
    type: 'presence',
    track_id: t.id,
    title: t.title,
    artist: t.artist,
    album: t.album ?? null,
    cover_url: showArt ? t.cover_url ?? null : null,
    duration,
    position,
    playing: isPlaying,
    ts: Date.now(),
  }
}

function send(force = false) {
  if (!ws || ws.readyState !== WebSocket.OPEN) return
  const s = useSettingsStore.getState()
  if (!s.discordEnabled) return

  // Do not send Gateway presence before Opcode 0 READY has authorized the socket
  if (s.discordMode === 'gateway' && !isGatewayReady) return

  const st = audioEngine.getState()
  const pr = audioEngine.getProgress()
  const t = st.currentTrack
  const isPlaying = st.isPlaying

  const payloadKey = `${t?.id ?? 'none'}:${isPlaying}:${Math.floor((pr.currentTime || 0) / 10)}`
  if (!force && payloadKey === lastPayloadKey) return
  lastPayloadKey = payloadKey
  lastSentAt = Date.now()

  try {
    if (s.discordMode === 'gateway') {
      const cover = t?.cover_url?.trim()
      const cachedAsset = cover ? assetCache.get(cover) : null

      // If artwork is enabled and not yet cached as mp:external, resolve in background
      if (
        s.discordShowArtwork &&
        cover &&
        (!cachedAsset || !cachedAsset.startsWith('mp:')) &&
        s.discordUserToken
      ) {
        resolveDiscordAsset(cover, s.discordClientId, s.discordUserToken.trim()).then((asset) => {
          if (
            asset &&
            asset !== cachedAsset &&
            ws &&
            ws.readyState === WebSocket.OPEN &&
            isGatewayReady
          ) {
            send(true)
          }
        })
      }

      const payload = buildGatewayPresence(
        t,
        isPlaying,
        pr,
        s.discordShowArtwork,
        s.discordClientId,
        cachedAsset
      )
      ws.send(JSON.stringify(payload))
    } else {
      const payload = buildDaemonPayload(t, isPlaying, pr, s.discordShowArtwork)
      ws.send(JSON.stringify(payload))
    }
  } catch {
    /* ignore */
  }
}

function connect() {
  const s = useSettingsStore.getState()
  if (!s.discordEnabled) {
    disconnect()
    return
  }

  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return

  clearTimers()

  if (s.discordMode === 'gateway') {
    connectGateway(s.discordUserToken.trim())
  } else {
    connectDaemon(s.discordDaemonUrl.trim())
  }
}

function connectGateway(token: string) {
  if (!token || token.length < 20) {
    updateState('off', 'Enter your Discord token to connect to Gateway', null)
    return
  }

  updateState('connecting', null, null)
  lastSeq = null
  isGatewayReady = false

  try {
    ws = new WebSocket('wss://gateway.discord.gg/?v=10&encoding=json')
  } catch {
    updateState('error', 'Could not open WebSocket connection', null)
    scheduleReconnect()
    return
  }

  ws.onopen = () => {
    backoff = 1000
    // Wait for Opcode 10 Hello before identifying
  }

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data as string)
      if (typeof data.s === 'number') lastSeq = data.s

      switch (data.op) {
        case 10: {
          // Opcode 10: Hello
          const interval = data.d?.heartbeat_interval || 41250
          if (heartbeatTimer !== null) window.clearInterval(heartbeatTimer)
          heartbeatTimer = window.setInterval(() => {
            if (ws && ws.readyState === WebSocket.OPEN) {
              ws.send(JSON.stringify({ op: 1, d: lastSeq }))
            }
          }, interval)

          // Send Opcode 2 Identify
          const osName =
            typeof navigator !== 'undefined' && /Mac|iPhone|iPad/i.test(navigator.userAgent)
              ? 'Mac OS X'
              : typeof navigator !== 'undefined' && /Win/i.test(navigator.userAgent)
                ? 'Windows'
                : 'Linux'

          const identify = {
            op: 2,
            d: {
              token: token,
              capabilities: 30717,
              properties: {
                os: osName,
                browser: 'Discord Client',
                device: 'WebX',
                $os: osName,
                $browser: 'Discord Client',
                $device: 'WebX',
              },
              presence: {
                status: 'online',
                since: 0,
                activities: [],
                afk: false,
              },
              compress: false,
            },
          }
          ws?.send(JSON.stringify(identify))
          break
        }

        case 0: {
          // Opcode 0: Dispatch
          if (data.t === 'READY') {
            isGatewayReady = true
            const username = data.d?.user?.username ? `@${data.d.user.username}` : 'Discord'
            updateState('connected', null, username)
            lastPayloadKey = ''
            send(true)
          }
          break
        }

        case 1: {
          // Heartbeat requested immediately
          if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ op: 1, d: lastSeq }))
          }
          break
        }

        case 11: {
          // Heartbeat ACK received
          break
        }

        case 7:
        case 9: {
          // Reconnect or invalid session
          disconnect()
          scheduleReconnect()
          break
        }
      }
    } catch {
      /* ignore JSON parse errors */
    }
  }

  ws.onclose = (event) => {
    ws = null
    isGatewayReady = false
    clearTimers()
    if (!useSettingsStore.getState().discordEnabled) {
      updateState('off', null, null)
      return
    }

    if (event.code === 4004) {
      // Authentication failed — do not loop reconnect
      updateState('error', 'Authentication failed — invalid Discord token', null)
    } else {
      updateState('error', `Gateway disconnected (${event.code || 'network'})`, null)
      scheduleReconnect()
    }
  }

  ws.onerror = () => {
    updateState('error', 'Connection error', null)
  }
}

function connectDaemon(url: string) {
  updateState('connecting', null, null)
  try {
    ws = new WebSocket(url || 'ws://127.0.0.1:6472')
  } catch {
    updateState('error', 'Invalid daemon address', null)
    scheduleReconnect()
    return
  }

  ws.onopen = () => {
    backoff = 1000
    updateState('connected', null, 'Local Daemon')
    lastPayloadKey = ''
    send(true)
  }

  ws.onclose = () => {
    ws = null
    if (useSettingsStore.getState().discordEnabled) {
      updateState('error', 'Helper daemon not reachable', null)
      scheduleReconnect()
    } else {
      updateState('off', null, null)
    }
  }

  ws.onerror = () => {
    updateState('error', 'Could not connect to helper daemon', null)
  }
}

/** Idempotent — start the bridge and follow the settings toggle. */
export function startDiscordPresence(): void {
  if (started || typeof window === 'undefined') return
  started = true

  let lastTrackId: string | null = null
  audioEngine.subscribeState((st) => {
    const id = st.currentTrack?.id ?? null
    const changed = id !== lastTrackId
    lastTrackId = id
    if (changed || !st.isBuffering) send(changed)
  })

  // Position drifts (seeks, pauses) — re-sync at most every 15 s while playing
  audioEngine.subscribeProgress(() => {
    if (Date.now() - lastSentAt > 15_000 && audioEngine.getState().isPlaying) send(true)
  })

  let prev = useSettingsStore.getState()
  useSettingsStore.subscribe((s) => {
    const changedConfig =
      s.discordEnabled !== prev.discordEnabled ||
      s.discordMode !== prev.discordMode ||
      s.discordUserToken !== prev.discordUserToken ||
      s.discordClientId !== prev.discordClientId ||
      s.discordDaemonUrl !== prev.discordDaemonUrl

    if (changedConfig) {
      disconnect()
      backoff = 1000
      if (s.discordEnabled) connect()
    } else if (s.discordShowArtwork !== prev.discordShowArtwork) {
      send(true)
    }
    prev = s
  })

  if (prev.discordEnabled) connect()
}

/** Manual reconnect from the settings UI. */
export function reconnectDiscord(): void {
  disconnect()
  backoff = 1000
  connect()
}
