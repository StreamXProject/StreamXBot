import { API_ENDPOINTS } from './endpoints'
import { http } from './client'
import { BrowseResponseSchema } from '@/schemas/browse'
import { ArtistSchema, type Artist } from '@/schemas/artist'
import type { Album } from '@/schemas/album'
import type { Track } from '@/schemas/track'
import { fetchAlbums } from './albums'

export interface SearchResult {
  tracks: Track[]
  total: number
  albums: Album[]
  artists: Artist[]
}

const EMPTY: SearchResult = { tracks: [], total: 0, albums: [], artists: [] }

export async function searchArtists(q: string, signal?: AbortSignal): Promise<Artist[]> {
  const raw = await http.get<{ items?: unknown[] } | unknown[]>(API_ENDPOINTS.SEARCH_ARTISTS, { signal, params: { q, limit: 12 } })
  const items = Array.isArray(raw) ? raw : raw?.items ?? []
  return items.map((i) => ArtistSchema.safeParse(i)).filter((r) => r.success).map((r) => r.data!).filter((a) => a.id)
}

export async function searchAll(query: string, signal?: AbortSignal): Promise<SearchResult> {
  const q = query.trim()
  if (!q) return EMPTY
  const lq = q.toLowerCase()

  const [tracksRes, artistsRes, albumsRes] = await Promise.allSettled([
    http.get<unknown>(API_ENDPOINTS.SEARCH, { signal, params: { q, limit: 50 } }).then((r) => BrowseResponseSchema.parse(r)),
    searchArtists(q, signal),
    fetchAlbums(signal),
  ])

  if (tracksRes.status === 'rejected' && artistsRes.status === 'rejected') throw tracksRes.reason

  const tracks = tracksRes.status === 'fulfilled' ? tracksRes.value : { items: [], total: 0 }
  const albums = albumsRes.status === 'fulfilled'
    ? albumsRes.value.filter((a) => a.title.toLowerCase().includes(lq) || a.artist.toLowerCase().includes(lq)).slice(0, 12)
    : []

  return {
    tracks: tracks.items,
    total: tracks.total,
    albums,
    artists: artistsRes.status === 'fulfilled' ? artistsRes.value : [],
  }
}
