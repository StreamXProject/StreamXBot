import { API_ENDPOINTS } from './endpoints'
import { http } from './client'
import { BrowseResponseSchema, type BrowseResponse } from '@/schemas/browse'
import { parseTracks, type Track } from '@/schemas/track'
import { AvailablePlaylistSchema, type AvailablePlaylist } from '@/schemas/playlist'

export async function fetchBrowseTracks(page = 1, perPage = 50, signal?: AbortSignal): Promise<BrowseResponse> {
  const raw = await http.get<unknown>(API_ENDPOINTS.BROWSE, { signal, params: { page, per_page: perPage, limit: perPage } })
  return BrowseResponseSchema.parse(raw)
}

export type FeaturedMix = AvailablePlaylist & { subtitle: string }

const MIX_SUBTITLES: Record<string, string> = {
  'daily:random': 'A fresh rotation from across your library',
  'daily:rediscover': 'Tracks you loved and haven\u2019t played in a while',
  'daily:rising': 'Recently added and picking up plays',
  'daily:surprise': 'Genres you don\u2019t usually reach for',
  me_top_played: 'Your most played tracks',
}

export async function fetchFeaturedMixes(signal?: AbortSignal): Promise<FeaturedMix[]> {
  const raw = await http.get<{ items?: unknown[] } | unknown[]>(API_ENDPOINTS.PLAYLISTS_AVAILABLE, { signal })
  const items = Array.isArray(raw) ? raw : raw?.items ?? []
  return items
    .map((i) => AvailablePlaylistSchema.safeParse(i))
    .filter((r) => r.success)
    .map((r) => {
      const p = r.data!
      return { ...p, subtitle: MIX_SUBTITLES[p.id] ?? MIX_SUBTITLES[p.kind] ?? 'Curated for you' }
    })
}

/** Load tracks for any curated endpoint (`/daily-playlist/random`, `/me/top-played`, …) */
export async function fetchMixTracks(endpointOrKey: string, signal?: AbortSignal): Promise<Track[]> {
  const path = endpointOrKey.startsWith('/') ? endpointOrKey : API_ENDPOINTS.DAILY_PLAYLIST(endpointOrKey.replace(/^daily:/, ''))
  const raw = await http.get<{ items?: unknown[] } | unknown[]>(path, { signal, params: { limit: 75 } })
  return parseTracks(Array.isArray(raw) ? raw : raw?.items)
}

export const fetchDailyPlaylistTracks = fetchMixTracks

export async function fetchShuffle(limit = 100, opts?: { lossless?: boolean; artist?: string; genre?: string }, signal?: AbortSignal): Promise<Track[]> {
  const raw = await http.get<{ items?: unknown[] }>(API_ENDPOINTS.TRACKS_SHUFFLE, { signal, params: { limit, ...opts }, noDedupe: true })
  return parseTracks(raw?.items)
}

export interface Topic { name: string; count?: number }
export async function fetchTopics(signal?: AbortSignal): Promise<Topic[]> {
  const raw = await http.get<{ items?: unknown[] } | unknown[]>(API_ENDPOINTS.TOPICS, { signal })
  const items = Array.isArray(raw) ? raw : raw?.items ?? []
  return items
    .map((i) => {
      if (typeof i === 'string') return { name: i }
      const o = i as Record<string, unknown>
      const name = (o.name ?? o.topic_name ?? o.title) as string | undefined
      return name ? { name, count: typeof o.count === 'number' ? o.count : undefined } : null
    })
    .filter((t): t is Topic => t !== null)
}

export const TOPIC_PAGE_SIZE = 100 // backend caps /topics/{name}/tracks at limit<=100

export async function fetchTopicTracks(name: string, signal?: AbortSignal): Promise<Track[]> {
  const res = await fetchTopicTracksPage(name, 1, TOPIC_PAGE_SIZE, signal)
  return res.items
}

export async function fetchTopicTracksPage(name: string, page = 1, perPage = TOPIC_PAGE_SIZE, signal?: AbortSignal): Promise<{ items: Track[]; page: number; per_page: number; total: number }> {
  const safe = Math.min(Math.max(1, perPage), TOPIC_PAGE_SIZE)
  const raw = await http.get<{ items?: unknown[]; page?: number; per_page?: number; total?: number } | unknown[]>(API_ENDPOINTS.TOPIC_TRACKS(name), { signal, params: { page, limit: safe } })
  const items = parseTracks(Array.isArray(raw) ? raw : raw?.items)
  const obj = Array.isArray(raw) ? {} : raw ?? {}
  return { items, page: obj.page ?? page, per_page: obj.per_page ?? safe, total: obj.total ?? items.length }
}
