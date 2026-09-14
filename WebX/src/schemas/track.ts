import { z } from 'zod'

/**
 * The backend exposes tracks in two shapes:
 *  - BrowseItem (flat): { _id, title, artist, album, duration_sec, cover_url, type, … }
 *  - TrackResponse (nested): { _id, audio: {title, artist, …}, spotify: {cover_url, …} }
 * This schema normalises both into a single `Track`.
 */
export const TrackCoreSchema = z
  .object({
    id: z.string().optional(),
    _id: z.string().optional(),
    title: z.string().optional().nullable(),
    artist: z.string().optional().nullable(),
    album: z.string().optional().nullable(),
    album_id: z.string().optional().nullable(),
    artist_id: z.string().optional().nullable(),
    duration_sec: z.number().optional().nullable(),
    cover_url: z.string().optional().nullable(),
    liked: z.boolean().optional().default(false),
    spotify_url: z.string().optional().nullable(),
    stream_url: z.string().optional().nullable(),
    source_chat_id: z.number().optional().nullable(),
    source_message_id: z.number().optional().nullable(),
    sampling_rate_hz: z.number().optional().nullable(),
    type: z.string().optional().nullable(),
    topic_name: z.string().optional().nullable(),
    updated_at: z.number().optional().nullable(),
    audio: z.record(z.string(), z.any()).optional().nullable(),
    spotify: z.record(z.string(), z.any()).optional().nullable(),
    telegram: z.record(z.string(), z.any()).optional().nullable(),
  })
  .passthrough()

export interface Track {
  id: string
  title: string
  artist: string
  album?: string
  album_id?: string | null
  artist_id?: string | null
  duration_sec?: number
  cover_url?: string | null
  liked?: boolean
  spotify_url?: string | null
  /** Absolute stream URL if the server supplied one; otherwise built at play time */
  stream_url?: string | null
  source_chat_id?: number | null
  source_message_id?: number | null
  sampling_rate_hz?: number | null
  bit_depth?: number | null
  bitrate_kbps?: number | null
  year?: number | null
  type?: string | null
  topic_name?: string | null
  updated_at?: number | null
}

const UNKNOWN = new Set(['Unknown Title', 'Unknown Artist', 'Unknown Album', '', null, undefined])
const pick = (...vals: unknown[]): string | undefined => {
  for (const v of vals) if (typeof v === 'string' && !UNKNOWN.has(v.trim())) return v.trim()
  return undefined
}

export const TrackSchema = TrackCoreSchema.transform((data): Track => {
  const audio = (data.audio ?? {}) as Record<string, unknown>
  const spotify = (data.spotify ?? {}) as Record<string, unknown>
  const artists = Array.isArray(audio.artists) ? (audio.artists as string[]).join(', ') : undefined

  const id = String(data.id || data._id || '')
  const title = pick(data.title, audio.title) || 'Unknown Title'
  const artist = pick(data.artist, audio.artist, audio.performer, artists) || 'Unknown Artist'
  const album = pick(data.album, audio.album)
  const cover_url = pick(data.cover_url, spotify.big_cover_url, spotify.cover_url) || null
  const stream_url = data.stream_url && /^https?:\/\//.test(data.stream_url) ? data.stream_url : null

  return {
    id,
    title,
    artist,
    album,
    album_id: data.album_id || (audio.album_id as string | undefined) || null,
    artist_id: data.artist_id || (audio.artist_id as string | undefined) || null,
    duration_sec: Number(data.duration_sec || audio.duration_sec || 0) || 0,
    cover_url,
    liked: Boolean(data.liked),
    spotify_url: data.spotify_url || (spotify.url as string | undefined) || null,
    stream_url,
    source_chat_id: data.source_chat_id ?? null,
    source_message_id: data.source_message_id ?? null,
    sampling_rate_hz: data.sampling_rate_hz || (audio.sampling_rate_hz as number | undefined) || null,
    bit_depth: (audio.bit_depth as number | undefined) || null,
    bitrate_kbps: (audio.bitrate_kbps as number | undefined) || null,
    year: (audio.year as number | undefined) || null,
    type: data.type || (audio.type as string | undefined) || null,
    topic_name: data.topic_name || null,
    updated_at: data.updated_at ?? null,
  }
})

export function parseTracks(items: unknown): Track[] {
  const arr = Array.isArray(items) ? items : []
  const out: Track[] = []
  for (const raw of arr) {
    // Favourites wrap the track: { track: {...}, created_at }
    const candidate = raw && typeof raw === 'object' && 'track' in (raw as object) ? (raw as { track: unknown }).track : raw
    const r = TrackSchema.safeParse(candidate)
    if (r.success && r.data.id) out.push(r.data)
  }
  return out
}

export const LyricWordSpanSchema = z.object({
  time: z.number(),
  duration: z.number().optional(),
  text: z.string(),
})
export type LyricWordSpan = z.infer<typeof LyricWordSpanSchema>

export const LyricLineSchema = z.object({
  time: z.number(),
  text: z.string(),
  duration: z.number().optional(),
  spans: z.array(LyricWordSpanSchema).optional(),
})
export type LyricLine = z.infer<typeof LyricLineSchema>

export interface Lyrics {
  lines: LyricLine[]
  plain: string
  synced: boolean
  source?: string
  kind?: string
}
