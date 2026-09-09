/**
 * Stream URL construction + cache warming, ported from StreamXWeb.
 *
 * `<audio>` cannot send headers, so the auth token travels as `?token=`.
 * ALAC is transcoded to FLAC on the server when the browser can't decode it.
 */
import { API_ENDPOINTS } from './endpoints'
import { buildUrl, getToken, http } from './client'
import type { Track } from '@/schemas/track'
import { useSettingsStore } from '@/stores/settingsStore'

let alacSupport: boolean | null = null
export function canBrowserPlayAlac(): boolean {
  if (alacSupport !== null) return alacSupport
  try {
    const a = document.createElement('audio')
    alacSupport = a.canPlayType('audio/mp4; codecs="alac"') !== ''
  } catch {
    alacSupport = false
  }
  return alacSupport
}

let flacSupport: boolean | null = null
export function canBrowserPlayFlac(): boolean {
  if (flacSupport !== null) return flacSupport
  try {
    const a = document.createElement('audio')
    flacSupport = a.canPlayType('audio/flac') !== '' || a.canPlayType('audio/x-flac') !== ''
  } catch {
    flacSupport = false
  }
  return flacSupport
}

/** Decide which `format` query param (if any) to request for a track */
export function resolveStreamFormat(track: Track): string | undefined {
  const pref = useSettingsStore.getState().streamFormat
  const type = (track.type || '').toLowerCase()
  if (pref === 'flac') return 'flac'
  if (pref === 'original') return undefined
  // auto: transcode ALAC → FLAC when the browser can't decode ALAC natively
  if (type.includes('alac') && !canBrowserPlayAlac()) return 'flac'
  return undefined
}

export function getStreamUrl(track: Track, opts?: { format?: string; token?: string | null }): string {
  if (track.stream_url) return track.stream_url
  const token = opts?.token !== undefined ? opts.token : getToken()
  const format = opts?.format !== undefined ? opts.format : resolveStreamFormat(track)
  return buildUrl(API_ENDPOINTS.TRACK_STREAM(track.id), { token, format })
}

export function getDownloadUrl(trackId: string): string {
  return buildUrl(API_ENDPOINTS.TRACK_DOWNLOAD(trackId), { token: getToken() })
}

const warmedAt = new Map<string, number>()
const warmInflight = new Map<string, Promise<{ ok: boolean; ready: boolean }>>()
const WARM_TTL = 10 * 60 * 1000

/** Ask the server to fetch/cache the file so the first byte arrives fast */
export async function warmTrack(trackId: string, opts?: { force?: boolean }): Promise<{ ok: boolean; ready: boolean }> {
  const now = Date.now()
  if (!opts?.force && now - (warmedAt.get(trackId) ?? 0) < WARM_TTL) return { ok: true, ready: true }
  const existing = warmInflight.get(trackId)
  if (existing) return existing
  const p = (async () => {
    try {
      const data = await http.get<{ ok?: boolean; ready?: boolean }>(API_ENDPOINTS.TRACK_WARM(trackId), { timeoutMs: 25000, noDedupe: true })
      const ready = data?.ready !== false
      if (ready) warmedAt.set(trackId, Date.now())
      return { ok: true, ready }
    } catch {
      return { ok: false, ready: false }
    } finally {
      warmInflight.delete(trackId)
    }
  })()
  warmInflight.set(trackId, p)
  return p
}
