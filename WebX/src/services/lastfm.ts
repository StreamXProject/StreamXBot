/**
 * Last.fm scrobbling — runs entirely in the browser using the user's own API key + secret.
 *
 * Flow: auth.getToken → user approves at last.fm → auth.getSession (stored) → track.updateNowPlaying on
 * track start and track.scrobble once playback passes the configured fraction (default 50%).
 * Last.fm's API endpoint sends CORS headers, so no server proxy is needed.
 */
import { md5 } from '@/lib/md5'
import { useSettingsStore } from '@/stores/settingsStore'
import { audioEngine } from '@/audio/AudioEngine'
import type { Track } from '@/schemas/track'

const API = 'https://ws.audioscrobbler.com/2.0/'

type Params = Record<string, string>

function sign(params: Params, secret: string): string {
  const keys = Object.keys(params).filter((k) => k !== 'format' && k !== 'callback').sort()
  return md5(keys.map((k) => `${k}${params[k]}`).join('') + secret)
}

async function call<T>(method: string, params: Params, opts: { post?: boolean; signed?: boolean } = {}): Promise<T> {
  const { lastfmApiKey: apiKey, lastfmApiSecret: secret } = useSettingsStore.getState()
  if (!apiKey) throw new Error('Last.fm API key is missing')
  const full: Params = { method, api_key: apiKey, ...params }
  if (opts.signed !== false) {
    if (!secret) throw new Error('Last.fm shared secret is missing')
    full.api_sig = sign(full, secret)
  }
  full.format = 'json'
  const body = new URLSearchParams(full)
  const res = opts.post
    ? await fetch(API, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body })
    : await fetch(`${API}?${body.toString()}`)
  const data = (await res.json().catch(() => ({}))) as T & { error?: number; message?: string }
  if (!res.ok || data.error) throw new Error(data.message || `Last.fm error ${data.error ?? res.status}`)
  return data
}

/** Step 1 — request a token and return the URL the user must approve. */
export async function beginAuth(): Promise<{ token: string; url: string }> {
  const { token } = await call<{ token: string }>('auth.getToken', {})
  const apiKey = useSettingsStore.getState().lastfmApiKey
  return { token, url: `https://www.last.fm/api/auth/?api_key=${encodeURIComponent(apiKey)}&token=${encodeURIComponent(token)}` }
}

/** Step 2 — after approval, exchange the token for a permanent session key. */
export async function finishAuth(token: string): Promise<{ name: string; key: string }> {
  const data = await call<{ session: { name: string; key: string } }>('auth.getSession', { token })
  useSettingsStore.getState().set('lastfmSessionKey', data.session.key)
  useSettingsStore.getState().set('lastfmUsername', data.session.name)
  return data.session
}

export function disconnect(): void {
  const set = useSettingsStore.getState().set
  set('lastfmSessionKey', '')
  set('lastfmUsername', '')
  set('lastfmEnabled', false)
}

function trackParams(track: Track): Params {
  const p: Params = { artist: track.artist || 'Unknown artist', track: track.title }
  if (track.album) p.album = track.album
  if (track.duration_sec) p.duration = String(Math.round(track.duration_sec))
  return p
}

export async function updateNowPlaying(track: Track): Promise<void> {
  const { lastfmSessionKey: sk } = useSettingsStore.getState()
  if (!sk) return
  await call('track.updateNowPlaying', { ...trackParams(track), sk }, { post: true })
}

export async function scrobble(track: Track, startedAtSec: number): Promise<void> {
  const { lastfmSessionKey: sk } = useSettingsStore.getState()
  if (!sk) return
  await call('track.scrobble', { ...trackParams(track), timestamp: String(Math.floor(startedAtSec)), chosenByUser: '1', sk }, { post: true })
}

/* ---------- automatic scrobbler ---------- */

let started = false
let currentId: string | null = null
let startedAt = 0
let scrobbled = false
let nowPlayingSent = false
let lastFailToast = 0

export interface ScrobbleStatus { lastScrobbled?: string; lastError?: string; count: number }
export const scrobbleStatus: ScrobbleStatus = { count: 0 }
const statusListeners = new Set<() => void>()
export function onScrobbleStatus(fn: () => void): () => void {
  statusListeners.add(fn)
  return () => statusListeners.delete(fn)
}
const emit = () => statusListeners.forEach((fn) => fn())

function enabled(): boolean {
  const s = useSettingsStore.getState()
  return s.lastfmEnabled && Boolean(s.lastfmSessionKey && s.lastfmApiKey && s.lastfmApiSecret)
}

function fail(err: unknown) {
  scrobbleStatus.lastError = (err as Error).message
  emit()
  const now = Date.now()
  if (now - lastFailToast > 60_000) {
    lastFailToast = now
    console.warn('[Last.fm]', err)
  }
}

/** Idempotent — wire the scrobbler to the audio engine. */
export function startScrobbler(): void {
  if (started || typeof window === 'undefined') return
  started = true

  audioEngine.subscribeState((st) => {
    const t = st.currentTrack
    if (!t) {
      currentId = null
      return
    }
    if (t.id !== currentId) {
      currentId = t.id
      startedAt = Date.now() / 1000
      scrobbled = false
      nowPlayingSent = false
    }
    if (st.isPlaying && !nowPlayingSent && enabled() && useSettingsStore.getState().lastfmNowPlaying) {
      nowPlayingSent = true
      updateNowPlaying(t).catch(fail)
    }
  })

  audioEngine.subscribeProgress((p) => {
    if (scrobbled || !currentId || !enabled()) return
    const st = audioEngine.getState()
    const t = st.currentTrack
    if (!t || t.id !== currentId || !st.isPlaying) return
    const duration = p.duration || t.duration_sec || 0
    if (duration < 30) return // Last.fm ignores tracks shorter than 30 s
    const threshold = Math.min(0.95, Math.max(0.1, useSettingsStore.getState().lastfmScrobbleAt))
    // Last.fm rule: half the track, or 4 minutes, whichever comes first
    const needed = Math.min(duration * threshold, 240)
    if (p.currentTime >= needed) {
      scrobbled = true
      scrobble(t, startedAt)
        .then(() => {
          scrobbleStatus.count += 1
          scrobbleStatus.lastScrobbled = `${t.title} — ${t.artist}`
          scrobbleStatus.lastError = undefined
          emit()
        })
        .catch((e) => {
          scrobbled = false // allow a retry on the next tick
          fail(e)
        })
    }
  })
}
